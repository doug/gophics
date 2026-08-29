package wav

import (
	"encoding/binary"
	"errors"
)

// Encode encodes mono 16-bit PCM samples as a RIFF/PCM WAV file. It is the
// portable audio-clip format the media shell standardizes on, so a Clip
// recorded on one platform plays on any other.
func Encode(pcm []int16, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	const channels, bits = 1, 16
	dataLen := len(pcm) * 2
	buf := make([]byte, 44+dataLen)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataLen))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16) // PCM fmt chunk size
	binary.LittleEndian.PutUint16(buf[20:], 1)  // audio format: PCM
	binary.LittleEndian.PutUint16(buf[22:], channels)
	binary.LittleEndian.PutUint32(buf[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(sampleRate*channels*bits/8)) // byte rate
	binary.LittleEndian.PutUint16(buf[32:], channels*bits/8)                    // block align
	binary.LittleEndian.PutUint16(buf[34:], bits)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataLen))
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(s))
	}
	return buf
}

// Decode parses a 16-bit PCM WAV, returning mono samples (stereo is
// downmixed) and the sample rate. Extra RIFF chunks are skipped.
func Decode(b []byte) (pcm []int16, sampleRate int, err error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, 0, errors.New("wav: not a RIFF/WAVE file")
	}
	var channels, bits int
	var data []byte
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		sz := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if body+sz > len(b) {
			sz = len(b) - body
		}
		switch id {
		case "fmt ":
			if sz >= 16 {
				channels = int(binary.LittleEndian.Uint16(b[body+2:]))
				sampleRate = int(binary.LittleEndian.Uint32(b[body+4:]))
				bits = int(binary.LittleEndian.Uint16(b[body+14:]))
			}
		case "data":
			data = b[body : body+sz]
		}
		off = body + sz + (sz & 1) // chunks are word-aligned
	}
	if bits != 16 || channels < 1 {
		return nil, 0, errors.New("wav: unsupported (want 16-bit PCM)")
	}
	// A rate of zero decoded as success for a long time, because nothing here
	// looked at it: the fmt chunk was accepted on its bit depth and channel
	// count alone. Every caller then divides by it to get a duration, so each
	// one had to remember a guard, and the one that forgot would divide by
	// zero on a file it did not write. Rejecting it here is the fix that does
	// not depend on remembering. Found by FuzzDecodeWAV in two seconds.
	if sampleRate <= 0 {
		return nil, 0, errors.New("wav: the file declares no sample rate")
	}
	n := len(data) / 2
	raw := make([]int16, n)
	for i := range n {
		raw[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	if channels == 1 {
		return raw, sampleRate, nil
	}
	frames := n / channels
	mono := make([]int16, frames)
	for i := range frames {
		var sum int
		for c := 0; c < channels; c++ {
			sum += int(raw[i*channels+c])
		}
		mono[i] = int16(sum / channels)
	}
	return mono, sampleRate, nil
}

// Envelope downsamples |pcm| into at most buckets peak values in 0..1, for the
// waveform an app draws beside a recording.
//
// It lives here, beside Encode, because every backend that produces a Clip
// needs it and they must agree: a waveform computed one way on desktop and
// another on Android would make the same recording look different depending on
// where it was made.
func Envelope(pcm []int16, buckets int) []float32 {
	if len(pcm) == 0 || buckets <= 0 {
		return nil
	}
	if buckets > len(pcm) {
		buckets = len(pcm)
	}
	out := make([]float32, buckets)
	per := max(len(pcm)/buckets, 1)
	for i := range out {
		var peak int
		start := i * per
		for j := start; j < start+per && j < len(pcm); j++ {
			v := int(pcm[j])
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		out[i] = float32(peak) / 32768
	}
	return out
}
