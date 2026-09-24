package mapper_test

import (
	"math"
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	fftTestPath         = "propulsion.mainEngine.drive.torque"
	fftTestSpectrumPath = "propulsion.mainEngine.drive.torqueSpectrum"
)

// feedSamples pushes one sample per DoMap call, as the live pipeline would
// (one message per incoming value), and returns every non-empty Spectrum
// found across all the calls.
func feedSamples(m *FftMapper, samples []float64, start time.Time, period time.Duration) []message.Spectrum {
	spectra := make([]message.Spectrum, 0)
	for i, v := range samples {
		input := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
			message.NewUpdate().WithSource(
				*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
			).WithTimestamp(
				start.Add(time.Duration(i) * period),
			).AddValue(
				message.NewValue().WithPath(fftTestPath).WithValue(v),
			),
		)
		out, err := m.DoMap(input)
		Expect(err).ToNot(HaveOccurred())
		for _, svm := range out.ToSingleValueMapped() {
			if svm.Path == fftTestSpectrumPath {
				spectra = append(spectra, svm.Value.(message.Spectrum))
			}
		}
	}
	return spectra
}

var _ = Describe("DoMap fft", func() {
	// sampleRate and windowBits are chosen so the FFT's native bin width
	// (sampleRate/windowLen) is exactly 1Hz, matching FrequencyStepSize
	// below one-for-one, so signalFrequency (an integer) lands exactly on
	// a single output bucket instead of splitting across two.
	const (
		sampleRate      = 256.0
		windowBits      = 8 // windowLen = 1<<8 = 256
		windowLen       = 1 << windowBits
		hopSize         = windowLen / 2
		signalFrequency = 10.0
		amplitude       = 3.0
	)
	period := time.Duration(float64(time.Second) / sampleRate)

	newMapper := func() *FftMapper {
		m, err := NewFftMapper(
			config.MapperConfig{Context: "testingContext"},
			[]*config.FftConfig{
				{
					Path:                  fftTestPath,
					SpectrumPath:          fftTestSpectrumPath,
					SamplesChannelBitSize: windowBits,
					FrequencyStepSize:     1.0,
				},
			},
		)
		Expect(err).ToNot(HaveOccurred())
		return m
	}

	sineWave := func(n int, freq float64) []float64 {
		samples := make([]float64, n)
		for i := range samples {
			samples[i] = amplitude * math.Sin(2*math.Pi*freq*float64(i)/sampleRate)
		}
		return samples
	}

	It("emits exactly one spectrum once the window fills, with a peak at the signal's frequency", func() {
		samples := sineWave(windowLen+hopSize, signalFrequency)
		spectra := feedSamples(newMapper(), samples, time.Now(), period)

		Expect(spectra).To(HaveLen(1))
		spectrum := spectra[0]

		Expect(spectrum.NumberOfSamples).To(Equal(windowLen))
		Expect(spectrum.Duration).To(BeNumerically("~", windowLen/sampleRate, 1e-6))

		peakIndex, peakMagnitude := 0, 0.0
		var magnitudeSum float64
		for i, c := range spectrum.Coefficients {
			magnitudeSum += c.Magnitude
			if c.Magnitude > peakMagnitude {
				peakIndex, peakMagnitude = i, c.Magnitude
			}
		}

		// bucket i covers roughly [i-0.5, i+0.5) Hz since FrequencyStepSize is 1.0
		Expect(float64(peakIndex)).To(BeNumerically("~", signalFrequency, 1))
		// the peak should clearly stand out from the rest of the spectrum's floor
		averageOthers := (magnitudeSum - peakMagnitude) / float64(len(spectrum.Coefficients)-1)
		Expect(peakMagnitude).To(BeNumerically(">", averageOthers*5))
	})

	It("overlaps windows by 50%, emitting a second spectrum after only hopSize more samples", func() {
		samples := sineWave(windowLen+2*hopSize, signalFrequency)
		spectra := feedSamples(newMapper(), samples, time.Now(), period)

		Expect(spectra).To(HaveLen(2))
	})

	It("discards a window with a dropped-sample gap instead of publishing a distorted spectrum", func() {
		samples := sineWave(windowLen+hopSize, signalFrequency)
		start := time.Now()

		m := newMapper()
		spectra := make([]message.Spectrum, 0)
		for i, v := range samples {
			timestamp := start.Add(time.Duration(i) * period)
			if i == windowLen/2 {
				// drop a chunk of real time out of the middle of the window
				timestamp = timestamp.Add(50 * period)
			}
			input := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(timestamp).AddValue(
					message.NewValue().WithPath(fftTestPath).WithValue(v),
				),
			)
			out, err := m.DoMap(input)
			Expect(err).ToNot(HaveOccurred())
			for _, svm := range out.ToSingleValueMapped() {
				if svm.Path == fftTestSpectrumPath {
					spectra = append(spectra, svm.Value.(message.Spectrum))
				}
			}
		}

		Expect(spectra).To(BeEmpty())
	})

	It("stamps a spectrum with the newest sample in its window, not the oldest", func() {
		// The whole pipeline stamps a row with a time derived from when
		// the raw data arrived, so a spectrum is dated by the sample that
		// completed its window. Dating it window[0] instead - as this
		// used to - put every spectrum a full window into the past
		// relative to everything around it.
		start := time.Now().Truncate(time.Millisecond)
		// A spectrum is only emitted once the buffer holds a full window
		// plus one hop, so this feeds exactly enough for one.
		samples := sineWave(windowLen+hopSize, signalFrequency)

		// Every sample carries the same source uuid, so the spectrum's own
		// uuid can be checked against it below.
		source := uuid.Must(uuid.NewV7Precise())

		m := newMapper()
		timestamps := make([]time.Time, 0)
		uuids := make([]uuid.UUID, 0)
		for i, v := range samples {
			input := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(source),
				).WithTimestamp(
					start.Add(time.Duration(i) * period),
				).AddValue(
					message.NewValue().WithPath(fftTestPath).WithValue(v),
				),
			)
			out, err := m.DoMap(input)
			Expect(err).ToNot(HaveOccurred())
			for _, svm := range out.ToSingleValueMapped() {
				if svm.Path == fftTestSpectrumPath {
					timestamps = append(timestamps, svm.Timestamp)
					uuids = append(uuids, svm.Source.Uuid)
				}
			}
		}

		Expect(timestamps).To(HaveLen(1))
		// windowLen samples were fed, so the window covers indices
		// 0..windowLen-1 and the newest of them is the last.
		Expect(timestamps[0]).To(BeTemporally("==", start.Add(time.Duration(windowLen-1)*period)))

		// And the spectrum carries a real uuid rather than the uuid.Nil it
		// used to - derived from the sample that completed the window, so
		// it keeps that sample's randomness and takes its own timestamp.
		// See section 7.1.1 of TRANSFER_REVIEW.md.
		Expect(uuids).To(HaveLen(1))
		Expect(uuids[0]).ToNot(Equal(uuid.Nil))
		Expect(uuids[0].Version()).To(BeEquivalentTo(7))
		Expect(uuids[0][8:]).To(Equal(source[8:]))
		Expect(uuids[0][:8]).ToNot(Equal(source[:8]))
	})

	It("discards a non-numeric value instead of panicking", func() {
		m := newMapper()
		input := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
			message.NewUpdate().WithSource(
				*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
			).WithTimestamp(time.Now()).AddValue(
				message.NewValue().WithPath(fftTestPath).WithValue("not a number"),
			),
		)
		out, err := m.DoMap(input)
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Updates).To(BeEmpty())
	})
})
