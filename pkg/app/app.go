package app

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"pycast/pkg/audio"
	"pycast/pkg/config"
	"pycast/pkg/i18n"
	"pycast/pkg/mp3"
	"pycast/pkg/presets"
	"pycast/pkg/uvox"
)

type connState int

const (
	stateDisconnected connState = iota
	stateConnecting
	stateLive
)

// languages lists the selectable languages with their native display names.
var languages = []struct {
	code  string
	label string
}{
	{i18n.English, "English"},
	{i18n.Spanish, "Español"},
}

func languageLabels() []string {
	labels := make([]string, len(languages))
	for i, l := range languages {
		labels[i] = l.label
	}
	return labels
}

func currentLanguageIndex() int {
	for i, l := range languages {
		if l.code == i18n.Current() {
			return i
		}
	}
	return 0
}

type streamerApp struct {
	mw *walk.MainWindow

	lblHeading *walk.Label
	lblLang    *walk.Label
	cbLang     *walk.ComboBox
	lblDevice  *walk.Label
	cbDevice   *walk.ComboBox
	lblPresets *walk.Label
	cbPresets  *walk.ComboBox
	btnSave    *walk.PushButton
	btnDel     *walk.PushButton
	lblHost    *walk.Label
	leHost     *walk.LineEdit
	lblPort    *walk.Label
	lePort     *walk.LineEdit
	lblSID     *walk.Label
	leSID      *walk.LineEdit
	lblUser    *walk.Label
	leUser     *walk.LineEdit
	lblPass    *walk.Label
	lePass     *walk.LineEdit
	lblBitrate *walk.Label
	cbBitrate  *walk.ComboBox
	lblVU      *walk.Label
	lblStatus  *walk.Label
	lblStats   *walk.Label
	btnConnect *walk.PushButton
	pbL        *walk.ProgressBar
	pbR        *walk.ProgressBar

	devices     []audio.Device
	deviceNames []string
	capturer    *audio.Capturer
	presetMgr   *presets.Manager
	cfgMgr      *config.Manager
	presetNames []string

	state      connState
	bitrate    int
	shouldStop bool
	liveStart  *time.Time
	bytesSent  int64
	audioChan  chan []int16
	source     *uvox.UvoxSource
}

// Run builds the UI and blocks on the message loop. English is the default
// language; a saved preference (config.json) overrides it.
func Run() {
	cfgMgr := config.NewManager("config.json")
	i18n.SetLanguage(cfgMgr.Language())

	app := &streamerApp{
		capturer:  audio.NewCapturer(),
		presetMgr: presets.NewManager("presets.json"),
		cfgMgr:    cfgMgr,
		audioChan: make(chan []int16, 100),
		bitrate:   256,
	}

	app.initDevices()
	app.runUI()
}

func (app *streamerApp) initDevices() {
	devs, err := audio.ListDevices()
	if err != nil {
		app.devices = nil
		app.deviceNames = []string{i18n.T("device.list_error")}
		return
	}
	app.devices = devs
	app.deviceNames = make([]string, len(devs))
	for i, d := range devs {
		app.deviceNames[i] = d.Name
	}
}

// refreshPresetOptions rebuilds the preset dropdown model. Index 0 is a
// placeholder and the last index clears the fields; both are matched by
// position so translated labels never break the logic.
func (app *streamerApp) refreshPresetOptions() []string {
	all := app.presetMgr.GetAll()
	opts := []string{i18n.T("preset.load")}
	for k := range all {
		opts = append(opts, k)
	}
	opts = append(opts, i18n.T("preset.clear"))
	app.presetNames = opts
	return opts
}

func (app *streamerApp) runUI() {
	presetOpts := app.refreshPresetOptions()
	bitrateOpts := []string{"128", "192", "256", "320"}

	defaultDevIdx := 0
	for i, d := range app.devices {
		if d.IsDefault {
			defaultDevIdx = i
			break
		}
	}

	err := MainWindow{
		AssignTo: &app.mw,
		Title:    i18n.T("app.title"),
		MinSize:  Size{Width: 440, Height: 720},
		Size:     Size{Width: 450, Height: 740},
		Layout:   VBox{Margins: Margins{Left: 20, Top: 15, Right: 20, Bottom: 15}, Spacing: 8},
		Children: []Widget{
			// Language selector
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{AssignTo: &app.lblLang, Text: i18n.T("label.language")},
					HSpacer{},
					ComboBox{
						AssignTo:     &app.cbLang,
						Model:        languageLabels(),
						CurrentIndex: currentLanguageIndex(),
						MinSize:      Size{Width: 120, Height: 0},
					},
				},
			},

			// Title
			Label{
				AssignTo: &app.lblHeading,
				Text:     i18n.T("app.heading"),
				Font:     Font{PointSize: 14, Bold: true},
			},
			VSpacer{Size: 4},

			// Audio Input Device
			Label{AssignTo: &app.lblDevice, Text: i18n.T("label.audio_device")},
			ComboBox{
				AssignTo:              &app.cbDevice,
				Model:                 app.deviceNames,
				CurrentIndex:          defaultDevIdx,
				OnCurrentIndexChanged: app.onDeviceChanged,
			},

			// Presets
			Label{AssignTo: &app.lblPresets, Text: i18n.T("label.server_presets")},
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
						Text:      i18n.T("btn.save"),
						MinSize:   Size{Width: 65, Height: 0},
						OnClicked: app.onSavePreset,
					},
					PushButton{
						AssignTo:  &app.btnDel,
						Text:      i18n.T("btn.delete"),
						MinSize:   Size{Width: 65, Height: 0},
						OnClicked: app.onDeletePreset,
					},
				},
			},
			VSpacer{Size: 4},

			// Server Inputs
			Label{AssignTo: &app.lblHost, Text: i18n.T("label.host")},
			LineEdit{AssignTo: &app.leHost},

			Label{AssignTo: &app.lblPort, Text: i18n.T("label.port")},
			LineEdit{AssignTo: &app.lePort},

			Label{AssignTo: &app.lblSID, Text: i18n.T("label.sid")},
			LineEdit{AssignTo: &app.leSID},

			Label{AssignTo: &app.lblUser, Text: i18n.T("label.username")},
			LineEdit{AssignTo: &app.leUser},

			Label{AssignTo: &app.lblPass, Text: i18n.T("label.password")},
			LineEdit{AssignTo: &app.lePass, PasswordMode: true},

			// Bitrate
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{AssignTo: &app.lblBitrate, Text: i18n.T("label.bitrate")},
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
			Label{AssignTo: &app.lblVU, Text: i18n.T("label.vu_meter"), Font: Font{Bold: true}},
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
				Text:     i18n.T("status.disconnected"),
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
				Text:      i18n.T("btn.start"),
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

	// Attach after creation so it never fires while widgets are still nil.
	app.cbLang.CurrentIndexChanged().Attach(app.onLanguageChanged)

	if len(app.devices) > 0 {
		app.startCapture(app.devices[defaultDevIdx])
	}

	go app.statsLoop()

	app.mw.Run()
}

// applyLanguage re-renders every translated text in the active language.
func (app *streamerApp) applyLanguage() {
	_ = app.mw.SetTitle(i18n.T("app.title"))
	app.lblHeading.SetText(i18n.T("app.heading"))
	app.lblLang.SetText(i18n.T("label.language"))
	app.lblDevice.SetText(i18n.T("label.audio_device"))
	app.lblPresets.SetText(i18n.T("label.server_presets"))
	app.btnSave.SetText(i18n.T("btn.save"))
	app.btnDel.SetText(i18n.T("btn.delete"))
	app.lblHost.SetText(i18n.T("label.host"))
	app.lblPort.SetText(i18n.T("label.port"))
	app.lblSID.SetText(i18n.T("label.sid"))
	app.lblUser.SetText(i18n.T("label.username"))
	app.lblPass.SetText(i18n.T("label.password"))
	app.lblBitrate.SetText(i18n.T("label.bitrate"))
	app.lblVU.SetText(i18n.T("label.vu_meter"))

	_ = app.cbPresets.SetModel(app.refreshPresetOptions())
	_ = app.cbPresets.SetCurrentIndex(0)

	app.updateStatusText()
}

func (app *streamerApp) onLanguageChanged() {
	idx := app.cbLang.CurrentIndex()
	if idx < 0 || idx >= len(languages) {
		return
	}
	i18n.SetLanguage(languages[idx].code)
	_ = app.cfgMgr.SetLanguage(languages[idx].code)
	app.applyLanguage()
}

// updateStatusText syncs the status label and connect button with the current
// connection state, using the active language.
func (app *streamerApp) updateStatusText() {
	switch app.state {
	case stateConnecting:
		app.lblStatus.SetText(i18n.T("status.connecting"))
		app.btnConnect.SetText(i18n.T("status.connecting"))
	case stateLive:
		app.lblStatus.SetText(fmt.Sprintf(i18n.T("status.live"), app.bitrate))
		app.btnConnect.SetText(i18n.T("btn.stop"))
	default:
		app.lblStatus.SetText(i18n.T("status.disconnected"))
		app.btnConnect.SetText(i18n.T("btn.start"))
	}
}

func (app *streamerApp) setError(err error) {
	app.lblStatus.SetText(fmt.Sprintf("%s: %s", i18n.T("status.error"), i18n.Message(err)))
}

func (app *streamerApp) onDeviceChanged() {
	idx := app.cbDevice.CurrentIndex()
	if idx >= 0 && idx < len(app.devices) {
		app.startCapture(app.devices[idx])
	}
}

func (app *streamerApp) startCapture(dev audio.Device) {
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

		if app.state == stateLive && len(pcmData) > 0 {
			select {
			case app.audioChan <- pcmData:
			default:
				// Buffer full, drop oldest
			}
		}
	}, func(err error) {
		if app.mw != nil {
			app.mw.Synchronize(func() {
				app.lblStatus.SetText(fmt.Sprintf("%s: %s", i18n.T("status.capture_error"), i18n.Message(err)))
			})
		}
	})
}

func (app *streamerApp) clearFields() {
	app.leHost.SetText("")
	app.lePort.SetText("")
	app.leSID.SetText("")
	app.leUser.SetText("")
	app.lePass.SetText("")
}

func (app *streamerApp) onPresetChanged() {
	idx := app.cbPresets.CurrentIndex()
	if idx < 0 || idx >= len(app.presetNames) {
		return
	}
	switch {
	case idx == 0:
		return // "Load Preset..." placeholder
	case idx == len(app.presetNames)-1:
		app.clearFields() // "Clear Fields"
		return
	}

	p, ok := app.presetMgr.Get(app.presetNames[idx])
	if !ok {
		return
	}
	app.leHost.SetText(p.Host)
	app.lePort.SetText(p.Port)
	app.leSID.SetText(p.SID)
	app.leUser.SetText(p.User)
	app.lePass.SetText(p.Pass)

	for i, br := range []string{"128", "192", "256", "320"} {
		if br == p.Bitrate {
			_ = app.cbBitrate.SetCurrentIndex(i)
			break
		}
	}
}

func (app *streamerApp) onSavePreset() {
	name, ok := promptDialog(app.mw, i18n.T("dialog.save_preset.title"), i18n.T("dialog.save_preset.message"))
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

func (app *streamerApp) onDeletePreset() {
	idx := app.cbPresets.CurrentIndex()
	if idx <= 0 || idx >= len(app.presetNames)-1 {
		return
	}

	_ = app.presetMgr.Delete(app.presetNames[idx])
	_ = app.cbPresets.SetModel(app.refreshPresetOptions())
	_ = app.cbPresets.SetCurrentIndex(0)
}

func (app *streamerApp) toggleConnection() {
	if app.state == stateLive {
		app.stopStream()
	} else {
		app.startStream()
	}
}

func (app *streamerApp) startStream() {
	host := strings.TrimSpace(app.leHost.Text())
	portStr := strings.TrimSpace(app.lePort.Text())
	sidStr := strings.TrimSpace(app.leSID.Text())
	user := strings.TrimSpace(app.leUser.Text())
	pass := app.lePass.Text()

	port, errP := strconv.Atoi(portStr)
	sid, errS := strconv.Atoi(sidStr)

	if host == "" || errP != nil || errS != nil || pass == "" {
		app.lblStatus.SetText(i18n.T("status.missing_fields"))
		return
	}

	bitrate := 256
	bIdx := app.cbBitrate.CurrentIndex()
	bOpts := []int{128, 192, 256, 320}
	if bIdx >= 0 && bIdx < len(bOpts) {
		bitrate = bOpts[bIdx]
	}

	app.state = stateConnecting
	app.bitrate = bitrate
	app.lblStatus.SetText(i18n.T("status.connecting"))
	app.cbBitrate.SetEnabled(false)
	app.btnConnect.SetText(i18n.T("status.connecting"))
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
				app.setError(err)
				app.resetBtn()
			})
			return
		}

		app.source = source
		encoder, err := mp3.NewStreamEncoder(app.capturer.TargetSampleRate(), 2, bitrate)
		if err != nil {
			source.Close(false)
			app.mw.Synchronize(func() {
				app.setError(err)
				app.resetBtn()
			})
			return
		}

		app.mw.Synchronize(func() {
			app.state = stateLive
			now := time.Now()
			app.liveStart = &now
			atomic.StoreInt64(&app.bytesSent, 0)
			app.updateStatusText()
			app.btnConnect.SetEnabled(true)
		})

		// Drain any leftover buffered packets
		for len(app.audioChan) > 0 {
			<-app.audioChan
		}

		// 0.5s silence frame
		silentPCM := make([]int16, (app.capturer.TargetSampleRate()/2)*2)

		for !app.shouldStop && app.state == stateLive {
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
							app.lblStatus.SetText(i18n.T("status.connection_lost"))
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

func (app *streamerApp) stopStream() {
	app.shouldStop = true
	app.state = stateDisconnected
	if app.source != nil {
		app.source.Close(true)
	}
	app.resetBtn()
}

func (app *streamerApp) resetBtn() {
	app.state = stateDisconnected
	app.liveStart = nil
	app.btnConnect.SetEnabled(true)
	app.cbBitrate.SetEnabled(true)
	app.lblStats.SetText("")
	app.updateStatusText()
}

func (app *streamerApp) statsLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if app.state == stateLive && app.liveStart != nil && app.mw != nil {
			elapsed := int(time.Since(*app.liveStart).Seconds())
			h := elapsed / 3600
			m := (elapsed % 3600) / 60
			s := elapsed % 60
			mb := float64(atomic.LoadInt64(&app.bytesSent)) / (1024.0 * 1024.0)

			app.mw.Synchronize(func() {
				if app.lblStats != nil {
					app.lblStats.SetText(fmt.Sprintf(i18n.T("status.stats"), h, m, s, mb))
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
						Text:      i18n.T("btn.cancel"),
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
