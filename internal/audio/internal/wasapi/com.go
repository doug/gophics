// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build windows

package wasapi

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WASAPI COM interface definitions using the same vtable pattern as
// wgpu/hal/dx12/dxgi/. All calls go through syscall.SyscallN for
// zero-CGO operation.

// guid matches the Windows GUID layout.
type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// WASAPI GUIDs.
var (
	clsidMMDeviceEnumerator = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioClient         = guid{0x1CB9AD4C, 0xDBFA, 0x4C32, [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioRenderClient   = guid{0xF294ACFC, 0x3146, 0x4483, [8]byte{0xA7, 0xBF, 0xAD, 0xDC, 0xA7, 0xC2, 0x60, 0xE2}}
)

// COM constants.
const (
	clsctxAll = 0x1 | 0x2 | 0x4 | 0x10 // CLSCTX_INPROC_SERVER | CLSCTX_INPROC_HANDLER | CLSCTX_LOCAL_SERVER | CLSCTX_REMOTE_SERVER

	eRender  = 0 // Audio rendering (playback)
	eConsole = 0 // Games, system notification sounds, voice commands

	audclntSharemodeShared = 0 // AUDCLNT_SHAREMODE_SHARED

	audclntStreamflagsEventcallback     = 0x00040000 // AUDCLNT_STREAMFLAGS_EVENTCALLBACK
	audclntStreamflagsAutoconvertpcm    = 0x80000000 // AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM
	audclntStreamflagsSrcDefaultQuality = 0x08000000 // AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY

	coinitMultithreaded = 0x0 // COINIT_MULTITHREADED
)

// waveFormatEx is the standard Windows audio format descriptor.
type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// DLLs and procedures.
var (
	ole32    = windows.NewLazyDLL("ole32.dll")
	kernel32 = windows.NewLazyDLL("kernel32.dll")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")

	procCreateEventW        = kernel32.NewProc("CreateEventW")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
)

// -----------------------------------------------------------------------
// IMMDeviceEnumerator
// -----------------------------------------------------------------------

type iMMDeviceEnumeratorVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IMMDeviceEnumerator
	EnumAudioEndpoints                     uintptr
	GetDefaultAudioEndpoint                uintptr
	GetDevice                              uintptr
	RegisterEndpointNotificationCallback   uintptr
	UnregisterEndpointNotificationCallback uintptr
}

type iMMDeviceEnumerator struct {
	vtbl *iMMDeviceEnumeratorVtbl
}

// GetDefaultAudioEndpoint retrieves the default audio endpoint for the
// specified data flow direction and role.
func (e *iMMDeviceEnumerator) GetDefaultAudioEndpoint(dataFlow, role uint32) (*iMMDevice, error) {
	var device *iMMDevice
	hr, _, _ := syscall.SyscallN(
		e.vtbl.GetDefaultAudioEndpoint,
		uintptr(unsafe.Pointer(e)),
		uintptr(dataFlow),
		uintptr(role),
		uintptr(unsafe.Pointer(&device)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("IMMDeviceEnumerator.GetDefaultAudioEndpoint: HRESULT 0x%08X", hr)
	}
	return device, nil
}

// Release decrements the reference count.
func (e *iMMDeviceEnumerator) Release() {
	syscall.SyscallN(e.vtbl.Release, uintptr(unsafe.Pointer(e)))
}

// -----------------------------------------------------------------------
// IMMDevice
// -----------------------------------------------------------------------

type iMMDeviceVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IMMDevice
	Activate          uintptr
	OpenPropertyStore uintptr
	GetID             uintptr
	GetState          uintptr
}

type iMMDevice struct {
	vtbl *iMMDeviceVtbl
}

// Activate creates a COM object with the specified interface from this device.
func (d *iMMDevice) Activate(iid *guid, clsCtx uint32) (*iAudioClient, error) {
	var client *iAudioClient
	hr, _, _ := syscall.SyscallN(
		d.vtbl.Activate,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(clsCtx),
		0, // pActivationParams
		uintptr(unsafe.Pointer(&client)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("IMMDevice.Activate: HRESULT 0x%08X", hr)
	}
	return client, nil
}

// Release decrements the reference count.
func (d *iMMDevice) Release() {
	syscall.SyscallN(d.vtbl.Release, uintptr(unsafe.Pointer(d)))
}

// -----------------------------------------------------------------------
// IAudioClient
// -----------------------------------------------------------------------

type iAudioClientVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IAudioClient
	Initialize        uintptr
	GetBufferSize     uintptr
	GetStreamLatency  uintptr
	GetCurrentPadding uintptr
	IsFormatSupported uintptr
	GetMixFormat      uintptr
	GetDevicePeriod   uintptr
	Start             uintptr
	Stop              uintptr
	Reset             uintptr
	SetEventHandle    uintptr
	GetService        uintptr
}

type iAudioClient struct {
	vtbl *iAudioClientVtbl
}

// Initialize sets up the audio stream between the client and the device.
func (c *iAudioClient) Initialize(shareMode, streamFlags uint32, bufferDuration, periodicity int64, format *waveFormatEx) error {
	hr, _, _ := syscall.SyscallN(
		c.vtbl.Initialize,
		uintptr(unsafe.Pointer(c)),
		uintptr(shareMode),
		uintptr(streamFlags),
		uintptr(bufferDuration),
		uintptr(periodicity),
		uintptr(unsafe.Pointer(format)),
		0, // AudioSessionGuid
	)
	if hr != 0 {
		return fmt.Errorf("IAudioClient.Initialize: HRESULT 0x%08X", hr)
	}
	return nil
}

// GetBufferSize returns the size of the audio buffer in frames.
func (c *iAudioClient) GetBufferSize() (uint32, error) {
	var bufferFrames uint32
	hr, _, _ := syscall.SyscallN(
		c.vtbl.GetBufferSize,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(&bufferFrames)),
	)
	if hr != 0 {
		return 0, fmt.Errorf("IAudioClient.GetBufferSize: HRESULT 0x%08X", hr)
	}
	return bufferFrames, nil
}

// GetCurrentPadding returns the number of frames of padding in the buffer.
func (c *iAudioClient) GetCurrentPadding() (uint32, error) {
	var padding uint32
	hr, _, _ := syscall.SyscallN(
		c.vtbl.GetCurrentPadding,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(&padding)),
	)
	if hr != 0 {
		return 0, fmt.Errorf("IAudioClient.GetCurrentPadding: HRESULT 0x%08X", hr)
	}
	return padding, nil
}

// Start begins audio streaming.
func (c *iAudioClient) Start() error {
	hr, _, _ := syscall.SyscallN(c.vtbl.Start, uintptr(unsafe.Pointer(c)))
	if hr != 0 {
		return fmt.Errorf("IAudioClient.Start: HRESULT 0x%08X", hr)
	}
	return nil
}

// Stop pauses audio streaming.
func (c *iAudioClient) Stop() error {
	hr, _, _ := syscall.SyscallN(c.vtbl.Stop, uintptr(unsafe.Pointer(c)))
	if hr != 0 {
		return fmt.Errorf("IAudioClient.Stop: HRESULT 0x%08X", hr)
	}
	return nil
}

// Reset resets the audio stream, clearing the buffer.
func (c *iAudioClient) Reset() error {
	hr, _, _ := syscall.SyscallN(c.vtbl.Reset, uintptr(unsafe.Pointer(c)))
	if hr != 0 {
		return fmt.Errorf("IAudioClient.Reset: HRESULT 0x%08X", hr)
	}
	return nil
}

// SetEventHandle sets the event handle that the system signals when a
// buffer is ready for the client.
func (c *iAudioClient) SetEventHandle(event uintptr) error {
	hr, _, _ := syscall.SyscallN(
		c.vtbl.SetEventHandle,
		uintptr(unsafe.Pointer(c)),
		event,
	)
	if hr != 0 {
		return fmt.Errorf("IAudioClient.SetEventHandle: HRESULT 0x%08X", hr)
	}
	return nil
}

// GetService retrieves a pointer to the requested service interface.
func (c *iAudioClient) GetService(iid *guid) (unsafe.Pointer, error) {
	var service unsafe.Pointer
	hr, _, _ := syscall.SyscallN(
		c.vtbl.GetService,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&service)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("IAudioClient.GetService: HRESULT 0x%08X", hr)
	}
	return service, nil
}

// GetMixFormat retrieves the stream format that the audio engine uses
// for its internal processing of shared-mode streams.
func (c *iAudioClient) GetMixFormat() (*waveFormatEx, error) {
	var format *waveFormatEx
	hr, _, _ := syscall.SyscallN(
		c.vtbl.GetMixFormat,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(&format)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("IAudioClient.GetMixFormat: HRESULT 0x%08X", hr)
	}
	return format, nil
}

// Release decrements the reference count.
func (c *iAudioClient) Release() {
	syscall.SyscallN(c.vtbl.Release, uintptr(unsafe.Pointer(c)))
}

// -----------------------------------------------------------------------
// IAudioRenderClient
// -----------------------------------------------------------------------

type iAudioRenderClientVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IAudioRenderClient
	GetBuffer     uintptr
	ReleaseBuffer uintptr
}

type iAudioRenderClient struct {
	vtbl *iAudioRenderClientVtbl
}

// GetBuffer retrieves a pointer to the next available region of the
// render buffer. The caller must write numFrames worth of data.
func (r *iAudioRenderClient) GetBuffer(numFrames uint32) (unsafe.Pointer, error) {
	var data unsafe.Pointer
	hr, _, _ := syscall.SyscallN(
		r.vtbl.GetBuffer,
		uintptr(unsafe.Pointer(r)),
		uintptr(numFrames),
		uintptr(unsafe.Pointer(&data)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("IAudioRenderClient.GetBuffer: HRESULT 0x%08X", hr)
	}
	return data, nil
}

// ReleaseBuffer releases the buffer acquired by GetBuffer.
// flags can be 0 (data written) or AUDCLNT_BUFFERFLAGS_SILENT (2).
func (r *iAudioRenderClient) ReleaseBuffer(numFrames, flags uint32) error {
	hr, _, _ := syscall.SyscallN(
		r.vtbl.ReleaseBuffer,
		uintptr(unsafe.Pointer(r)),
		uintptr(numFrames),
		uintptr(flags),
	)
	if hr != 0 {
		return fmt.Errorf("IAudioRenderClient.ReleaseBuffer: HRESULT 0x%08X", hr)
	}
	return nil
}

// Release decrements the reference count.
func (r *iAudioRenderClient) Release() {
	syscall.SyscallN(r.vtbl.Release, uintptr(unsafe.Pointer(r)))
}

// -----------------------------------------------------------------------
// COM helper functions
// -----------------------------------------------------------------------

// coInitializeEx initializes the COM library on the current thread.
func coInitializeEx(coinit uint32) error {
	hr, _, _ := procCoInitializeEx.Call(0, uintptr(coinit))
	// S_OK (0) or S_FALSE (1) are both acceptable.
	if hr != 0 && hr != 1 {
		return fmt.Errorf("CoInitializeEx: HRESULT 0x%08X", hr)
	}
	return nil
}

// coCreateInstance creates a COM object.
func coCreateInstance(clsid, iid *guid) (unsafe.Pointer, error) {
	var obj unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsid)),
		0, // pUnkOuter
		clsctxAll,
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&obj)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("CoCreateInstance: HRESULT 0x%08X", hr)
	}
	return obj, nil
}

// coUninitialize uninitializes the COM library on the current thread.
func coUninitialize() {
	procCoUninitialize.Call()
}

// createEvent creates an auto-reset event object.
func createEvent() (uintptr, error) {
	h, _, err := procCreateEventW.Call(0, 0, 0, 0) // auto-reset, non-signaled
	if h == 0 {
		return 0, fmt.Errorf("CreateEventW: %w", err)
	}
	return h, nil
}

// waitForSingleObject waits for an event with a timeout in milliseconds.
func waitForSingleObject(handle uintptr, milliseconds uint32) {
	procWaitForSingleObject.Call(handle, uintptr(milliseconds))
}

// closeHandle closes a Windows handle.
func closeHandle(handle uintptr) {
	procCloseHandle.Call(handle)
}

// referenceTimeFromMs converts milliseconds to REFERENCE_TIME (100ns units).
func referenceTimeFromMs(ms int) int64 {
	return int64(ms) * 10000
}
