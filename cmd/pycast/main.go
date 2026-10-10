package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pycast/pkg/audio"
	"pycast/pkg/mp3"
	"pycast/pkg/presets"
	"pycast/pkg/uvox"
)

type StreamerApp struct {
	mw          *walk.MainWindow
	cbDevice    *walk.ComboBox
	cbPresets   *walk.ComboBox
	btnSave     *walk.PushButton
	btnDel      *walk.PushButton
	leHost      *walk.LineEdit
	lePort      *walk.LineEdit
	leSID       *walk.LineEdit
	leUser      *walk.LineEdit
	lePass      *walk.LineEdit
	cbBitrate   *walk.ComboBox
	lblStatus   *walk.Label
	lblStats    *walk.Label
	btnConnect  *walk.PushButton
	pbL         *walk.ProgressBar
	pbR         *walk.ProgressBar

	devices     []audio.Device
	deviceNames []string
	capturer    *audio.Capturer
	presetMgr   *presets.Manager
	presetNames []string

	isConnected bool
	shouldStop  bool
	liveStart   *time.Time
	bytesSent   int64
	audioChan   chan []int16
	source      *uvox.UvoxSource
}

func main() {
	app := &StreamerApp{
		capturer:  audio.NewCapturer(),
		presetMgr: presets.NewManager("presets.json"),
		audioChan: make(chan []int16, 100),
	}

	app.initDevices()
	app.runUI()
}

func (app *StreamerApp) initDevices() {
	devs, err := audio.ListDevices()
	if err != nil {
		app.devices = nil
		app.deviceNames = []string{"(Error listando dispositivos)"}
		return
	}
	app.devices = devs
	app.deviceNames = make([]string, len(devs))
	for i, d := range devs {
		app.deviceNames[i] = d.Name
	}
}

func (app *StreamerApp) refreshPresetOptions() []string {
	all := app.presetMgr.GetAll()
	opts := []string{"Cargar Preset..."}
	for k := range all {
		opts = append(opts, k)
	}
	opts = append(opts, "Limpiar Datos")
	app.presetNames = opts
	return opts
}

func (app *StreamerApp) runUI() {
	presetOpts := app.refreshPresetOptions()
	bitrateOpts := []string{"128", "192", "256", "320"}

	// Find default device index
	defaultDevIdx := 0
	for i, d := range app.devices {
		if d.IsDefault {
			defaultDevIdx = i
			break
		}
	}

	err := MainWindow{
		AssignTo: &app.mw,
		Title:    "Shoutcast v2 Streamer (Go)",
		MinSize:  Size{Width: 440, Height: 680},
		Size:     Size{Width: 450, Height: 700},
		Layout:   VBox{Margins: Margins{Left: 20, Top: 15, Right: 20, Bottom: 15}, Spacing: 8},
		Children: []Widget{
			// Title
			Label{
				Text: "Shoutcast v2 Streamer",
				Font: Font{PointSize: 14, Bold: true},
			},
			VSpacer{Size: 4},

			// Audio Input Device
			Label{Text: "Dispositivo de Audio:"},
			ComboBox{
				AssignTo:              &app.cbDevice,
				Model:                 app.deviceNames,
				CurrentIndex:          defaultDevIdx,
				OnCurrentIndexChanged: app.onDeviceChanged,
			},

			// Presets
			Label{Text: "Presets de Servidor:"},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 5},
				Children: []Widget{
					ComboBox{
						AssignTo:              &app.cbPresets,
						Model:                 presetOpts,
						CurrentIndex:          0,
						OnCurrentIndexChanged: app.onPresetChanged,
					},
					PushButton{
						AssignTo:  &app.btnSave,
						Text:      "Guardar",
						MinSize:   Size{Width: 65, Height: 0},
						OnClicked: app.onSavePreset,
					},
					PushButton{
						AssignTo:  &app.btnDel,
						Text:      "Borrar",
						MinSize:   Size{Width: 65, Height: 0},
						OnClicked: app.onDeletePreset,
					},
				},
			},
			VSpacer{Size: 4},

			// Server Inputs
			Label{Text: "Hostname or IP:"},
			LineEdit{AssignTo: &app.leHost},

			Label{Text: "Port:"},
			LineEdit{AssignTo: &app.lePort},

			Label{Text: "Stream ID (SID):"},
			LineEdit{AssignTo: &app.leSID},

			Label{Text: "DJ Username:"},
			LineEdit{AssignTo: &app.leUser},

			Label{Text: "Password:"},
			LineEdit{AssignTo: &app.lePass, PasswordMode: true},

			// Bitrate
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: "Bitrate (kbps):"},
					HSpacer{},
					ComboBox{
						AssignTo:     &app.cbBitrate,
						Model:        bitrateOpts,
						CurrentIndex: 2, // 256 kbps default
						MinSize:      Size{Width: 100, Height: 0},
					},
				},
			},
			VSpacer{Size: 6},

			// VU Meter
			Label{Text: "Nivel de Audio (VU Meter):", Font: Font{Bold: true}},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Label{Text: "L", MinSize: Size{Width: 12, Height: 0}},
					ProgressBar{
						AssignTo: &app.pbL,
						MinValue: 0,
						MaxValue: 100,
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Label{Text: "R", MinSize: Size{Width: 12, Height: 0}},
					ProgressBar{
						AssignTo: &app.pbR,
						MinValue: 0,
						MaxValue: 100,
					},
				},
			},
			VSpacer{Size: 8},

			// Status & Stats
			Label{
				AssignTo: &app.lblStatus,
				Text:     "Desconectado",
				Font:     Font{PointSize: 11, Bold: true},
			},
			Label{
				AssignTo: &app.lblStats,
				Text:     "",
				Font:     Font{PointSize: 9},
			},
			VSpacer{Size: 4},

			// Connect Button
			PushButton{
				AssignTo:  &app.btnConnect,
				Text:      "Iniciar Broadcast",
				Font:      Font{PointSize: 11, Bold: true},
				MinSize:   Size{Width: 0, Height: 40},
				OnClicked: app.toggleConnection,
			},
		},
	}.Create()

	if err != nil {
		fmt.Printf("Error creating window: %v\n", err)
		return
	}

	// Start audio capture on initial device
	if len(app.devices) > 0 {
		app.startCapture(app.devices[defaultDevIdx])
	}

	// Periodic stats update
	go app.statsLoop()

	app.mw.Run()
}

func (app *StreamerApp) onDeviceChanged() {
	idx := app.cbDevice.CurrentIndex()
	if idx >= 0 && idx < len(app.devices) {
		app.startCapture(app.devices[idx])
	}
}

func (app *StreamerApp) startCapture(dev audio.Device) {
	_ = app.capturer.Start(dev, func(pcmData []int16, peakL, peakR float32) {
		if peakL >= 0 && app.mw != nil {
			app.mw.Synchronize(func() {
				if app.pbL != nil {
					app.pbL.SetValue(int(peakL * 100))
				}
				if app.pbR != nil {
					app.pbR.SetValue(int(peakR * 100))
				}
			})
		}

		if app.isConnected && len(pcmData) > 0 {
			select {
			case app.audioChan <- pcmData:
			default:
				// Buffer full, drop oldest
			}
		}
	}, func(err error) {
		if app.mw != nil {
			app.mw.Synchronize(func() {
				app.lblStatus.SetText(fmt.Sprintf("Error de captura: %v", err))
			})
		}
	})
}

func (app *StreamerApp) onPresetChanged() {
	idx := app.cbPresets.CurrentIndex()
	if idx < 0 || idx >= len(app.presetNames) {
		return
	}
	choice := app.presetNames[idx]
	if choice == "Limpiar Datos" {
		app.leHost.SetText("")
		app.lePort.SetText("")
		app.leSID.SetText("")
		app.leUser.SetText("")
		app.lePass.SetText("")
		return
	}
	if choice == "Cargar Preset..." {
		return
	}

	p, ok := app.presetMgr.Get(choice)
	if ok {
		app.leHost.SetText(p.Host)
		app.lePort.SetText(p.Port)
		app.leSID.SetText(p.SID)
		app.leUser.SetText(p.User)
		app.lePass.SetText(p.Pass)

		// Set bitrate in dropdown
		for i, br := range []string{"128", "192", "256", "320"} {
			if br == p.Bitrate {
				_ = app.cbBitrate.SetCurrentIndex(i)
				break
			}
		}
	}
}

func (app *StreamerApp) onSavePreset() {
	name, ok := promptDialog(app.mw, "Guardar Preset", "Nombre para este preset:")
	if !ok || name == "" {
		return
	}

	bitrate := "256"
	bIdx := app.cbBitrate.CurrentIndex()
	bOpts := []string{"128", "192", "256", "320"}
	if bIdx >= 0 && bIdx < len(bOpts) {
		bitrate = bOpts[bIdx]
	}

	p := presets.Preset{
		Host:    strings.TrimSpace(app.leHost.Text()),
		Port:    strings.TrimSpace(app.lePort.Text()),
		SID:     strings.TrimSpace(app.leSID.Text()),
		User:    strings.TrimSpace(app.leUser.Text()),
		Pass:    app.lePass.Text(),
		Bitrate: bitrate,
	}

	_ = app.presetMgr.Set(name, p)

	opts := app.refreshPresetOptions()
	_ = app.cbPresets.SetModel(opts)
	for i, o := range opts {
		if o == name {
			_ = app.cbPresets.SetCurrentIndex(i)
			break
		}
	}
}

func (app *StreamerApp) onDeletePreset() {
	idx := app.cbPresets.CurrentIndex()
	if idx < 0 || idx >= len(app.presetNames) {
		return
	}
	name := app.presetNames[idx]
	if name == "Cargar Preset..." || name == "Limpiar Datos" {
		return
	}

	_ = app.presetMgr.Delete(name)
	opts := app.refreshPresetOptions()
	_ = app.cbPresets.SetModel(opts)
	_ = app.cbPresets.SetCurrentIndex(0)
}

func (app *StreamerApp) toggleConnection() {
	if app.isConnected {
		app.stopStream()
	} else {
		app.startStream()
	}
}

func (app *StreamerApp) startStream() {
	host := strings.TrimSpace(app.leHost.Text())
	portStr := strings.TrimSpace(app.lePort.Text())
	sidStr := strings.TrimSpace(app.leSID.Text())
	user := strings.TrimSpace(app.leUser.Text())
	pass := app.lePass.Text()

	port, errP := strconv.Atoi(portStr)
	sid, errS := strconv.Atoi(sidStr)

	if host == "" || errP != nil || errS != nil || pass == "" {
		app.lblStatus.SetText("Error: Faltan campos")
		return
	}

	bitrate := 256
	bIdx := app.cbBitrate.CurrentIndex()
	bOpts := []int{128, 192, 256, 320}
	if bIdx >= 0 && bIdx < len(bOpts) {
		bitrate = bOpts[bIdx]
	}

	app.lblStatus.SetText("Conectando...")
	app.cbBitrate.SetEnabled(false)
	app.btnConnect.SetText("Conectando...")
	app.btnConnect.SetEnabled(false)

	app.shouldStop = false

	go func() {
		cfg := uvox.Config{
			Host:     host,
			Port:     port,
			SID:      sid,
			User:     user,
			Password: pass,
			Bitrate:  bitrate,
			Name:     "PyStreamer",
			Genre:    "Variado",
		}
		source := uvox.NewUvoxSource(cfg)

		if err := source.Connect(8 * time.Second); err != nil {
			app.mw.Synchronize(func() {
				app.lblStatus.SetText(fmt.Sprintf("Error: %v", err))
				app.resetBtn()
			})
			return
		}

		app.source = source
		encoder, err := mp3.NewStreamEncoder(app.capturer.TargetSampleRate(), 2, bitrate)
		if err != nil {
			source.Close(false)
			app.mw.Synchronize(func() {
				app.lblStatus.SetText(fmt.Sprintf("Error encoder: %v", err))
				app.resetBtn()
			})
			return
		}

		app.mw.Synchronize(func() {
			app.isConnected = true
			now := time.Now()
			app.liveStart = &now
			atomic.StoreInt64(&app.bytesSent, 0)
			app.lblStatus.SetText(fmt.Sprintf("En Vivo (MP3 @ %d kbps)", bitrate))
			app.btnConnect.SetText("Detener Broadcast")
			app.btnConnect.SetEnabled(true)
		})

		// Drain any leftover buffered packets
		for len(app.audioChan) > 0 {
			<-app.audioChan
		}

		// 0.5s silence frame
		silentPCM := make([]int16, (app.capturer.TargetSampleRate()/2)*2)

		for !app.shouldStop && app.isConnected {
			var pcmData []int16
			select {
			case data := <-app.audioChan:
				pcmData = data
			case <-time.After(500 * time.Millisecond):
				pcmData = silentPCM
			}

			if app.shouldStop {
				break
			}

			mp3Bytes := encoder.Encode(pcmData)
			if len(mp3Bytes) > 0 {
				if err := source.SendAudio(mp3Bytes); err != nil {
					if !app.shouldStop {
						app.mw.Synchronize(func() {
							app.lblStatus.SetText("Conexión perdida")
							app.resetBtn()
						})
					}
					break
				}
				atomic.AddInt64(&app.bytesSent, int64(len(mp3Bytes)))
			}
		}

		// Flush final data
		flushed := encoder.Flush()
		if len(flushed) > 0 {
			_ = source.SendAudio(flushed)
			atomic.AddInt64(&app.bytesSent, int64(len(flushed)))
		}

		source.Close(true)
		app.mw.Synchronize(func() {
			app.resetBtn()
		})
	}()
}

func (app *StreamerApp) stopStream() {
	app.shouldStop = true
	app.isConnected = false
	if app.source != nil {
		app.source.Close(true)
	}
	app.resetBtn()
}

func (app *StreamerApp) resetBtn() {
	app.isConnected = false
	app.liveStart = nil
	app.btnConnect.SetText("Iniciar Broadcast")
	app.btnConnect.SetEnabled(true)
	app.cbBitrate.SetEnabled(true)
	if app.lblStatus.Text() == "En Vivo" || app.lblStatus.Text() == "Conectando..." {
		app.lblStatus.SetText("Desconectado")
	}
	app.lblStats.SetText("")
}

func (app *StreamerApp) statsLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if app.isConnected && app.liveStart != nil && app.mw != nil {
			elapsed := int(time.Since(*app.liveStart).Seconds())
			h := elapsed / 3600
			m := (elapsed % 3600) / 60
			s := elapsed % 60
			mb := float64(atomic.LoadInt64(&app.bytesSent)) / (1024.0 * 1024.0)

			app.mw.Synchronize(func() {
				if app.lblStats != nil {
					app.lblStats.SetText(fmt.Sprintf("Al aire %02d:%02d:%02d  ·  %.1f MB enviados", h, m, s, mb))
				}
			})
		}
	}
}

// promptDialog shows a simple input dialog.
func promptDialog(owner walk.Form, title, message string) (string, bool) {
	var dlg *walk.Dialog
	var le *walk.LineEdit
	var acceptPB, cancelPB *walk.PushButton
	accepted := false

	_, _ = Dialog{
		AssignTo:      &dlg,
		Title:         title,
		MinSize:       Size{Width: 320, Height: 130},
		Layout:        VBox{Margins: Margins{Left: 15, Top: 15, Right: 15, Bottom: 15}, Spacing: 10},
		DefaultButton: &acceptPB,
		CancelButton:  &cancelPB,
		Children: []Widget{
			Label{Text: message},
			LineEdit{AssignTo: &le},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 5},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo:  &acceptPB,
						Text:      "OK",
						OnClicked: func() { accepted = true; dlg.Accept() },
					},
					PushButton{
						AssignTo:  &cancelPB,
						Text:      "Cancelar",
						OnClicked: func() { dlg.Cancel() },
					},
				},
			},
		},
	}.Run(owner)

	if accepted {
		return strings.TrimSpace(le.Text()), true
	}
	return "", false
}
