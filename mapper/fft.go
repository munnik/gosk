package mapper

import (
	"math/cmplx"
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
	firstSampleTime    time.Time
	samplesBuffer      []float64
	samplesBufferMutex *sync.Mutex
	fft                *fourier.FFT
}

func NewFftMapper(c config.MapperConfig, fftc []*config.FftConfig) (*FftMapper, error) {
	mappings := make(map[string]*singleFftMapper)
	for _, cfg := range fftc {
		mappings[cfg.Path] = &singleFftMapper{
			spectrumPath:       cfg.SpectrumPath,
			samplesBuffer:      make([]float64, 0, 1<<cfg.SamplesChannelBitSize),
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
				if len(m.mappings[svm.Path].samplesBuffer) == 0 {
					m.mappings[svm.Path].firstSampleTime = time.Now()
				}
				m.mappings[svm.Path].samplesBuffer = append(m.mappings[svm.Path].samplesBuffer, value)
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
	if len(m.mappings[path].samplesBuffer) < cap(m.mappings[path].samplesBuffer) {
		return
	}
	value := message.Spectrum{
		NumberOfSamples: len(m.mappings[path].samplesBuffer),
		Duration:        time.Since(m.mappings[path].firstSampleTime).Seconds(),
	}
	coeff := m.mappings[path].fft.Coefficients(nil, m.mappings[path].samplesBuffer)
	m.mappings[path].samplesBuffer = m.mappings[path].samplesBuffer[:0] // truncate the buffer
	value.Coefficients = make([]message.Coefficient, 0, len(coeff))

	samplesPerSecond := float64(value.NumberOfSamples) / value.Duration
	for i, c := range coeff {
		value.MaxFrequency = m.mappings[path].fft.Freq(i) * samplesPerSecond
		value.Coefficients = append(value.Coefficients, message.Coefficient{
			Magnitude: 2 * cmplx.Abs(c) / float64(cap(m.mappings[path].samplesBuffer)),
			Phase:     cmplx.Phase(c),
		})
	}
	update.WithTimestamp(m.mappings[path].firstSampleTime)
	update.AddValue(
		message.NewValue().WithPath(m.mappings[path].spectrumPath).WithValue(value),
	)
}
