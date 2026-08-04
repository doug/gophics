//go:build !nogpu

// Package gpu provides a GPU-accelerated rendering backend using gogpu/wgpu.
package gpu

import (
	"errors"
	"fmt"
	"sync"

	"github.com/doug/gophics/internal/gfx/gputypes"
)

// Command encoder errors.
var (
	// ErrEncoderNotRecording is returned when recording operations are called
	// on an encoder that is not in the Recording state.
	ErrEncoderNotRecording = errors.New("gpu: encoder not in recording state")

	// ErrEncoderLocked is returned when operations are called on an encoder
	// that is locked (a pass is in progress).
	ErrEncoderLocked = errors.New("gpu: encoder is locked (pass in progress)")

	// ErrEncoderFinished is returned when operations are called on an encoder
	// that has already been finished.
	ErrEncoderFinished = errors.New("gpu: encoder already finished")

	// ErrEncoderConsumed is returned when operations are called on an encoder
	// that has been submitted to the queue.
	ErrEncoderConsumed = errors.New("gpu: encoder has been consumed")

	// ErrNilDevice is returned when creating an encoder without a device.
	ErrNilDevice = errors.New("gpu: device is nil")

	// ErrNilEncoder is returned when operations reference a nil encoder.
	ErrNilEncoder = errors.New("gpu: command encoder is nil")

	// ErrNilCoreBuffer is returned when a buffer operation references nil.
	ErrNilCoreBuffer = errors.New("gpu: core buffer is nil")

	// ErrCopyRangeOutOfBounds is returned when a copy operation exceeds buffer bounds.
	ErrCopyRangeOutOfBounds = errors.New("gpu: copy range out of bounds")

	// ErrCopyOverlap is returned when source and destination buffers overlap.
	ErrCopyOverlap = errors.New("gpu: source and destination buffers overlap")

	// ErrCopyOffsetNotAligned is returned when offset is not properly aligned.
	ErrCopyOffsetNotAligned = errors.New("gpu: copy offset must be 4-byte aligned")

	// ErrCopySizeNotAligned is returned when size is not properly aligned.
	ErrCopySizeNotAligned = errors.New("gpu: copy size must be 4-byte aligned")
)

// CommandEncoderStatus is the lifecycle state of a CoreCommandEncoder.
//
// It mirrors the WebGPU command-encoder state machine. It is a local,
// portable type (no native HAL dependency) so the encoder builds for every
// platform, WASM included.
type CommandEncoderStatus uint8

const (
	// CommandEncoderStatusRecording means the encoder accepts commands.
	CommandEncoderStatusRecording CommandEncoderStatus = iota
	// CommandEncoderStatusLocked means a pass is in progress.
	CommandEncoderStatusLocked
	// CommandEncoderStatusFinished means Finish() was called.
	CommandEncoderStatusFinished
	// CommandEncoderStatusConsumed means the buffer was submitted.
	CommandEncoderStatusConsumed
	// CommandEncoderStatusError is an invalid/nil encoder.
	CommandEncoderStatusError
)

// QuerySetID identifies a timestamp/occlusion query set.
type QuerySetID uint64

// =============================================================================
// Command Encoder
// =============================================================================

// CoreCommandEncoder records GPU commands for later submission to a queue.
//
// It provides a Go-style, immediate-error API over the WebGPU command
// encoding pattern:
//  1. Create encoder via NewCoreCommandEncoder()
//  2. Record commands (copy operations, begin/end passes)
//  3. Call Finish() to get a CoreCommandBuffer
//  4. Submit CoreCommandBuffer to a Queue
//
// State machine:
//
//	Recording -> (BeginRenderPass/BeginComputePass) -> Locked
//	Locked    -> (EndPass)                          -> Recording
//	Recording -> Finish()                           -> Finished
//	Finished  -> (submitted to queue)               -> Consumed
//
// CoreCommandEncoder is NOT safe for concurrent use. Each encoder should
// be used from a single goroutine.
type CoreCommandEncoder struct {
	mu sync.Mutex

	// device is the parent gpu backend device reference.
	device *Backend

	// label is the debug label for this encoder.
	label string

	// finished is set once Finish() has been called.
	finished bool

	// activeRenderPass tracks the currently active render pass (if any).
	activeRenderPass *RenderPassEncoder

	// activeComputePass tracks the currently active compute pass (if any).
	activeComputePass *ComputePassEncoder
}

// NewCoreCommandEncoder creates a new command encoder from a backend.
//
// The encoder is created in the Recording state, ready to record commands.
//
// Returns nil and an error if the backend is not initialized.
func NewCoreCommandEncoder(backend *Backend, label string) (*CoreCommandEncoder, error) {
	if backend == nil {
		return nil, ErrNilDevice
	}

	backend.mu.RLock()
	defer backend.mu.RUnlock()

	if !backend.initialized {
		return nil, ErrNotInitialized
	}

	enc := &CoreCommandEncoder{
		device: backend,
		label:  label,
	}

	return enc, nil
}

// Label returns the encoder's debug label.
func (e *CoreCommandEncoder) Label() string {
	if e == nil {
		return ""
	}
	return e.label
}

// Status returns the current encoder status.
func (e *CoreCommandEncoder) Status() CommandEncoderStatus {
	if e == nil {
		return CommandEncoderStatusError
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.statusLocked()
}

// checkRecording returns an error if the encoder is not in Recording state.
// This method acquires the mutex; use checkRecordingLocked if mutex is already held.
//
//nolint:unused // Public API for external use, internal code uses checkRecordingLocked
func (e *CoreCommandEncoder) checkRecording() error {
	if e == nil {
		return ErrNilEncoder
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return e.checkRecordingLocked()
}

// checkRecordingLocked returns an error if the encoder is not in Recording state.
// The caller must hold e.mu.
func (e *CoreCommandEncoder) checkRecordingLocked() error {
	status := e.statusLocked()
	switch status {
	case CommandEncoderStatusRecording:
		return nil
	case CommandEncoderStatusLocked:
		return ErrEncoderLocked
	case CommandEncoderStatusFinished:
		return ErrEncoderFinished
	case CommandEncoderStatusConsumed:
		return ErrEncoderConsumed
	default:
		return ErrEncoderNotRecording
	}
}

// statusLocked returns the encoder status. The caller must hold e.mu.
func (e *CoreCommandEncoder) statusLocked() CommandEncoderStatus {
	if e.finished {
		return CommandEncoderStatusFinished
	}
	if e.activeRenderPass != nil || e.activeComputePass != nil {
		return CommandEncoderStatusLocked
	}
	return CommandEncoderStatusRecording
}

// BeginRenderPass starts a render pass with the given descriptor.
//
// The encoder must be in the Recording state. After this call, the encoder
// transitions to the Locked state, returning to Recording when the pass ends.
func (e *CoreCommandEncoder) BeginRenderPass(desc *RenderPassDescriptor) (*RenderPassEncoder, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return nil, fmt.Errorf("begin render pass: %w", err)
	}

	if desc == nil {
		return nil, fmt.Errorf("begin render pass: descriptor is nil")
	}

	pass := &RenderPassEncoder{
		encoder: e,
		state:   RenderPassStateRecording,
	}
	e.activeRenderPass = pass
	return pass, nil
}

// endRenderPass ends the current render pass.
//
// This is called internally by RenderPassEncoder.End().
// The encoder transitions from Locked back to Recording state.
func (e *CoreCommandEncoder) endRenderPass(pass *RenderPassEncoder) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeRenderPass != pass {
		return fmt.Errorf("end render pass: wrong pass being ended")
	}

	e.activeRenderPass = nil
	return nil
}

// BeginComputePass starts a compute pass with the given descriptor.
//
// The encoder must be in the Recording state. After this call, the encoder
// transitions to the Locked state, returning to Recording when the pass ends.
func (e *CoreCommandEncoder) BeginComputePass(desc *ComputePassDescriptor) (*ComputePassEncoder, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return nil, fmt.Errorf("begin compute pass: %w", err)
	}

	pass := &ComputePassEncoder{
		encoder: e,
		state:   ComputePassStateRecording,
	}
	e.activeComputePass = pass
	return pass, nil
}

// endComputePass ends the current compute pass.
//
// This is called internally by ComputePassEncoder.End().
// The encoder transitions from Locked back to Recording state.
func (e *CoreCommandEncoder) endComputePass(pass *ComputePassEncoder) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.activeComputePass != pass {
		return fmt.Errorf("end compute pass: wrong pass being ended")
	}

	e.activeComputePass = nil
	return nil
}

// CopyBufferToBuffer copies data from one buffer to another.
//
// The encoder must be in the Recording state. Both offsets and size must be
// 4-byte aligned, and the ranges must be within buffer bounds.
func (e *CoreCommandEncoder) CopyBufferToBuffer(src, dst *Buffer, srcOffset, dstOffset, size uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy buffer to buffer: %w", err)
	}

	// Validate buffers
	if src == nil || dst == nil {
		return ErrNilCoreBuffer
	}

	// Validate alignment (WebGPU requires 4-byte alignment)
	const alignment uint64 = 4
	if srcOffset%alignment != 0 {
		return fmt.Errorf("%w: source offset %d", ErrCopyOffsetNotAligned, srcOffset)
	}
	if dstOffset%alignment != 0 {
		return fmt.Errorf("%w: destination offset %d", ErrCopyOffsetNotAligned, dstOffset)
	}
	if size%alignment != 0 {
		return fmt.Errorf("%w: size %d", ErrCopySizeNotAligned, size)
	}

	// Validate bounds
	if srcOffset+size > src.Size() {
		return fmt.Errorf("%w: source offset %d + size %d > buffer size %d",
			ErrCopyRangeOutOfBounds, srcOffset, size, src.Size())
	}
	if dstOffset+size > dst.Size() {
		return fmt.Errorf("%w: destination offset %d + size %d > buffer size %d",
			ErrCopyRangeOutOfBounds, dstOffset, size, dst.Size())
	}

	// TODO: When HAL is integrated, record the copy command via the portable
	// wgpu encoder (encoder.CopyBufferToBuffer).

	return nil
}

// CopyBufferToTexture copies data from a buffer to a texture.
//
// The encoder must be in the Recording state.
func (e *CoreCommandEncoder) CopyBufferToTexture(source *ImageCopyBuffer, destination *ImageCopyTexture, copySize gputypes.Extent3D) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy buffer to texture: %w", err)
	}

	if source == nil {
		return fmt.Errorf("copy buffer to texture: source is nil")
	}
	if destination == nil {
		return fmt.Errorf("copy buffer to texture: destination is nil")
	}

	// TODO: When HAL is integrated, record the copy command

	return nil
}

// CopyTextureToBuffer copies data from a texture to a buffer.
//
// The encoder must be in the Recording state.
func (e *CoreCommandEncoder) CopyTextureToBuffer(source *ImageCopyTexture, destination *ImageCopyBuffer, copySize gputypes.Extent3D) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy texture to buffer: %w", err)
	}

	if source == nil {
		return fmt.Errorf("copy texture to buffer: source is nil")
	}
	if destination == nil {
		return fmt.Errorf("copy texture to buffer: destination is nil")
	}

	// TODO: When HAL is integrated, record the copy command

	return nil
}

// CopyTextureToTexture copies data from one texture to another.
//
// The encoder must be in the Recording state.
func (e *CoreCommandEncoder) CopyTextureToTexture(source, destination *ImageCopyTexture, copySize gputypes.Extent3D) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("copy texture to texture: %w", err)
	}

	if source == nil {
		return fmt.Errorf("copy texture to texture: source is nil")
	}
	if destination == nil {
		return fmt.Errorf("copy texture to texture: destination is nil")
	}

	// TODO: When HAL is integrated, record the copy command

	return nil
}

// ClearBuffer clears a region of a buffer to zero.
//
// The encoder must be in the Recording state. Offset and size must be
// 4-byte aligned (size 0 clears from offset to the end of the buffer).
func (e *CoreCommandEncoder) ClearBuffer(buffer *Buffer, offset, size uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return fmt.Errorf("clear buffer: %w", err)
	}

	if buffer == nil {
		return ErrNilCoreBuffer
	}

	// Validate alignment
	const alignment uint64 = 4
	if offset%alignment != 0 {
		return fmt.Errorf("%w: offset %d", ErrCopyOffsetNotAligned, offset)
	}

	// Size 0 means clear entire buffer from offset
	actualSize := size
	if actualSize == 0 {
		actualSize = buffer.Size() - offset
	}

	if actualSize%alignment != 0 {
		return fmt.Errorf("%w: size %d", ErrCopySizeNotAligned, actualSize)
	}

	// Validate bounds
	if offset+actualSize > buffer.Size() {
		return fmt.Errorf("%w: offset %d + size %d > buffer size %d",
			ErrCopyRangeOutOfBounds, offset, actualSize, buffer.Size())
	}

	// TODO: When HAL is integrated, record the clear command

	return nil
}

// Finish completes recording and returns a command buffer.
//
// The encoder must be in the Recording state (no active passes). After this
// call the encoder transitions to Finished and cannot record further.
func (e *CoreCommandEncoder) Finish() (*CoreCommandBuffer, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.checkRecordingLocked(); err != nil {
		return nil, fmt.Errorf("finish: %w", err)
	}

	e.finished = true
	return &CoreCommandBuffer{
		label: e.label,
	}, nil
}

// =============================================================================
// Supporting Types
// =============================================================================

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	// Label is an optional debug name.
	Label string

	// ColorAttachments are the color render targets.
	ColorAttachments []RenderPassColorAttachment

	// DepthStencilAttachment is the depth/stencil target (optional).
	DepthStencilAttachment *RenderPassDepthStencilAttachment
}

// RenderPassColorAttachment describes a color attachment.
type RenderPassColorAttachment struct {
	// View is the texture view to render to.
	View *TextureView

	// ResolveTarget is the MSAA resolve target (optional).
	ResolveTarget *TextureView

	// LoadOp specifies what to do at pass start.
	LoadOp gputypes.LoadOp

	// StoreOp specifies what to do at pass end.
	StoreOp gputypes.StoreOp

	// ClearValue is the clear color (used if LoadOp is Clear).
	ClearValue gputypes.Color
}

// RenderPassDepthStencilAttachment describes a depth/stencil attachment.
type RenderPassDepthStencilAttachment struct {
	// View is the texture view to use.
	View *TextureView

	// DepthLoadOp specifies what to do with depth at pass start.
	DepthLoadOp gputypes.LoadOp

	// DepthStoreOp specifies what to do with depth at pass end.
	DepthStoreOp gputypes.StoreOp

	// DepthClearValue is the depth clear value.
	DepthClearValue float32

	// DepthReadOnly makes the depth aspect read-only.
	DepthReadOnly bool

	// StencilLoadOp specifies what to do with stencil at pass start.
	StencilLoadOp gputypes.LoadOp

	// StencilStoreOp specifies what to do with stencil at pass end.
	StencilStoreOp gputypes.StoreOp

	// StencilClearValue is the stencil clear value.
	StencilClearValue uint32

	// StencilReadOnly makes the stencil aspect read-only.
	StencilReadOnly bool
}

// ComputePassDescriptor describes a compute pass.
type ComputePassDescriptor struct {
	// Label is an optional debug name for the compute pass.
	Label string

	// TimestampWrites are timestamp queries to write at pass boundaries (optional).
	TimestampWrites *ComputePassTimestampWrites
}

// ComputePassTimestampWrites describes timestamp query writes for a compute pass.
type ComputePassTimestampWrites struct {
	// QuerySet is the query set to write timestamps to.
	QuerySet QuerySetID

	// BeginningOfPassWriteIndex is the query index for pass start.
	BeginningOfPassWriteIndex *uint32

	// EndOfPassWriteIndex is the query index for pass end.
	EndOfPassWriteIndex *uint32
}

// ImageCopyBuffer describes a buffer for texture copy operations.
type ImageCopyBuffer struct {
	// Buffer is the buffer to copy to/from.
	Buffer *Buffer

	// Layout describes how the data is laid out in the buffer.
	Layout gputypes.TextureDataLayout
}

// ImageCopyTexture describes a texture for copy operations.
type ImageCopyTexture struct {
	// Texture is the texture to copy to/from.
	Texture *GPUTexture

	// MipLevel is the mip level to copy.
	MipLevel uint32

	// Origin is the origin of the copy in the texture.
	Origin gputypes.Origin3D

	// Aspect is the aspect of the texture to copy.
	Aspect gputypes.TextureAspect
}

// Note: TextureView is defined in hal_texture.go with full implementation.
// Note: RenderPassEncoder is defined in render_pass.go with full implementation.
// Note: ComputePassEncoder is defined in compute_pass.go with full implementation.

// =============================================================================
// CoreCommandBuffer
// =============================================================================

// CoreCommandBuffer is a finished command recording ready for submission.
//
// Command buffers are created by CoreCommandEncoder.Finish() and submitted
// to a Queue for execution.
type CoreCommandBuffer struct {
	// label is the debug label.
	label string
}

// Label returns the command buffer's debug label.
func (cb *CoreCommandBuffer) Label() string {
	if cb == nil {
		return ""
	}
	return cb.label
}
