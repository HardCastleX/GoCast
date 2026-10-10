package audio

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"

	"pycast/pkg/i18n"
)

const (
	FormatPCM        uint16 = 0x0001
	FormatIEEEFloat  uint16 = 0x0003
	FormatExtensible uint16 = 0xFFFE

	// captureWaitMs bounds how long the capture loop blocks on the WASAPI event.
	captureWaitMs uint32 = 100
)

// Device represents an available audio input/output device.
type Device struct {
	ID         string
	Name       string
	IsLoopback bool
	IsDefault  bool
	DataFlow   uint32
	Index      uint32
}

// Callback is called with stereo 16-bit PCM samples and peak levels (0.0 - 1.0).
type Callback func(pcmData []int16, peakL, peakR float32)

// ListDevices returns all available WASAPI rendering (loopback) and capture devices.
func ListDevices() ([]Device, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return nil, err
	}
	defer ole.CoUninitialize()

	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		return nil, err
	}
	defer de.Release()

	// Get default render device name to mark it as default loopback
	var defaultRenderName string
	var defRender *wca.IMMDevice
	if err := de.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defRender); err == nil {
		var ps *wca.IPropertyStore
		if err := defRender.OpenPropertyStore(wca.STGM_READ, &ps); err == nil {
			var pv wca.PROPVARIANT
			if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err == nil {
				defaultRenderName = pv.String()
			}
			ps.Release()
		}
		defRender.Release()
	}

	var devices []Device

	// 1. Render endpoints for Loopback
	var dcRender *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &dcRender); err == nil {
		var count uint32
		_ = dcRender.GetCount(&count)
		for i := uint32(0); i < count; i++ {
			var dev *wca.IMMDevice
			if err := dcRender.Item(i, &dev); err != nil {
				continue
			}
			var ps *wca.IPropertyStore
			if err := dev.OpenPropertyStore(wca.STGM_READ, &ps); err == nil {
				var pv wca.PROPVARIANT
				if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err == nil {
					name := pv.String()
					devices = append(devices, Device{
						ID:         fmt.Sprintf("render_%d", i),
						Name:       fmt.Sprintf("%s [Loopback]", name),
						IsLoopback: true,
						IsDefault:  (name == defaultRenderName),
						DataFlow:   wca.ERender,
						Index:      i,
					})
				}
				ps.Release()
			}
			dev.Release()
		}
		dcRender.Release()
	}

	// 2. Capture endpoints for Microphones / Line-in
	var dcCapture *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE, &dcCapture); err == nil {
		var count uint32
		_ = dcCapture.GetCount(&count)
		for i := uint32(0); i < count; i++ {
			var dev *wca.IMMDevice
			if err := dcCapture.Item(i, &dev); err != nil {
				continue
			}
			var ps *wca.IPropertyStore
			if err := dev.OpenPropertyStore(wca.STGM_READ, &ps); err == nil {
				var pv wca.PROPVARIANT
				if err := ps.GetValue(&wca.PKEY_Device_FriendlyName, &pv); err == nil {
					name := pv.String()
					devices = append(devices, Device{
						ID:         fmt.Sprintf("capture_%d", i),
						Name:       name,
						IsLoopback: false,
						DataFlow:   wca.ECapture,
						Index:      i,
					})
				}
				ps.Release()
			}
			dev.Release()
		}
		dcCapture.Release()
	}

	return devices, nil
}

// Capturer handles background WASAPI audio capture and PCM processing.
type Capturer struct {
	mu         sync.Mutex
	cancelFunc context.CancelFunc
	running    bool
	targetRate int

	pool sync.Pool
}

// NewCapturer creates a new Capturer.
func NewCapturer() *Capturer {
	c := &Capturer{targetRate: 48000}
	c.pool.New = func() any { return make([]int16, 0, 4096) }
	return c
}

// getBuf returns a stereo PCM buffer of length n, reusing memory from the pool.
func (c *Capturer) getBuf(n int) []int16 {
	if v := c.pool.Get(); v != nil {
		if b := v.([]int16); cap(b) >= n {
			return b[:n]
		}
	}
	return make([]int16, n)
}

// Release returns a buffer obtained from the Capturer to the pool. The buffer
// must not be used afterwards.
func (c *Capturer) Release(buf []int16) {
	if cap(buf) == 0 {
		return
	}
	c.pool.Put(buf[:0])
}

// TargetSampleRate returns the output sample rate (e.g. 48000).
func (c *Capturer) TargetSampleRate() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.targetRate <= 0 {
		return 48000
	}
	return c.targetRate
}

// Stop halts any ongoing audio capture.
func (c *Capturer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelFunc != nil {
		c.cancelFunc()
		c.cancelFunc = nil
	}
	c.running = false
}

// Start begins audio capture from the given device name or default loopback.
func (c *Capturer) Start(dev Device, onData Callback, onError func(error)) error {
	c.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancelFunc = cancel
	c.running = true
	c.mu.Unlock()

	go c.captureLoop(ctx, dev, onData, onError)
	return nil
}

func clampF32(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (c *Capturer) captureLoop(ctx context.Context, dev Device, onData Callback, onError func(error)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.com_init", err.Error()))
		}
		return
	}
	defer ole.CoUninitialize()

	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &de); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.enumerator", err.Error()))
		}
		return
	}
	defer de.Release()

	var dc *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(dev.DataFlow, wca.DEVICE_STATE_ACTIVE, &dc); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.enum_devices", err.Error()))
		}
		return
	}
	defer dc.Release()

	var mmd *wca.IMMDevice
	if err := dc.Item(dev.Index, &mmd); err != nil {
		// Fallback to default endpoint
		if err := de.GetDefaultAudioEndpoint(dev.DataFlow, wca.EConsole, &mmd); err != nil {
			if onError != nil {
				onError(i18n.NewError("audio.open_device", err.Error()))
			}
			return
		}
	}
	defer mmd.Release()

	var ac *wca.IAudioClient
	if err := mmd.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &ac); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.activate_client", err.Error()))
		}
		return
	}
	defer ac.Release()

	var wfx *wca.WAVEFORMATEX
	if err := ac.GetMixFormat(&wfx); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.mix_format", err.Error()))
		}
		return
	}
	defer ole.CoTaskMemFree(uintptr(unsafe.Pointer(wfx)))

	inRate := int(wfx.NSamplesPerSec)
	inChannels := int(wfx.NChannels)
	bitsPerSample := int(wfx.WBitsPerSample)
	blockAlign := int(wfx.NBlockAlign)
	isFloat := (wfx.WFormatTag == FormatIEEEFloat) || (bitsPerSample == 32)

	// Choose appropriate target rate for MP3 (44100 or 48000)
	targetRate := 48000
	if inRate%44100 == 0 {
		targetRate = 44100
	}
	c.mu.Lock()
	c.targetRate = targetRate
	c.mu.Unlock()

	// Event-driven capture: WASAPI signals this event whenever a packet is ready.
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.event_setup", err.Error()))
		}
		return
	}
	defer windows.CloseHandle(event)

	streamFlags := uint32(wca.AUDCLNT_STREAMFLAGS_EVENTCALLBACK)
	if dev.IsLoopback {
		streamFlags |= wca.AUDCLNT_STREAMFLAGS_LOOPBACK
	}

	// 200ms buffer size
	bufferDuration := wca.REFERENCE_TIME(200 * 10000)
	if err := ac.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, streamFlags, bufferDuration, 0, wfx, nil); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.init_wasapi", err.Error()))
		}
		return
	}

	if err := ac.SetEventHandle(uintptr(event)); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.event_setup", err.Error()))
		}
		return
	}

	var acc *wca.IAudioCaptureClient
	if err := ac.GetService(wca.IID_IAudioCaptureClient, &acc); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.capture_client", err.Error()))
		}
		return
	}
	defer acc.Release()

	if err := ac.Start(); err != nil {
		if onError != nil {
			onError(i18n.NewError("audio.start_capture", err.Error()))
		}
		return
	}
	defer func() {
		_ = ac.Stop()
	}()

	var data *byte
	var availableFrames uint32
	var flags uint32
	var devPos uint64
	var qcpPos uint64

	lastVU := time.Now()
	silenceFrames := targetRate / 10

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Block until WASAPI has a packet ready. On timeout the device is idle,
		// so emit silence to keep the stream (and the VU meter) alive.
		status, _ := windows.WaitForSingleObject(event, captureWaitMs)
		if status == uint32(windows.WAIT_TIMEOUT) {
			if onData != nil {
				silence := c.getBuf(silenceFrames * 2)
				clear(silence)
				onData(silence, 0, 0)
			}
			continue
		}

		// Drain every queued packet before waiting on the event again.
		for {
			if err := acc.GetBuffer(&data, &availableFrames, &flags, &devPos, &qcpPos); err != nil {
				break // AUDCLNT_S_BUFFER_EMPTY: nothing left to read
			}
			if availableFrames == 0 {
				break
			}

			isSilent := (flags & wca.AUDCLNT_BUFFERFLAGS_SILENT) != 0
			numFrames := int(availableFrames)

			// Convert captured frames into 16-bit stereo PCM (L, R)
			pcmStereo := c.getBuf(numFrames * 2)
			var peakL, peakR float32

			if isSilent {
				clear(pcmStereo)
			} else {
				rawBytes := unsafe.Slice(data, numFrames*blockAlign)

				for i := 0; i < numFrames; i++ {
					frameOffset := i * blockAlign
					var sampleL, sampleR float32

					if isFloat {
						// 32-bit float
						fL := *(*float32)(unsafe.Pointer(&rawBytes[frameOffset]))
						sampleL = clampF32(fL, -1.0, 1.0)
						if inChannels > 1 {
							fR := *(*float32)(unsafe.Pointer(&rawBytes[frameOffset+4]))
							sampleR = clampF32(fR, -1.0, 1.0)
						} else {
							sampleR = sampleL
						}
					} else if bitsPerSample == 16 {
						// 16-bit int
						iL := *(*int16)(unsafe.Pointer(&rawBytes[frameOffset]))
						sampleL = float32(iL) / 32768.0
						if inChannels > 1 {
							iR := *(*int16)(unsafe.Pointer(&rawBytes[frameOffset+2]))
							sampleR = float32(iR) / 32768.0
						} else {
							sampleR = sampleL
						}
					} else if bitsPerSample == 24 {
						// 24-bit int
						b0 := rawBytes[frameOffset]
						b1 := rawBytes[frameOffset+1]
						b2 := rawBytes[frameOffset+2]
						valL := int32(b0) | (int32(b1) << 8) | (int32(int8(b2)) << 16)
						sampleL = float32(valL) / 8388608.0

						if inChannels > 1 {
							b0r := rawBytes[frameOffset+3]
							b1r := rawBytes[frameOffset+4]
							b2r := rawBytes[frameOffset+5]
							valR := int32(b0r) | (int32(b1r) << 8) | (int32(int8(b2r)) << 16)
							sampleR = float32(valR) / 8388608.0
						} else {
							sampleR = sampleL
						}
					}

					pcmStereo[i*2] = int16(sampleL * 32767.0)
					pcmStereo[i*2+1] = int16(sampleR * 32767.0)

					absL := float32(math.Abs(float64(sampleL)))
					absR := float32(math.Abs(float64(sampleR)))
					if absL > peakL {
						peakL = absL
					}
					if absR > peakR {
						peakR = absR
					}
				}
			}

			_ = acc.ReleaseBuffer(availableFrames)

			// Resample if necessary (e.g. 96kHz to 48kHz or 88.2kHz to 44.1kHz)
			outPCM := pcmStereo
			if inRate != targetRate {
				outFrames := int(int64(numFrames) * int64(targetRate) / int64(inRate))
				if outFrames > 0 {
					dst := c.getBuf(outFrames * 2)
					resampleStereoInto(dst, pcmStereo, inRate, targetRate)
					c.Release(pcmStereo)
					outPCM = dst
				}
			}

			if onData == nil {
				c.Release(outPCM)
				continue
			}

			// Calculate VU peak with boost like Python (min(1.0, peak * 1.5))
			scaledL := peakL * 1.5
			if scaledL > 1.0 {
				scaledL = 1.0
			}
			scaledR := peakR * 1.5
			if scaledR > 1.0 {
				scaledR = 1.0
			}

			now := time.Now()
			if now.Sub(lastVU) >= 40*time.Millisecond {
				lastVU = now
				onData(outPCM, scaledL, scaledR)
			} else {
				onData(outPCM, -1, -1) // -1: don't update VU on this tick
			}
		}
	}
}

// resampleStereoInto writes the linearly resampled stereo input into dst, whose
// length must be outFrames*2. It performs no allocation.
func resampleStereoInto(dst, input []int16, inRate, outRate int) {
	inFrames := len(input) / 2
	outFrames := len(dst) / 2
	if inRate == outRate || inFrames < 2 || outFrames <= 0 {
		copy(dst, input)
		return
	}

	ratio := float64(inFrames-1) / float64(outFrames)
	for i := 0; i < outFrames; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx+1 < inFrames {
			l0 := float64(input[srcIdx*2])
			l1 := float64(input[(srcIdx+1)*2])
			r0 := float64(input[srcIdx*2+1])
			r1 := float64(input[(srcIdx+1)*2+1])

			dst[i*2] = int16(l0 + frac*(l1-l0))
			dst[i*2+1] = int16(r0 + frac*(r1-r0))
		} else {
			dst[i*2] = input[srcIdx*2]
			dst[i*2+1] = input[srcIdx*2+1]
		}
	}
}

// resampleStereo performs linear interpolation resampling for stereo 16-bit PCM,
// allocating the output buffer.
func resampleStereo(input []int16, inRate, outRate int) []int16 {
	if inRate == outRate || len(input) < 2 {
		return input
	}
	inFrames := len(input) / 2
	outFrames := int(int64(inFrames) * int64(outRate) / int64(inRate))
	if outFrames <= 0 {
		return nil
	}
	output := make([]int16, outFrames*2)
	resampleStereoInto(output, input, inRate, outRate)
	return output
}
