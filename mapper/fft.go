package mapper

import (
	"math/cmplx"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"gonum.org/v1/gonum/dsp/fourier"
)

type FftMapper struct {
	config   config.MapperConfig
	protocol string
	mappings map[string]*singleFftMapper
}

type singleFftMapper struct {
	spectrumPath       string
	samplesBuffer      map[time.Time]float64
	samplesBufferMutex *sync.Mutex
	fft                *fourier.FFT
}

func NewFftMapper(c config.MapperConfig, fftc []*config.FftConfig) (*FftMapper, error) {
	mappings := make(map[string]*singleFftMapper)
	for _, cfg := range fftc {
		mappings[cfg.Path] = &singleFftMapper{
			spectrumPath: cfg.SpectrumPath,
			// allow double space for values comming in not in order
			samplesBuffer:      make(map[time.Time]float64, 2<<cfg.SamplesChannelBitSize),
			samplesBufferMutex: &sync.Mutex{},
			fft:                fourier.NewFFT(1 << cfg.SamplesChannelBitSize),
		}
	}
	return &FftMapper{
		config:   c,
		protocol: config.SignalKType,
		mappings: mappings,
	}, nil
}

func (m *FftMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, true)
}

func (m *FftMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel("signalk").WithType(m.protocol).WithUuid(uuid.Nil)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Time{}) // initialize with empty timestamp instead of hidden now

	for _, svm := range input.ToSingleValueMapped() {
		if _, ok := m.mappings[svm.Path]; ok {
			if value, ok := svm.Value.(float64); ok {
				m.mappings[svm.Path].samplesBuffer[svm.Timestamp] = value
			}
			m.doFft(u, svm.Path)
		}
	}

	if len(u.Values) == 0 {
		return result, nil
	}
	return result.AddUpdate(u), nil
}

func (m *FftMapper) doFft(update *message.Update, path string) {
	m.mappings[path].samplesBufferMutex.Lock()
	defer m.mappings[path].samplesBufferMutex.Unlock()
	if len(m.mappings[path].samplesBuffer) < m.mappings[path].fft.Len()<<1 {
		return
	}

	timestamps := m.sortTimestamps(path)
	value := message.Spectrum{
		NumberOfSamples: m.mappings[path].fft.Len(),
		Duration:        timestamps[m.mappings[path].fft.Len()].Sub(timestamps[0]).Seconds(),
		Coefficients:    make([]message.Coefficient, 0, m.mappings[path].fft.Len()),
	}

	samples := m.extractSamples(path, timestamps)
	coeff := m.mappings[path].fft.Coefficients(nil, samples)

	samplesPerSecond := float64(value.NumberOfSamples) / value.Duration
	for i, c := range coeff {
		value.Coefficients = append(value.Coefficients, message.Coefficient{
			Frequency: m.mappings[path].fft.Freq(i) * samplesPerSecond,
			Magnitude: 2 * cmplx.Abs(c) / float64(value.NumberOfSamples),
			Phase:     cmplx.Phase(c),
		})
	}
	update.AddValue(
		message.NewValue().WithPath(m.mappings[path].spectrumPath).WithValue(value),
	).WithTimestamp(timestamps[0])
}

func (m *FftMapper) extractSamples(path string, timestamps []time.Time) []float64 {
	samples := make([]float64, 0, m.mappings[path].fft.Len())
	for i := 0; i < cap(samples); i++ {
		samples = append(samples, m.mappings[path].samplesBuffer[timestamps[i]])
		delete(m.mappings[path].samplesBuffer, timestamps[i])
	}
	return samples
}

func (m *FftMapper) sortTimestamps(path string) []time.Time {
	timestamps := make([]time.Time, 0, m.mappings[path].fft.Len()<<1)
	for k := range m.mappings[path].samplesBuffer {
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
