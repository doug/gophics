// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
	"time"
)

// buildWAV constructs a minimal WAV file from the given parameters and raw PCM data.
func buildWAV(format, channels, sampleRate, bitsPerSample int, pcmData []byte) []byte {
	var buf bytes.Buffer

	dataSize := len(pcmData)
	byteRate := sampleRate * channels * (bitsPerSample / 8)
	blockAlign := channels * (bitsPerSample / 8)
	fmtSize := 16

	// RIFF header
	buf.WriteString("RIFF")
	writeLEUint32(&buf, uint32(4+8+fmtSize+8+dataSize)) // file size - 8
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	writeLEUint32(&buf, uint32(fmtSize))
	writeLEUint16(&buf, uint16(format))
	writeLEUint16(&buf, uint16(channels))
	writeLEUint32(&buf, uint32(sampleRate))
	writeLEUint32(&buf, uint32(byteRate))
	writeLEUint16(&buf, uint16(blockAlign))
	writeLEUint16(&buf, uint16(bitsPerSample))

	// data chunk
	buf.WriteString("data")
	writeLEUint32(&buf, uint32(dataSize))
	buf.Write(pcmData)

	return buf.Bytes()
}

func writeLEUint16(buf *bytes.Buffer, v uint16) {
	b := [2]byte{}
	binary.LittleEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func writeLEUint32(buf *bytes.Buffer, v uint32) {
	b := [4]byte{}
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

func TestDecodeWAV_16BitPCM(t *testing.T) {
	// 4 samples of 16-bit mono at 44100 Hz
	pcm := make([]byte, 8)
	binary.LittleEndian.PutUint16(pcm[0:2], 0)              // 0.0
	binary.LittleEndian.PutUint16(pcm[2:4], 0x7FFF)         // ~+1.0 (max positive)
	binary.LittleEndian.PutUint16(pcm[4:6], uint16(0x8000)) // -1.0 (min negative)
	binary.LittleEndian.PutUint16(pcm[6:8], 0x4000)         // ~+0.5

	wav := buildWAV(wavFormatPCM, 1, 44100, 16, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	if dec.SampleRate() != 44100 {
		t.Errorf("SampleRate = %d, want 44100", dec.SampleRate())
	}
	if dec.Channels() != 1 {
		t.Errorf("Channels = %d, want 1", dec.Channels())
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 4 {
		t.Fatalf("got %d samples, want 4", len(samples))
	}

	// sample 0: int16(0)/32768 = 0.0
	assertNear(t, "sample[0]", samples[0], 0.0, 1e-4)
	// sample 1: int16(0x7FFF)/32768 ≈ 0.99997
	assertNear(t, "sample[1]", samples[1], float32(int16(0x7FFF))/32768.0, 1e-4)
	// sample 2: int16(0x8000)/32768 = -1.0
	assertNear(t, "sample[2]", samples[2], -1.0, 1e-4)
	// sample 3: int16(0x4000)/32768 ≈ 0.5
	assertNear(t, "sample[3]", samples[3], float32(int16(0x4000))/32768.0, 1e-4)
}

func TestDecodeWAV_8BitPCM(t *testing.T) {
	// 3 samples of 8-bit mono: silence, max, min
	pcm := []byte{128, 255, 0}
	wav := buildWAV(wavFormatPCM, 1, 22050, 8, pcm)

	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	if dec.SampleRate() != 22050 {
		t.Errorf("SampleRate = %d, want 22050", dec.SampleRate())
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 3 {
		t.Fatalf("got %d samples, want 3", len(samples))
	}

	assertNear(t, "sample[0] (silence)", samples[0], 0.0, 1e-4)
	assertNear(t, "sample[1] (max)", samples[1], (255.0-128.0)/128.0, 1e-4)
	assertNear(t, "sample[2] (min)", samples[2], -128.0/128.0, 1e-4)
}

func TestDecodeWAV_24BitPCM(t *testing.T) {
	// 2 samples: positive and negative
	pcm := make([]byte, 6)
	// +0.5 ≈ 4194304 (0x400000) as 24-bit
	pcm[0] = 0x00
	pcm[1] = 0x00
	pcm[2] = 0x40
	// -0.5 ≈ -4194304 (0xC00000) as 24-bit
	pcm[3] = 0x00
	pcm[4] = 0x00
	pcm[5] = 0xC0

	wav := buildWAV(wavFormatPCM, 1, 48000, 24, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}

	assertNear(t, "sample[0] (+0.5)", samples[0], 0.5, 1e-3)
	assertNear(t, "sample[1] (-0.5)", samples[1], -0.5, 1e-3)
}

func TestDecodeWAV_32BitPCM(t *testing.T) {
	pcm := make([]byte, 8)
	// +0.5 ≈ 1073741824 (0x40000000) as int32
	binary.LittleEndian.PutUint32(pcm[0:4], 0x40000000)
	// -1.0 = 0x80000000 as int32
	binary.LittleEndian.PutUint32(pcm[4:8], 0x80000000)

	wav := buildWAV(wavFormatPCM, 1, 44100, 32, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}

	assertNear(t, "sample[0] (+0.5)", samples[0], 0.5, 1e-3)
	assertNear(t, "sample[1] (-1.0)", samples[1], -1.0, 1e-3)
}

func TestDecodeWAV_32BitFloat(t *testing.T) {
	pcm := make([]byte, 12)
	binary.LittleEndian.PutUint32(pcm[0:4], math.Float32bits(0.0))
	binary.LittleEndian.PutUint32(pcm[4:8], math.Float32bits(0.75))
	binary.LittleEndian.PutUint32(pcm[8:12], math.Float32bits(-0.25))

	wav := buildWAV(wavFormatFloat, 1, 44100, 32, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 3 {
		t.Fatalf("got %d samples, want 3", len(samples))
	}

	assertNear(t, "sample[0]", samples[0], 0.0, 1e-6)
	assertNear(t, "sample[1]", samples[1], 0.75, 1e-6)
	assertNear(t, "sample[2]", samples[2], -0.25, 1e-6)
}

func TestDecodeWAV_Stereo(t *testing.T) {
	// 2 frames of 16-bit stereo: (L, R)
	pcm := make([]byte, 8)
	binary.LittleEndian.PutUint16(pcm[0:2], 0x4000) // L = +0.5
	binary.LittleEndian.PutUint16(pcm[2:4], 0xC000) // R = -0.5
	binary.LittleEndian.PutUint16(pcm[4:6], 0x2000) // L = +0.25
	binary.LittleEndian.PutUint16(pcm[6:8], 0xE000) // R = -0.25

	wav := buildWAV(wavFormatPCM, 2, 44100, 16, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	if dec.Channels() != 2 {
		t.Errorf("Channels = %d, want 2", dec.Channels())
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 4 {
		t.Fatalf("got %d samples, want 4 (2 frames * 2 channels)", len(samples))
	}
}

func TestDecodeWAV_Duration(t *testing.T) {
	// 44100 samples at 44100 Hz mono = 1 second
	numSamples := 44100
	pcm := make([]byte, numSamples*2) // 16-bit
	wav := buildWAV(wavFormatPCM, 1, 44100, 16, pcm)

	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	dur := dec.Duration()
	if dur != time.Second {
		t.Errorf("Duration = %v, want %v", dur, time.Second)
	}
}

func TestDecodeWAV_Reset(t *testing.T) {
	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], 0x4000) // +0.5
	binary.LittleEndian.PutUint16(pcm[2:4], 0x2000) // +0.25

	wav := buildWAV(wavFormatPCM, 1, 44100, 16, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	first := readAllSamples(t, dec)
	dec.Reset()
	second := readAllSamples(t, dec)

	if len(first) != len(second) {
		t.Fatalf("after Reset: got %d samples, want %d", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("after Reset: sample[%d] = %v, want %v", i, second[i], first[i])
		}
	}
}

func TestDecodeWAV_UnknownChunksSkipped(t *testing.T) {
	// Build WAV with an unknown chunk between fmt and data
	var buf bytes.Buffer

	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], 0x4000)
	binary.LittleEndian.PutUint16(pcm[2:4], 0x2000)

	fmtSize := 16
	unknownPayload := []byte{0x01, 0x02, 0x03, 0x04}
	unknownChunkSize := 8 + len(unknownPayload)
	dataSize := len(pcm)
	totalSize := 4 + (8 + fmtSize) + unknownChunkSize + (8 + dataSize)

	// RIFF header
	buf.WriteString("RIFF")
	writeLEUint32(&buf, uint32(totalSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	writeLEUint32(&buf, uint32(fmtSize))
	writeLEUint16(&buf, 1)     // PCM
	writeLEUint16(&buf, 1)     // mono
	writeLEUint32(&buf, 44100) // sample rate
	writeLEUint32(&buf, 88200) // byte rate
	writeLEUint16(&buf, 2)     // block align
	writeLEUint16(&buf, 16)    // bits per sample

	// Unknown chunk (should be skipped)
	buf.WriteString("JUNK")
	writeLEUint32(&buf, uint32(len(unknownPayload)))
	buf.Write(unknownPayload)

	// data chunk
	buf.WriteString("data")
	writeLEUint32(&buf, uint32(dataSize))
	buf.Write(pcm)

	dec, err := DecodeWAV(buf.Bytes())
	if err != nil {
		t.Fatalf("DecodeWAV with unknown chunk: %v", err)
	}

	samples := readAllSamples(t, dec)
	if len(samples) != 2 {
		t.Errorf("got %d samples, want 2", len(samples))
	}
}

func TestDecodeWAV_Errors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"too short", []byte{0, 1, 2}},
		{"missing RIFF", []byte("FOOBxxxxxxxx")},
		{"missing WAVE", append([]byte("RIFF"), append(make([]byte, 4), []byte("XYZW")...)...)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeWAV(tt.data)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestDecodeWAV_UnsupportedFormat(t *testing.T) {
	// Audio format 7 (mu-law) is not supported
	wav := buildWAV(7, 1, 44100, 8, []byte{128})
	_, err := DecodeWAV(wav)
	if err == nil {
		t.Error("expected error for unsupported audio format, got nil")
	}
}

func TestDecodeWAV_UnsupportedBitDepth(t *testing.T) {
	// 12-bit PCM is not supported
	wav := buildWAV(wavFormatPCM, 1, 44100, 12, make([]byte, 3))
	_, err := DecodeWAV(wav)
	if err == nil {
		t.Error("expected error for unsupported bit depth, got nil")
	}
}

func TestDecodeWAV_EmptyRead(t *testing.T) {
	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], 0x4000)
	binary.LittleEndian.PutUint16(pcm[2:4], 0x2000)

	wav := buildWAV(wavFormatPCM, 1, 44100, 16, pcm)
	dec, err := DecodeWAV(wav)
	if err != nil {
		t.Fatalf("DecodeWAV: %v", err)
	}

	// Read with empty buffer should return 0, nil
	n, err := dec.Read(nil)
	if n != 0 || err != nil {
		t.Errorf("Read(nil) = %d, %v; want 0, nil", n, err)
	}
}

// readAllSamples reads all float32 samples from a WAVDecoder.
func readAllSamples(t *testing.T, dec *WAVDecoder) []float32 {
	t.Helper()
	var samples []float32
	buf := make([]byte, 256) // read in small chunks to exercise partial reads
	for {
		n, err := dec.Read(buf)
		for i := 0; i+4 <= n; i += 4 {
			bits := binary.LittleEndian.Uint32(buf[i : i+4])
			samples = append(samples, math.Float32frombits(bits))
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
	return samples
}

func assertNear(t *testing.T, label string, got, want float32, epsilon float64) {
	t.Helper()
	diff := math.Abs(float64(got) - float64(want))
	if diff > epsilon {
		t.Errorf("%s = %v, want %v (diff %v > epsilon %v)", label, got, want, diff, epsilon)
	}
}
