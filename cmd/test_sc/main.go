package main

import (
	"fmt"
	"time"

	"pycast/pkg/audio"
)

func main() {
	fmt.Println("Buscando dispositivos de audio...")
	devices, err := audio.ListDevices()
	if err != nil {
		fmt.Printf("Error listando dispositivos: %v\n", err)
		return
	}

	var selected audio.Device
	var found bool
	for _, d := range devices {
		if d.IsDefault {
			selected = d
			found = true
			break
		}
	}
	if !found && len(devices) > 0 {
		selected = devices[0]
	}

	fmt.Printf("Dispositivo seleccionado: %s (Loopback: %v, Default: %v)\n", selected.Name, selected.IsLoopback, selected.IsDefault)
	fmt.Println("Grabando prueba durante 2 segundos...")

	capturer := audio.NewCapturer()
	framesCount := 0

	err = capturer.Start(selected, func(pcmData []int16, peakL, peakR float32) {
		if len(pcmData) > 0 {
			framesCount += len(pcmData) / 2
		}
		if peakL >= 0 {
			fmt.Printf("\rNivel L: %.2f | R: %.2f | Muestras grabadas: %d   ", peakL, peakR, framesCount)
		}
	}, func(err error) {
		fmt.Printf("\nError de captura: %v\n", err)
	})

	if err != nil {
		fmt.Printf("Error al iniciar captura: %v\n", err)
		return
	}

	time.Sleep(2 * time.Second)
	capturer.Stop()

	fmt.Printf("\n¡Prueba completada con éxito! Total de cuadros de audio: %d\n", framesCount)
}
