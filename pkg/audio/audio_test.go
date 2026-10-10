package audio

import (
	"math"
	"testing"
)

func TestListDevices(t *testing.T) {
	devices, err := ListDevices()
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}
	if len(devices) == 0 {
		t.Logf("No audio devices found (might be running in headless CI)")
		return
	}
	t.Logf("Discovered %d audio devices", len(devices))
	for _, d := range devices {
		t.Logf("  Device: %s (Loopback: %v)", d.Name, d.IsLoopback)
	}
}

func TestResampleStereo(t *testing.T) {
	inRate := 96000
	outRate := 48000
	numFrames := 960

	// 10ms of 1kHz tone
	input := make([]int16, numFrames*2)
	for i := 0; i < numFrames; i++ {
		val := int16(math.Sin(2*math.Pi*1000.0*float64(i)/float64(inRate)) * 16000.0)
		input[i*2] = val
		input[i*2+1] = val
	}

	resampled := resampleStereo(input, inRate, outRate)
	expectedFrames := 480
	if len(resampled)/2 != expectedFrames {
		t.Fatalf("expected %d resampled frames, got %d", expectedFrames, len(resampled)/2)
	}
}

func TestCapturerBufferPool(t *testing.T) {
	c := NewCapturer()

	b := c.getBuf(100)
	if len(b) != 100 {
		t.Fatalf("getBuf(100) len = %d, want 100", len(b))
	}
	if cap(b) < 100 {
		t.Fatalf("getBuf(100) cap = %d, want >= 100", cap(b))
	}

	// A buffer obtained after releasing one must still be usable.
	c.Release(b)
	b2 := c.getBuf(50)
	if len(b2) != 50 {
		t.Fatalf("getBuf(50) len = %d, want 50", len(b2))
	}

	// Releasing nil or empty buffers must be a safe no-op.
	c.Release(nil)
	c.Release([]int16{})
}
