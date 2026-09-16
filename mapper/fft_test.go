package mapper_test

import (
	"math"
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
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

	It("runs two independent spectra off a single mapper instance, each from its own path", func() {
		// mirrors node-hbr-rpa10's mapShaftPowerMeterPortFft/StarboardFft in
		// ../nix: one FftMapper, fed drive.torque and drive.bendingMoment
		// together (they come from the same upstream BinaryMapper), so a
		// torsional vibration (which shows up in torque, the sensor sum)
		// and a bending vibration (which shows up in bendingMoment, the
		// sensor difference - see mappingsForMannerTcpBendingMoment's doc
		// comment in ../nix) can be told apart by which spectrum peaks,
		// without any phase computation.
		const (
			bendingPath         = "propulsion.mainEngine.drive.bendingMoment"
			bendingSpectrumPath = "propulsion.mainEngine.drive.bendingMomentSpectrum"
			torsionalFrequency  = 10.0
			bendingFrequency    = 40.0
		)
		m, err := NewFftMapper(
			config.MapperConfig{Context: "testingContext"},
			[]*config.FftConfig{
				{
					Path:                  fftTestPath,
					SpectrumPath:          fftTestSpectrumPath,
					SamplesChannelBitSize: windowBits,
					FrequencyStepSize:     1.0,
				},
				{
					Path:                  bendingPath,
					SpectrumPath:          bendingSpectrumPath,
					SamplesChannelBitSize: windowBits,
					FrequencyStepSize:     1.0,
				},
			},
		)
		Expect(err).ToNot(HaveOccurred())

		torque := sineWave(windowLen+hopSize, torsionalFrequency)
		bending := sineWave(windowLen+hopSize, bendingFrequency)
		start := time.Now()
		var torqueSpectra, bendingSpectra []message.Spectrum
		for i := range torque {
			input := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(
					start.Add(time.Duration(i) * period),
				).AddValue(
					message.NewValue().WithPath(fftTestPath).WithValue(torque[i]),
				).AddValue(
					message.NewValue().WithPath(bendingPath).WithValue(bending[i]),
				),
			)
			out, err := m.DoMap(input)
			Expect(err).ToNot(HaveOccurred())
			for _, svm := range out.ToSingleValueMapped() {
				switch svm.Path {
				case fftTestSpectrumPath:
					torqueSpectra = append(torqueSpectra, svm.Value.(message.Spectrum))
				case bendingSpectrumPath:
					bendingSpectra = append(bendingSpectra, svm.Value.(message.Spectrum))
				}
			}
		}

		Expect(torqueSpectra).To(HaveLen(1))
		Expect(bendingSpectra).To(HaveLen(1))

		peakFrequency := func(s message.Spectrum) int {
			peakIndex, peakMagnitude := 0, 0.0
			for i, c := range s.Coefficients {
				if c.Magnitude > peakMagnitude {
					peakIndex, peakMagnitude = i, c.Magnitude
				}
			}
			return peakIndex
		}
		Expect(float64(peakFrequency(torqueSpectra[0]))).To(BeNumerically("~", torsionalFrequency, 1))
		Expect(float64(peakFrequency(bendingSpectra[0]))).To(BeNumerically("~", bendingFrequency, 1))
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
