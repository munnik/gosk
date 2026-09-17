package mapper

import (
	"math"
	"math/cmplx"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
	"go.uber.org/zap"
	"gonum.org/v1/gonum/dsp/fourier"
)

// warmupTimeoutExtension is how far ExtendTimeout pushes systemd's start
// timeout out on each call while an FftMapper is still filling its sample
// buffer - comfortably longer than sdnotify.ExtendTimeoutMinInterval so
// the extension never lapses between calls.
const warmupTimeoutExtension = 15 * time.Second

// defaultGapDetectionThreshold is used when a mapping's
// config.FftConfig.GapDetectionThreshold is unset (zero). See
// singleFftMapper.gapDetectionThreshold for what the threshold means.
const defaultGapDetectionThreshold = 3.0

type FftMapper struct {
	config   config.MapperConfig
	protocol string
	mappings map[string]*singleFftMapper
	// warmedUp is set once this mapper has produced its first spectrum -
	// i.e. once its Publisher has called sdnotify.Ready() (see
	// nanomsg/pub.go). Until then, DoMap keeps nudging systemd's start
	// timeout via sdnotify.ExtendTimeout to cover the many seconds it can
	// take to fill a window's worth of samples (see doFft); afterwards
	// there's no longer a start timeout to extend, so it stops.
	warmedUp atomic.Bool
}

type singleFftMapper struct {
	spectrumPath       string
	samplesBuffer      map[time.Time]float64
	samplesBufferMutex *sync.Mutex
	fft                *fourier.FFT
	frequencyStepSize  float64
	// window and windowSum implement a Hann window, applied to each set of
	// samples before the FFT to reduce spectral leakage from truncating a
	// non-integer number of cycles. windowSum is the window's coherent gain
	// (the sum of its coefficients), used to keep the reported magnitude
	// comparable to an unwindowed signal's.
	window    []float64
	windowSum float64
	// hopSize is half the window length: consecutive FFT windows overlap by
	// 50%, so a transient that happens to straddle one window's boundary is
	// still fully captured by the next.
	hopSize int
	// gapDetectionThreshold flags an FFT window as unreliable when one
	// sample interval in it is more than this many times the window's mean
	// interval — a sign of a dropped-sample gap (e.g. a brief connector
	// hiccup) rather than normal jitter. The FFT assumes uniformly spaced
	// samples, so silently transforming a window with a gap in it would
	// produce a spectrum that looks normal but is quietly wrong.
	gapDetectionThreshold float64
}

func NewFftMapper(c config.MapperConfig, fftc []*config.FftConfig) (*FftMapper, error) {
	mappings := make(map[string]*singleFftMapper)
	for _, cfg := range fftc {
		n := 1 << cfg.SamplesChannelBitSize
		window, windowSum := hannWindow(n)
		hopSize := max(n/2, 1)
		gapDetectionThreshold := cfg.GapDetectionThreshold
		if gapDetectionThreshold <= 0 {
			gapDetectionThreshold = defaultGapDetectionThreshold
		}
		mappings[cfg.Path] = &singleFftMapper{
			spectrumPath: cfg.SpectrumPath,
			// allow slack for values coming in not in order
			samplesBuffer:         make(map[time.Time]float64, n+hopSize),
			samplesBufferMutex:    &sync.Mutex{},
			fft:                   fourier.NewFFT(n),
			frequencyStepSize:     cfg.FrequencyStepSize,
			window:                window,
			windowSum:             windowSum,
			hopSize:               hopSize,
			gapDetectionThreshold: gapDetectionThreshold,
		}
	}
	return &FftMapper{
		config:   c,
		protocol: config.SignalKType,
		mappings: mappings,
	}, nil
}

// hannWindow returns an n-point Hann window and the sum of its coefficients
// (its coherent gain), used to normalize the FFT magnitude back to the same
// scale a rectangular (unwindowed) signal would produce.
func hannWindow(n int) ([]float64, float64) {
	w := make([]float64, n)
	if n == 1 {
		w[0] = 1
		return w, 1
	}
	var sum float64
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
		sum += w[i]
	}
	return w, sum
}

// GetTickerInterval always returns 0. FftMapper does not implement
// periodicMapper, so process (see main.go) never re-evaluates it on a
// timer regardless of what this returns. Noticing that a sensor has gone
// silent is the notify stage's job (mapper/notification.go), which already
// watches source paths for staleness — duplicating that here would just be
// a second, redundant staleness mechanism.
func (m *FftMapper) GetTickerInterval() time.Duration {
	return 0
}

func (m *FftMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, true)
}

func (m *FftMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	if !m.warmedUp.Load() {
		sdnotify.ExtendTimeout(warmupTimeoutExtension)
	}

	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel("signalk").WithType(m.protocol).WithUuid(uuid.Nil)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Time{}) // initialize with empty timestamp instead of hidden now

	for _, svm := range input.ToSingleValueMapped() {
		sfm, ok := m.mappings[svm.Path]
		if !ok {
			continue
		}
		value, ok := svm.Value.(float64)
		if !ok {
			logger.GetLogger().Warn(
				"Discarding a non-numeric value for an FFT path",
				zap.String("path", svm.Path),
				zap.Any("value", svm.Value),
			)
			continue
		}
		sfm.samplesBufferMutex.Lock()
		sfm.samplesBuffer[svm.Timestamp] = value
		sfm.samplesBufferMutex.Unlock()
		m.doFft(u, svm.Path)
	}

	if len(u.Values) == 0 {
		return result, nil
	}
	m.warmedUp.Store(true)
	return result.AddUpdate(u), nil
}

func (m *FftMapper) doFft(update *message.Update, path string) {
	sfm := m.mappings[path]
	sfm.samplesBufferMutex.Lock()
	defer sfm.samplesBufferMutex.Unlock()

	windowLen := sfm.fft.Len()
	if len(sfm.samplesBuffer) < windowLen+sfm.hopSize {
		return
	}

	timestamps := m.sortTimestamps(path)
	window := timestamps[:windowLen]
	boundary := timestamps[windowLen]
	duration := boundary.Sub(window[0]).Seconds()

	if valid := m.isRegularlySpaced(sfm, window, boundary, duration); valid {
		value := message.Spectrum{
			NumberOfSamples:   windowLen,
			Duration:          duration,
			Coefficients:      make([]message.Coefficient, 0, windowLen),
			FrequencyStepSize: sfm.frequencyStepSize,
		}

		samples := m.windowedSamples(path, window)
		coeff := sfm.fft.Coefficients(nil, samples)
		m.buildSpectrum(&value, coeff, path)

		update.AddValue(
			message.NewValue().WithPath(sfm.spectrumPath).WithValue(value),
		).WithTimestamp(window[0])
	} else {
		logger.GetLogger().Warn(
			"Discarding an FFT window with an irregular sample interval, a dropped-sample gap, or duplicate/out-of-order timestamps",
			zap.String("path", path),
			zap.Time("windowStart", window[0]),
			zap.Time("windowEnd", boundary),
		)
	}

	// slide the window forward by half its length regardless of whether it
	// was valid, so a bad window doesn't stall the pipeline: the next
	// attempt starts fresh instead of re-examining the same gap forever.
	m.evictConsumed(path, window)
}

// isRegularlySpaced reports whether window, followed by boundary, are
// plausibly uniformly-spaced samples: the FFT assumes that, and a gap or
// duplicate/out-of-order timestamp would otherwise silently produce a
// spectrum computed on non-uniformly-spaced data.
func (m *FftMapper) isRegularlySpaced(sfm *singleFftMapper, window []time.Time, boundary time.Time, duration float64) bool {
	if duration <= 0 {
		return false
	}
	meanDelta := duration / float64(len(window))
	previous := window[0]
	for i := 1; i <= len(window); i++ {
		next := boundary
		if i < len(window) {
			next = window[i]
		}
		delta := next.Sub(previous).Seconds()
		if delta <= 0 || delta > meanDelta*sfm.gapDetectionThreshold {
			return false
		}
		previous = next
	}
	return true
}

func (m *FftMapper) buildSpectrum(value *message.Spectrum, coeff []complex128, path string) {
	sfm := m.mappings[path]
	samplesPerSecond := float64(value.NumberOfSamples) / value.Duration
	var spectrumFrequency, coefficientFrequency float64
	var coefficientSum complex128

	for i, c := range coeff {
		coefficientFrequency = sfm.fft.Freq(i) * samplesPerSecond
		coefficientSum += c
		if coefficientFrequency > spectrumFrequency+sfm.frequencyStepSize/2 {
			value.Coefficients = append(
				value.Coefficients,
				message.Coefficient{
					// normalized by the window's coherent gain (windowSum)
					// rather than NumberOfSamples, so a windowed signal's
					// magnitude reads the same as an unwindowed one would.
					Magnitude: 2 * cmplx.Abs(coefficientSum) / sfm.windowSum,
					Phase:     cmplx.Phase(coefficientSum),
				},
			)
			// reset values and increase spectrumFrequency
			coefficientSum = 0
			spectrumFrequency += sfm.frequencyStepSize
		}
	}
}

// windowedSamples reads window's samples and applies the Hann window,
// without consuming them — eviction happens separately, since only the
// oldest half of window is retired to give the next window 50% overlap.
func (m *FftMapper) windowedSamples(path string, window []time.Time) []float64 {
	sfm := m.mappings[path]
	samples := make([]float64, len(window))
	for i, t := range window {
		samples[i] = sfm.samplesBuffer[t] * sfm.window[i]
	}
	return samples
}

// evictConsumed retires the oldest half of window, leaving its newest half
// buffered to become the start of the next, overlapping window.
func (m *FftMapper) evictConsumed(path string, window []time.Time) {
	sfm := m.mappings[path]
	for i := 0; i < sfm.hopSize; i++ {
		delete(sfm.samplesBuffer, window[i])
	}
}

func (m *FftMapper) sortTimestamps(path string) []time.Time {
	buffer := m.mappings[path].samplesBuffer
	timestamps := make([]time.Time, 0, len(buffer))
	for k := range buffer {
		timestamps = append(timestamps, k)
	}
	slices.SortFunc(timestamps, func(a, b time.Time) int {
		if a.Before(b) {
			return -1
		}
		if a.After(b) {
			return 1
		}
		return 0
	})
	return timestamps
}
