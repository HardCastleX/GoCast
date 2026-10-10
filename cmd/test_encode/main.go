package main

import (
	"fmt"
	"os"
	"time"

	"pycast/pkg/audio"
	"pycast/pkg/mp3"
)

func main() {
	fmt.Println("Testing audio capture and MP3 encoding in Go...")

	devices, err := audio.ListDevices()
	if err != nil {
		fmt.Printf("Error listing devices: %v\n", err)
		return
	}

	var selectedDev audio.Device
	var found bool
	for _, d := range devices {
		if d.IsDefault {
			selectedDev = d
			found = true
			break
		}
	}
	if !found {
		for _, d := range devices {
			if d.IsLoopback {
				selectedDev = d
				found = true
				break
			}
		}
	}
	if !found && len(devices) > 0 {
		selectedDev = devices[0]
	}

	fmt.Printf("Capturing from: %s (DataFlow: %d, Index: %d, Default: %v)\n", selectedDev.Name, selectedDev.DataFlow, selectedDev.Index, selectedDev.IsDefault)

	capturer := audio.NewCapturer()
	sampleRate := 48000
	bitrate := 128

	encoder, err := mp3.NewStreamEncoder(sampleRate, 2, bitrate)
	if err != nil {
		fmt.Printf("Error creating MP3 encoder: %v\n", err)
		return
	}

	outFile, err := os.Create("test_output.mp3")
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		return
	}
	defer outFile.Close()

	var totalMP3Bytes int
	dataCount := 0

	err = capturer.Start(selectedDev, func(pcmData []int16, peakL, peakR float32) {
		if len(pcmData) > 0 {
			mp3Bytes := encoder.Encode(pcmData)
			if len(mp3Bytes) > 0 {
				_, _ = outFile.Write(mp3Bytes)
				totalMP3Bytes += len(mp3Bytes)
			}
			dataCount++
		}
		if peakL >= 0 {
			fmt.Printf("\rLevel: L: %.2f | R: %.2f | Encoded MP3: %d bytes   ", peakL, peakR, totalMP3Bytes)
		}
	}, func(err error) {
		fmt.Printf("\nAudio capture error: %v\n", err)
	})

	if err != nil {
		fmt.Printf("Error starting capture: %v\n", err)
		return
	}

	// Capture for 2 seconds
	time.Sleep(2 * time.Second)
	capturer.Stop()

	flushed := encoder.Flush()
	if len(flushed) > 0 {
		_, _ = outFile.Write(flushed)
		totalMP3Bytes += len(flushed)
	}

	fmt.Printf("\nDone! Captured and encoded %d bytes of MP3.\n", totalMP3Bytes)
	_ = os.Remove("test_output.mp3")
}
