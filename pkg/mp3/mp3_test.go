package mp3

import (
	"math"
	"testing"
)

func TestStreamEncoder(t *testing.T) {
	bitrates := []int{128, 192, 256, 320}
	sampleRates := []int{44100, 48000}

	for _, sr := range sampleRates {
		for _, br := range bitrates {
			enc, err := NewStreamEncoder(sr, 2, br)
			if err != nil {
				t.Fatalf("failed to create StreamEncoder(sr=%d, br=%d): %v", sr, br, err)
			}

			// Generate 0.5s of 440Hz sine wave (stereo)
			numSamples := sr / 2
			pcm := make([]int16, numSamples*2)
			for i := 0; i < numSamples; i++ {
				val := int16(math.Sin(2*math.Pi*440.0*float64(i)/float64(sr)) * 16000.0)
				pcm[i*2] = val
				pcm[i*2+1] = val
			}

			mp3Data := enc.Encode(pcm)
			if len(mp3Data) == 0 {
				t.Fatalf("expected mp3 data from Encode, got 0 bytes (sr=%d, br=%d)", sr, br)
			}

			flushed := enc.Flush()
			totalBytes := len(mp3Data) + len(flushed)
			if totalBytes < 1000 {
				t.Fatalf("expected at least 1KB mp3 output, got %d bytes", totalBytes)
			}
		}
	}
}

// BenchmarkStreamEncoder exercises the encoding hot path; its CPU profile is
// used as the PGO profile (default.pgo).
func BenchmarkStreamEncoder(b *testing.B) {
	enc, err := NewStreamEncoder(48000, 2, 256)
	if err != nil {
		b.Fatal(err)
	}

	// ~100ms of stereo audio per iteration.
	const frames = 4800
	pcm := make([]int16, frames*2)
	for i := 0; i < frames; i++ {
		val := int16(math.Sin(2*math.Pi*440.0*float64(i)/48000.0) * 16000.0)
		pcm[i*2] = val
		pcm[i*2+1] = val
	}

	b.SetBytes(int64(len(pcm) * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc.Encode(pcm)
	}
}
