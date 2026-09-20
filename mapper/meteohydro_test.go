package mapper

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeWeatherSource records every request and returns a canned result, so
// the mapper can be exercised without an HTTP server and without the
// timing of a real request.
type fakeWeatherSource struct {
	mutex       sync.Mutex
	calls       int
	positions   [][2]float64
	observation *weatherObservation
	err         error
}

func (f *fakeWeatherSource) fetch(_ context.Context, latitude float64, longitude float64) (*weatherObservation, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.calls++
	f.positions = append(f.positions, [2]float64{latitude, longitude})
	return f.observation, f.err
}

func (f *fakeWeatherSource) callCount() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.calls
}

func (f *fakeWeatherSource) fail(err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.err = err
}

// fakeMarineSource is the fakeWeatherSource of the marine endpoint.
type fakeMarineSource struct {
	mutex       sync.Mutex
	calls       int
	observation *marineObservation
	err         error
}

func (f *fakeMarineSource) fetch(_ context.Context, _ float64, _ float64) (*marineObservation, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.calls++
	return f.observation, f.err
}

func (f *fakeMarineSource) callCount() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.calls
}

func float(value float64) *float64 {
	return &value
}

func degrees(value float64) float64 {
	return value * math.Pi / 180
}

const meteoHydroTestContext = "vessels.urn:mrn:imo:mmsi:123456789"

func meteoHydroTestConfig() config.MeteoHydroMapperConfig {
	c := config.DefaultMeteoHydroMapperConfig()
	c.Context = meteoHydroTestContext
	return c
}

// fullObservation is a complete observation, roughly a mild day with a 10
// m/s wind from the north east.
func fullObservation(timestamp time.Time) *weatherObservation {
	return &weatherObservation{
		timestamp:        timestamp,
		temperature:      float(283.15),
		dewPoint:         float(278.15),
		relativeHumidity: float(0.72),
		pressure:         float(101300),
		windSpeed:        float(10),
		windGust:         float(17),
		windDirection:    float(degrees(45)),
		visibility:       float(24000),
	}
}

// fullMarineObservation is a moderate sea from the north west with a bit
// of swell running in from the south west.
func fullMarineObservation(timestamp time.Time) *marineObservation {
	return &marineObservation{
		timestamp:             timestamp,
		waveHeight:            float(1.9),
		waveDirection:         float(degrees(300)),
		wavePeriod:            float(5.5),
		windWaveHeight:        float(1.8),
		windWaveDirection:     float(degrees(298)),
		windWavePeriod:        float(4.9),
		swellHeight:           float(0.6),
		swellDirection:        float(degrees(220)),
		swellPeriod:           float(4.7),
		seaSurfaceTemperature: float(292.45),
		currentSpeed:          float(0.35),
		currentDirection:      float(degrees(90)),
	}
}

func navigationUpdate(timestamp time.Time, values ...*message.Value) *message.Mapped {
	update := message.NewUpdate().WithTimestamp(timestamp)
	for _, value := range values {
		update.AddValue(value)
	}
	return message.NewMapped().WithContext(meteoHydroTestContext).AddUpdate(update)
}

func positionValue(latitude float64, longitude float64) *message.Value {
	return message.NewValue().WithPath(meteoHydroPathPosition).WithValue(message.Position{Latitude: &latitude, Longitude: &longitude})
}

// valuesByPath collapses an update into a path to value map. Every value
// this mapper publishes is a float64 on a unique path, except
// environment.current, which is left out here and checked on its own.
func valuesByPath(mapped *message.Mapped) map[string]float64 {
	result := make(map[string]float64)
	for _, update := range mapped.Updates {
		for _, value := range update.Values {
			if f, ok := value.Value.(float64); ok {
				result[value.Path] = f
			}
		}
	}
	return result
}

// valueAtPath returns the raw value published on path, for the paths that
// are not a plain float64.
func valueAtPath(mapped *message.Mapped, path string) (interface{}, bool) {
	for _, update := range mapped.Updates {
		for _, value := range update.Values {
			if value.Path == path {
				return value.Value, true
			}
		}
	}
	return nil, false
}

var _ = Describe("normalizeAngle", func() {
	It("folds an angle into (-pi, pi]", func() {
		Expect(normalizeAngle(0)).To(BeNumerically("~", 0, 1e-9))
		Expect(normalizeAngle(math.Pi)).To(BeNumerically("~", math.Pi, 1e-9))
		Expect(normalizeAngle(degrees(270))).To(BeNumerically("~", degrees(-90), 1e-9))
		Expect(normalizeAngle(degrees(-270))).To(BeNumerically("~", degrees(90), 1e-9))
		Expect(normalizeAngle(degrees(720 + 10))).To(BeNumerically("~", degrees(10), 1e-9))
	})
})

var _ = Describe("normalizeDirection", func() {
	It("folds an angle into [0, 2pi), the range a compass direction uses", func() {
		Expect(normalizeDirection(0)).To(BeNumerically("~", 0, 1e-9))
		Expect(normalizeDirection(degrees(301))).To(BeNumerically("~", degrees(301), 1e-9))
		Expect(normalizeDirection(degrees(-61))).To(BeNumerically("~", degrees(299), 1e-9))
		Expect(normalizeDirection(degrees(370))).To(BeNumerically("~", degrees(10), 1e-9))
	})
})

var _ = Describe("apparentWind", func() {
	It("returns the true wind when the vessel is not moving", func() {
		// wind from the east, vessel pointing north and stopped
		speed, angle := apparentWind(degrees(90), 8, windFrame{reference: 0, course: 0, speed: 0})

		Expect(speed).To(BeNumerically("~", 8, 1e-9))
		Expect(angle).To(BeNumerically("~", degrees(90), 1e-9))
	})

	It("adds the vessel's speed to a head wind", func() {
		speed, angle := apparentWind(0, 10, windFrame{reference: 0, course: 0, speed: 5})

		Expect(speed).To(BeNumerically("~", 15, 1e-9))
		Expect(angle).To(BeNumerically("~", 0, 1e-9))
	})

	It("subtracts the vessel's speed from a following wind", func() {
		speed, angle := apparentWind(math.Pi, 10, windFrame{reference: 0, course: 0, speed: 4})

		Expect(speed).To(BeNumerically("~", 6, 1e-9))
		Expect(angle).To(BeNumerically("~", math.Pi, 1e-9))
	})

	It("turns a following wind into a head wind when the vessel outruns it", func() {
		speed, angle := apparentWind(math.Pi, 4, windFrame{reference: 0, course: 0, speed: 10})

		Expect(speed).To(BeNumerically("~", 6, 1e-9))
		Expect(angle).To(BeNumerically("~", 0, 1e-9))
	})

	It("moves a beam wind forward", func() {
		// wind from starboard, vessel heading north at the same speed as
		// the wind: the apparent wind comes from 45 degrees to starboard
		speed, angle := apparentWind(degrees(90), 10, windFrame{reference: 0, course: 0, speed: 10})

		Expect(speed).To(BeNumerically("~", math.Sqrt(200), 1e-9))
		Expect(angle).To(BeNumerically("~", degrees(45), 1e-9))
	})

	It("reports an angle to port as a negative angle", func() {
		speed, angle := apparentWind(degrees(270), 10, windFrame{reference: 0, course: 0, speed: 10})

		Expect(speed).To(BeNumerically("~", math.Sqrt(200), 1e-9))
		Expect(angle).To(BeNumerically("~", degrees(-45), 1e-9))
	})

	It("measures the angle from the reference direction", func() {
		// same situation as the beam wind above, but the vessel points
		// east instead of north
		speed, angle := apparentWind(degrees(180), 10, windFrame{reference: degrees(90), course: degrees(90), speed: 10})

		Expect(speed).To(BeNumerically("~", math.Sqrt(200), 1e-9))
		Expect(angle).To(BeNumerically("~", degrees(45), 1e-9))
	})

	It("accounts for a course over ground that differs from the heading", func() {
		// the vessel points north but is set 90 degrees to starboard by
		// the current, in still air the apparent wind is the vessel's own
		// movement, felt from abeam to starboard
		speed, angle := apparentWind(0, 0, windFrame{reference: 0, course: degrees(90), speed: 6})

		Expect(speed).To(BeNumerically("~", 6, 1e-9))
		Expect(angle).To(BeNumerically("~", degrees(90), 1e-9))
	})
})

var _ = Describe("navigationState", func() {
	var now time.Time
	var state *navigationState
	const timeout = 5 * time.Minute
	const minSpeed = 0.5

	BeforeEach(func() {
		now = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
		state = &navigationState{}
	})

	It("prefers the heading over the course over ground", func() {
		state.headingTrue.set(degrees(10), now)
		state.courseOverGroundTrue.set(degrees(200), now)
		state.speedOverGround.set(6, now)

		frame, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeTrue())
		Expect(frame.reference).To(BeNumerically("~", degrees(10), 1e-9))
		// the vessel still moves along its course over ground
		Expect(frame.course).To(BeNumerically("~", degrees(-160), 1e-9))
		Expect(frame.speed).To(BeNumerically("~", 6, 1e-9))
	})

	It("uses the course over ground when there is no heading and the vessel moves", func() {
		state.courseOverGroundTrue.set(degrees(200), now)
		state.speedOverGround.set(6, now)

		frame, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeTrue())
		Expect(frame.reference).To(BeNumerically("~", degrees(-160), 1e-9))
	})

	It("does not use the course over ground of a vessel that is not moving", func() {
		state.courseOverGroundTrue.set(degrees(200), now)
		state.speedOverGround.set(0.1, now)

		_, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeFalse())
	})

	It("has no frame at all without a heading and without a speed over ground", func() {
		state.courseOverGroundTrue.set(degrees(200), now)

		_, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeFalse())
	})

	It("uses the heading of a vessel that is not moving, with a zero speed", func() {
		state.headingTrue.set(degrees(10), now)
		state.courseOverGroundTrue.set(degrees(200), now)
		state.speedOverGround.set(0.1, now)

		frame, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeTrue())
		Expect(frame.reference).To(BeNumerically("~", degrees(10), 1e-9))
		Expect(frame.course).To(BeNumerically("~", degrees(10), 1e-9))
		Expect(frame.speed).To(BeZero())
	})

	It("falls back on the heading as the direction of travel", func() {
		state.headingTrue.set(degrees(10), now)
		state.speedOverGround.set(6, now)

		frame, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeTrue())
		Expect(frame.course).To(BeNumerically("~", degrees(10), 1e-9))
	})

	It("converts a magnetic heading with the magnetic variation", func() {
		state.headingMagnetic.set(degrees(10), now)
		state.magneticVariation.set(degrees(5), now)

		heading, ok := state.heading(now, timeout)

		Expect(ok).To(BeTrue())
		Expect(heading).To(BeNumerically("~", degrees(15), 1e-9))
	})

	It("cannot convert a magnetic heading without the magnetic variation", func() {
		state.headingMagnetic.set(degrees(10), now)

		_, ok := state.heading(now, timeout)

		Expect(ok).To(BeFalse())
	})

	It("ignores navigation values that went stale", func() {
		state.headingTrue.set(degrees(10), now.Add(-2*timeout))
		state.speedOverGround.set(6, now.Add(-2*timeout))

		_, ok := state.windFrame(now, timeout, minSpeed)

		Expect(ok).To(BeFalse())
		Expect(state.isMoving(now, timeout, minSpeed)).To(BeFalse())
	})

	It("measures the interval between two positions", func() {
		state.update(message.SingleValueMapped{Path: meteoHydroPathPosition, Timestamp: now, Value: message.Position{Latitude: float(52), Longitude: float(4)}})
		Expect(state.interval).To(BeZero())

		state.update(message.SingleValueMapped{Path: meteoHydroPathPosition, Timestamp: now.Add(30 * time.Second), Value: message.Position{Latitude: float(52), Longitude: float(4)}})
		Expect(state.interval).To(Equal(30 * time.Second))

		// a repeated timestamp says nothing about the rate
		state.update(message.SingleValueMapped{Path: meteoHydroPathPosition, Timestamp: now.Add(30 * time.Second), Value: message.Position{Latitude: float(52), Longitude: float(4)}})
		Expect(state.interval).To(Equal(30 * time.Second))
	})
})

var _ = Describe("windChill and heatIndex", func() {
	It("calculates the wind chill of a cold day with a strong wind", func() {
		chill, ok := windChill(273.15, 10)

		Expect(ok).To(BeTrue())
		Expect(chill).To(BeNumerically("~", 266.1, 0.2))
	})

	It("is not defined above 10 degrees Celsius or in a light breeze", func() {
		_, ok := windChill(288.15, 10)
		Expect(ok).To(BeFalse())

		_, ok = windChill(273.15, 1)
		Expect(ok).To(BeFalse())
	})

	It("calculates the heat index of a warm and humid day", func() {
		// 90 degrees Fahrenheit at 70% relative humidity
		index, ok := heatIndex(305.372, 0.7)

		Expect(ok).To(BeTrue())
		Expect(index).To(BeNumerically("~", 314.2, 0.5))
	})

	It("is not defined below 80 degrees Fahrenheit", func() {
		_, ok := heatIndex(293.15, 0.7)

		Expect(ok).To(BeFalse())
	})
})

var _ = Describe("observationCache", func() {
	var now time.Time

	BeforeEach(func() {
		now = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	})

	It("serves every position within the same grid cell", func() {
		cache := newObservationCache[*weatherObservation](0.1, time.Hour, 10)
		cache.put(52.01, 4.01, now, fullObservation(now))

		_, ok := cache.get(52.02, 4.02, now)
		Expect(ok).To(BeTrue())

		_, ok = cache.get(52.4, 4.01, now)
		Expect(ok).To(BeFalse())
	})

	It("does not serve an observation that aged past the maximum age", func() {
		cache := newObservationCache[*weatherObservation](0.1, time.Hour, 10)
		cache.put(52, 4, now, fullObservation(now))

		_, ok := cache.get(52, 4, now.Add(59*time.Minute))
		Expect(ok).To(BeTrue())

		_, ok = cache.get(52, 4, now.Add(61*time.Minute))
		Expect(ok).To(BeFalse())
	})

	It("keeps at most the configured number of cells, dropping the oldest", func() {
		cache := newObservationCache[*weatherObservation](0.1, time.Hour, 2)
		cache.put(52, 4, now, fullObservation(now))
		cache.put(53, 4, now.Add(time.Minute), fullObservation(now))
		cache.put(54, 4, now.Add(2*time.Minute), fullObservation(now))

		Expect(cache.entries).To(HaveLen(2))
		_, ok := cache.get(52, 4, now.Add(2*time.Minute))
		Expect(ok).To(BeFalse())
		_, ok = cache.get(54, 4, now.Add(2*time.Minute))
		Expect(ok).To(BeTrue())
	})
})

var _ = Describe("MeteoHydroMapper", func() {
	var now time.Time
	var source *fakeWeatherSource
	var marine *fakeMarineSource
	var m *MeteoHydroMapper

	// prime runs the evaluation that fetches the observations for the
	// vessel's position and waits for the request to finish, so the next
	// tick is served from the cache.
	//
	// Whether that first evaluation publishes anything itself is a race
	// it is not worth writing a spec around: refreshMap starts the
	// request and reads the cache a few lines later, so a request that
	// completes in between lands in time to be published and one that
	// does not, does not. Forgetting that it published keeps the specs
	// that follow deterministic either way.
	prime := func(at time.Time) {
		m.refreshMap(at)
		Eventually(func() bool {
			return m.air.idle() && (m.marine == nil || m.marine.idle())
		}).Should(BeTrue())
		m.lastPublish = time.Time{}
	}

	// tick runs a single evaluation and waits for any request it started
	// to finish, so the next tick sees the result of that request.
	tick := func(at time.Time) *message.Mapped {
		result := m.refreshMap(at)
		Eventually(func() bool {
			return m.air.idle() && (m.marine == nil || m.marine.idle())
		}).Should(BeTrue())
		return result
	}

	// report feeds the mapper the vessel's position as of at, the way the
	// mapper it subscribes to would. A position that is never repeated
	// goes stale, which stops the mapper from publishing at all.
	report := func(at time.Time) {
		_, err := m.DoMap(navigationUpdate(at, positionValue(52.1, 4.2)))
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		now = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
		source = &fakeWeatherSource{observation: fullObservation(now)}
		marine = &fakeMarineSource{observation: fullMarineObservation(now)}
		m = newMeteoHydroMapper(meteoHydroTestConfig(), source, marine)
	})

	Describe("DoMap", func() {
		It("records navigation data without publishing anything", func() {
			result, err := m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Updates).To(BeEmpty())
			latitude, longitude, ok := m.navigation.position(now, time.Minute)
			Expect(ok).To(BeTrue())
			Expect(latitude).To(Equal(52.1))
			Expect(longitude).To(Equal(4.2))
		})

		It("ignores the navigation data of another vessel", func() {
			input := navigationUpdate(now, positionValue(52.1, 4.2)).WithContext("vessels.urn:mrn:imo:mmsi:987654321")

			result, err := m.DoMap(input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Updates).To(BeEmpty())
			_, _, ok := m.navigation.position(now, time.Minute)
			Expect(ok).To(BeFalse())
		})

		It("accepts data for vessels.self", func() {
			input := navigationUpdate(now, positionValue(52.1, 4.2)).WithContext("vessels.self")

			_, err := m.DoMap(input)

			Expect(err).ToNot(HaveOccurred())
			_, _, ok := m.navigation.position(now, time.Minute)
			Expect(ok).To(BeTrue())
		})
	})

	Describe("refreshMap", func() {
		It("publishes nothing, and asks for nothing, while the position is unknown", func() {
			Expect(tick(now).Updates).To(BeEmpty())
			Expect(source.callCount()).To(BeZero())
		})

		It("publishes nothing while the position is stale", func() {
			m.DoMap(navigationUpdate(now.Add(-time.Hour), positionValue(52.1, 4.2)))

			Expect(tick(now).Updates).To(BeEmpty())
			Expect(source.callCount()).To(BeZero())
		})

		It("publishes the weather for the position of the vessel", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))

			// the first tick only starts the request, the cache is still
			// empty when it decides what to publish
			Expect(tick(now).Updates).To(BeEmpty())
			Expect(source.callCount()).To(Equal(1))
			Expect(source.positions[0]).To(Equal([2]float64{52.1, 4.2}))

			result := tick(now.Add(10 * time.Second))

			Expect(result.Context).To(Equal(meteoHydroTestContext))
			Expect(result.Updates).To(HaveLen(1))
			Expect(result.Updates[0].Source.Type).To(Equal(config.MeteoHydroType))
			values := valuesByPath(result)
			Expect(values).To(HaveKeyWithValue(meteoHydroPathOutsideTemperature, 283.15))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathOutsideDewPoint, 278.15))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathOutsideRelativeHumidity, 0.72))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathOutsidePressure, 101300.0))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathWindSpeedOverGround, 10.0))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathWindDirectionTrue, degrees(45)))
		})

		It("skips the apparent values without a heading and without movement", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).To(HaveKey(meteoHydroPathWindDirectionTrue))
			Expect(values).ToNot(HaveKey(meteoHydroPathWindAngleApparent))
			Expect(values).ToNot(HaveKey(meteoHydroPathWindSpeedApparent))
			Expect(values).ToNot(HaveKey(meteoHydroPathWindAngleTrueGround))
		})

		It("calculates the apparent values against the heading of a vessel that is not moving", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(meteoHydroPathHeadingTrue).WithValue(degrees(45)),
				message.NewValue().WithPath(meteoHydroPathSpeedOverGround).WithValue(0.0),
				// a course over ground that must be ignored at this speed
				message.NewValue().WithPath(meteoHydroPathCourseOverGroundTrue).WithValue(degrees(180)),
			))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// the wind comes from dead ahead and the vessel adds nothing
			// to it
			Expect(values).To(HaveKeyWithValue(meteoHydroPathWindSpeedApparent, BeNumerically("~", 10, 1e-9)))
			Expect(values[meteoHydroPathWindAngleApparent]).To(BeNumerically("~", 0, 1e-9))
			Expect(values[meteoHydroPathWindAngleTrueGround]).To(BeNumerically("~", 0, 1e-9))
		})

		It("adds the movement of the vessel to the apparent wind", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(meteoHydroPathHeadingTrue).WithValue(degrees(45)),
				message.NewValue().WithPath(meteoHydroPathCourseOverGroundTrue).WithValue(degrees(45)),
				message.NewValue().WithPath(meteoHydroPathSpeedOverGround).WithValue(5.0),
			))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// sailing straight into a 10 m/s wind at 5 m/s
			Expect(values[meteoHydroPathWindSpeedApparent]).To(BeNumerically("~", 15, 1e-9))
			Expect(values[meteoHydroPathWindAngleApparent]).To(BeNumerically("~", 0, 1e-9))
		})

		It("publishes the magnetic wind direction when the variation is known", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(meteoHydroPathMagneticVariation).WithValue(degrees(5)),
			))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values[meteoHydroPathWindDirectionMagnetic]).To(BeNumerically("~", degrees(40), 1e-9))

			// a direction never comes out negative, unlike an angle
			m.DoMap(navigationUpdate(now.Add(time.Second),
				message.NewValue().WithPath(meteoHydroPathMagneticVariation).WithValue(degrees(50)),
			))
			values = valuesByPath(tick(now.Add(70 * time.Second)))
			Expect(values[meteoHydroPathWindDirectionMagnetic]).To(BeNumerically("~", degrees(355), 1e-9))
		})

		It("publishes the wind chill of a cold day", func() {
			source.observation = &weatherObservation{timestamp: now, temperature: float(273.15), windSpeed: float(10), windDirection: float(0)}
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(meteoHydroPathHeadingTrue).WithValue(0.0),
				message.NewValue().WithPath(meteoHydroPathSpeedOverGround).WithValue(5.0),
				message.NewValue().WithPath(meteoHydroPathCourseOverGroundTrue).WithValue(0.0),
			))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// the theoretical wind chill follows the true wind, the
			// apparent one the stronger wind the moving vessel feels
			Expect(values).To(HaveKey(meteoHydroPathOutsideTheoreticalWindChill))
			Expect(values).To(HaveKey(meteoHydroPathOutsideApparentWindChill))
			Expect(values[meteoHydroPathOutsideApparentWindChill]).To(BeNumerically("<", values[meteoHydroPathOutsideTheoreticalWindChill]))
		})

		It("publishes the sea state at sea", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.5, 3.5),
				message.NewValue().WithPath(meteoHydroPathHeadingTrue).WithValue(degrees(240)),
				message.NewValue().WithPath(meteoHydroPathMagneticVariation).WithValue(degrees(2)),
			))
			prime(now)

			result := tick(now.Add(10 * time.Second))
			values := valuesByPath(result)

			Expect(values).To(HaveKeyWithValue(meteoHydroPathWavesHeight, 1.9))
			Expect(values[meteoHydroPathWavesDirection]).To(BeNumerically("~", degrees(300), 1e-9))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathWavesPeriod, 5.5))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathWindWavesHeight, 1.8))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathSwellHeight, 0.6))
			Expect(values[meteoHydroPathSwellDirection]).To(BeNumerically("~", degrees(220), 1e-9))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathWaterTemperature, 292.45))
			// the sea runs 60 degrees off the starboard bow
			Expect(values[meteoHydroPathWavesAngle]).To(BeNumerically("~", degrees(60), 1e-9))

			// environment.current is an object, not a scalar
			raw, ok := valueAtPath(result, meteoHydroPathCurrent)
			Expect(ok).To(BeTrue())
			current, ok := raw.(message.Current)
			Expect(ok).To(BeTrue())
			Expect(*current.Drift).To(BeNumerically("~", 0.35, 1e-9))
			Expect(*current.SetTrue).To(BeNumerically("~", degrees(90), 1e-9))
			Expect(*current.SetMagnetic).To(BeNumerically("~", degrees(88), 1e-9))
		})

		It("publishes no sea state where there is none", func() {
			// what the marine API answers for an inland waterway: a value
			// for nothing at all
			marine.observation = &marineObservation{timestamp: now}
			m.DoMap(navigationUpdate(now,
				positionValue(51.85, 5.85),
				message.NewValue().WithPath(meteoHydroPathHeadingTrue).WithValue(degrees(240)),
			))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// the atmospheric values are published as usual
			Expect(values).To(HaveKey(meteoHydroPathOutsideTemperature))
			Expect(values).To(HaveKey(meteoHydroPathWindSpeedApparent))
			// but nothing about a sea that is not there
			Expect(values).ToNot(HaveKey(meteoHydroPathWavesHeight))
			Expect(values).ToNot(HaveKey(meteoHydroPathWavesAngle))
			Expect(values).ToNot(HaveKey(meteoHydroPathWaterTemperature))
			_, ok := valueAtPath(tick(now.Add(30*time.Second)), meteoHydroPathCurrent)
			Expect(ok).To(BeFalse())
		})

		It("publishes the gusts and the visibility", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).To(HaveKeyWithValue(meteoHydroPathWindGust, 17.0))
			Expect(values).To(HaveKeyWithValue(meteoHydroPathOutsideVisibility, 24000.0))
		})
	})

	Describe("live sensor data", func() {
		// what a real instrument's value looks like on the wire: the
		// same path this mapper publishes, from somebody else's source
		sensorUpdate := func(at time.Time, path string, value float64) *message.Mapped {
			update := message.NewUpdate().
				WithSource(*message.NewSource().WithLabel("Wind").WithType("nmea0183")).
				WithTimestamp(at).
				AddValue(message.NewValue().WithPath(path).WithValue(value))
			return message.NewMapped().WithContext(meteoHydroTestContext).AddUpdate(update)
		}

		BeforeEach(func() {
			report(now)
			prime(now)
		})

		It("leaves a path an instrument is publishing alone", func() {
			m.DoMap(sensorUpdate(now, meteoHydroPathWindSpeedOverGround, 7.3))

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).ToNot(HaveKey(meteoHydroPathWindSpeedOverGround))
			// only that path, everything the instrument does not
			// report is still filled in
			Expect(values).To(HaveKey(meteoHydroPathOutsideTemperature))
			Expect(values).To(HaveKey(meteoHydroPathWindDirectionTrue))
		})

		It("takes the path back once the instrument goes quiet", func() {
			m.DoMap(sensorUpdate(now, meteoHydroPathWindSpeedOverGround, 7.3))
			Expect(valuesByPath(tick(now.Add(10 * time.Second)))).ToNot(HaveKey(meteoHydroPathWindSpeedOverGround))

			// still quiet, but not long enough to be given up on
			report(now.Add(time.Minute))
			Expect(valuesByPath(tick(now.Add(time.Minute)))).ToNot(HaveKey(meteoHydroPathWindSpeedOverGround))

			// gone for longer than the timeout, the model takes over
			report(now.Add(3 * time.Minute))
			Expect(valuesByPath(tick(now.Add(3 * time.Minute)))).To(HaveKeyWithValue(meteoHydroPathWindSpeedOverGround, 10.0))
		})

		It("defers again as soon as the instrument comes back", func() {
			report(now.Add(3 * time.Minute))
			Expect(valuesByPath(tick(now.Add(3 * time.Minute)))).To(HaveKey(meteoHydroPathWindSpeedOverGround))

			m.DoMap(sensorUpdate(now.Add(4*time.Minute), meteoHydroPathWindSpeedOverGround, 7.3))
			report(now.Add(4 * time.Minute))
			Expect(valuesByPath(tick(now.Add(4 * time.Minute)))).ToNot(HaveKey(meteoHydroPathWindSpeedOverGround))
		})

		It("does not defer to its own output coming back", func() {
			// what this mapper published a moment ago, returning to it
			// because it subscribes to every mapper on the vessel
			own := message.NewUpdate().
				WithSource(*message.NewSource().WithLabel("meteohydro").WithType(config.MeteoHydroType)).
				WithTimestamp(now).
				AddValue(message.NewValue().WithPath(meteoHydroPathWindSpeedOverGround).WithValue(10.0))
			m.DoMap(message.NewMapped().WithContext(meteoHydroTestContext).AddUpdate(own))

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).To(HaveKeyWithValue(meteoHydroPathWindSpeedOverGround, 10.0))
		})

		It("keeps using an instrument's navigation data while deferring", func() {
			// the wind sensor of a vessel that also reports its heading
			// over the same connector: the heading is an input, not an
			// output, so it is used rather than deferred to
			m.DoMap(sensorUpdate(now, meteoHydroPathWindDirectionTrue, degrees(200)))
			m.DoMap(navigationUpdate(now, message.NewValue().WithPath(meteoHydroPathHeadingTrue).WithValue(degrees(45))))

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).ToNot(HaveKey(meteoHydroPathWindDirectionTrue))
			// the apparent wind is this mapper's own path and is still
			// calculated, from the model's wind and the vessel heading
			Expect(values).To(HaveKey(meteoHydroPathWindSpeedApparent))
			Expect(values[meteoHydroPathWindAngleApparent]).To(BeNumerically("~", 0, 1e-9))
		})
	})

	Describe("publish interval", func() {
		JustBeforeEach(func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			prime(now)
		})

		It("publishes at most once per minimum publish interval while moving", func() {
			m.navigation.speedOverGround.set(6, now.Add(10*time.Second))
			Expect(tick(now.Add(10 * time.Second)).Updates).To(HaveLen(1))

			// 9 seconds later is too soon, 10 seconds later is not
			Expect(tick(now.Add(19 * time.Second)).Updates).To(BeEmpty())
			Expect(tick(now.Add(20 * time.Second)).Updates).To(HaveLen(1))
		})

		It("publishes slower while the vessel is not moving", func() {
			Expect(tick(now.Add(10 * time.Second)).Updates).To(HaveLen(1))

			Expect(tick(now.Add(50 * time.Second)).Updates).To(BeEmpty())
			Expect(tick(now.Add(70 * time.Second)).Updates).To(HaveLen(1))
		})

		It("publishes no faster than the navigation data arrives", func() {
			m.navigation.speedOverGround.set(6, now.Add(10*time.Second))
			Expect(tick(now.Add(10 * time.Second)).Updates).To(HaveLen(1))

			// positions arrive once every 30 seconds
			m.DoMap(navigationUpdate(now.Add(30*time.Second), positionValue(52.1, 4.2)))
			m.DoMap(navigationUpdate(now.Add(60*time.Second), positionValue(52.1, 4.2)))
			m.navigation.speedOverGround.set(6, now.Add(60*time.Second))

			Expect(m.publishInterval(now.Add(60 * time.Second))).To(Equal(30 * time.Second))
			Expect(tick(now.Add(35 * time.Second)).Updates).To(BeEmpty())
			Expect(tick(now.Add(40 * time.Second)).Updates).To(HaveLen(1))
		})

		It("never publishes faster than the minimum allowed interval", func() {
			c := meteoHydroTestConfig()
			c.MinPublishInterval = time.Second
			m = newMeteoHydroMapper(c, source, marine)
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			m.navigation.speedOverGround.set(6, now)
			prime(now)

			Expect(m.publishInterval(now)).To(BeNumerically(">=", config.MinAllowedPublishInterval))
		})
	})

	Describe("requests to the marine API", func() {
		It("stops asking for a sea state where there is none", func() {
			marine.observation = &marineObservation{timestamp: now}
			report(now)
			prime(now)
			Expect(marine.callCount()).To(Equal(1))
			Expect(source.callCount()).To(Equal(1))

			// an hour later the weather is refreshed, the sea state that
			// does not exist is not asked for again
			report(now.Add(time.Hour))
			tick(now.Add(time.Hour))
			Expect(source.callCount()).To(Equal(2))
			Expect(marine.callCount()).To(Equal(1))

			// only after the inland retry interval is it checked again,
			// a vessel could have moved from a canal onto open water
			report(now.Add(25 * time.Hour))
			tick(now.Add(25 * time.Hour))
			Expect(marine.callCount()).To(Equal(2))
		})

		It("refreshes a real sea state on its own, slower interval", func() {
			report(now)
			prime(now)
			Expect(marine.callCount()).To(Equal(1))

			// the weather is refreshed every 10 minutes, the sea state
			// every 30
			report(now.Add(11 * time.Minute))
			tick(now.Add(11 * time.Minute))
			Expect(source.callCount()).To(Equal(2))
			Expect(marine.callCount()).To(Equal(1))

			report(now.Add(31 * time.Minute))
			tick(now.Add(31 * time.Minute))
			Expect(marine.callCount()).To(Equal(2))
		})

		It("keeps publishing the weather when the marine API fails", func() {
			marine.err = fmt.Errorf("the marine API is down")
			report(now)
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).To(HaveKey(meteoHydroPathOutsideTemperature))
			Expect(values).ToNot(HaveKey(meteoHydroPathWavesHeight))
		})

		It("never asks at all when the sea state is turned off", func() {
			m = newMeteoHydroMapper(meteoHydroTestConfig(), source, nil)
			report(now)
			prime(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(marine.callCount()).To(BeZero())
			Expect(values).To(HaveKey(meteoHydroPathOutsideTemperature))
			Expect(values).ToNot(HaveKey(meteoHydroPathWavesHeight))
		})
	})

	Describe("requests to the weather API", func() {
		It("reuses a cached observation instead of asking again", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			m.navigation.speedOverGround.set(6, now)

			for second := 0; second < 300; second += 10 {
				tick(now.Add(time.Duration(second) * time.Second))
			}

			// one request, and 29 ticks served from the cache
			Expect(source.callCount()).To(Equal(1))
		})

		It("asks again once the cached observation needs a refresh", func() {
			report(now)
			prime(now)
			Expect(source.callCount()).To(Equal(1))

			report(now.Add(9 * time.Minute))
			tick(now.Add(9 * time.Minute))
			Expect(source.callCount()).To(Equal(1))

			report(now.Add(11 * time.Minute))
			tick(now.Add(11 * time.Minute))
			Expect(source.callCount()).To(Equal(2))
		})

		It("asks again once the vessel leaves the grid cell", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			prime(now)
			Expect(source.callCount()).To(Equal(1))

			// still the same cell
			m.DoMap(navigationUpdate(now.Add(time.Minute), positionValue(52.12, 4.21)))
			tick(now.Add(2 * time.Minute))
			Expect(source.callCount()).To(Equal(1))

			// a different cell
			m.DoMap(navigationUpdate(now.Add(3*time.Minute), positionValue(52.4, 4.2)))
			tick(now.Add(3 * time.Minute))
			Expect(source.callCount()).To(Equal(2))
		})

		It("keeps the minimum interval between two requests", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			prime(now)
			Expect(source.callCount()).To(Equal(1))

			// a vessel crossing cells faster than the minimum request
			// interval still only asks once per interval
			m.DoMap(navigationUpdate(now.Add(10*time.Second), positionValue(52.4, 4.2)))
			tick(now.Add(10 * time.Second))
			Expect(source.callCount()).To(Equal(1))

			m.DoMap(navigationUpdate(now.Add(70*time.Second), positionValue(52.7, 4.2)))
			tick(now.Add(70 * time.Second))
			Expect(source.callCount()).To(Equal(2))
		})

		It("backs off exponentially when the weather API fails", func() {
			source.fail(fmt.Errorf("the weather API is down"))
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))

			prime(now)
			Expect(source.callCount()).To(Equal(1))

			// the first retry is not before the minimum request interval
			tick(now.Add(30 * time.Second))
			Expect(source.callCount()).To(Equal(1))

			tick(now.Add(time.Minute))
			Expect(source.callCount()).To(Equal(2))

			// and the one after that waits longer than that again
			tick(now.Add(2 * time.Minute))
			Expect(source.callCount()).To(Equal(2))
		})

		It("keeps publishing the cached observation while the weather API is down", func() {
			c := meteoHydroTestConfig()
			c.MaxDataAge = 30 * time.Minute
			m = newMeteoHydroMapper(c, source, marine)

			report(now)
			prime(now)
			source.fail(fmt.Errorf("the weather API is down"))

			report(now.Add(5 * time.Minute))
			Expect(tick(now.Add(5 * time.Minute)).Updates).To(HaveLen(1))

			report(now.Add(20 * time.Minute))
			Expect(tick(now.Add(20 * time.Minute)).Updates).To(HaveLen(1))

			// until the observation is simply too old to be published
			report(now.Add(40 * time.Minute))
			Expect(tick(now.Add(40 * time.Minute)).Updates).To(BeEmpty())
		})
	})
})

var _ = Describe("openMeteoSource", func() {
	It("requests the current weather for a position and converts it to Signal K units", func() {
		var requested string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requested = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"latitude": 52.1, "longitude": 4.2,
				"current": {
					"time": 1789000200,
					"temperature_2m": 12.4,
					"relative_humidity_2m": 81,
					"dew_point_2m": 9.3,
					"surface_pressure": 1011.2,
					"pressure_msl": 1013.4,
					"visibility": 26720.0,
					"wind_speed_10m": 7.5,
					"wind_direction_10m": 225,
					"wind_gusts_10m": 14.2
				}
			}`)
		}))
		defer server.Close()

		observation, err := newOpenMeteoSource(server.URL, "", time.Second).fetch(context.Background(), 52.1, 4.2)

		Expect(err).ToNot(HaveOccurred())
		Expect(requested).To(ContainSubstring("latitude=52.1000"))
		Expect(requested).To(ContainSubstring("longitude=4.2000"))
		Expect(requested).To(ContainSubstring("wind_speed_unit=ms"))
		Expect(*observation.temperature).To(BeNumerically("~", 285.55, 1e-9))
		Expect(*observation.dewPoint).To(BeNumerically("~", 282.45, 1e-9))
		Expect(*observation.relativeHumidity).To(BeNumerically("~", 0.81, 1e-9))
		// the pressure at the vessel, not the one at sea level
		Expect(*observation.pressure).To(BeNumerically("~", 101120, 1e-6))
		Expect(*observation.windSpeed).To(BeNumerically("~", 7.5, 1e-9))
		Expect(*observation.windGust).To(BeNumerically("~", 14.2, 1e-9))
		Expect(*observation.visibility).To(BeNumerically("~", 26720, 1e-9))
		Expect(*observation.windDirection).To(BeNumerically("~", degrees(225), 1e-9))
		Expect(observation.timestamp).To(Equal(time.Unix(1789000200, 0).UTC()))
	})

	It("falls back on the sea level pressure", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"current": {"time": 1789000200, "temperature_2m": 12.4, "pressure_msl": 1013.4}}`)
		}))
		defer server.Close()

		observation, err := newOpenMeteoSource(server.URL, "", time.Second).fetch(context.Background(), 52.1, 4.2)

		Expect(err).ToNot(HaveOccurred())
		Expect(*observation.pressure).To(BeNumerically("~", 101340, 1e-6))
	})

	It("reports the reason a request was rejected", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": true, "reason": "Latitude must be in range of -90 to 90"}`)
		}))
		defer server.Close()

		_, err := newOpenMeteoSource(server.URL, "", time.Second).fetch(context.Background(), 152.1, 4.2)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Latitude must be in range"))
	})

	It("passes a configured model through to the API", func() {
		var requested string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requested = r.URL.RawQuery
			fmt.Fprint(w, `{"current": {"time": 1789000200, "temperature_2m": 12.4}}`)
		}))
		defer server.Close()

		_, err := newOpenMeteoSource(server.URL, "knmi_seamless", time.Second).fetch(context.Background(), 52.1, 4.2)

		Expect(err).ToNot(HaveOccurred())
		Expect(requested).To(ContainSubstring("models=knmi_seamless"))
	})

	It("treats a response without any usable value as a failed request", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"current": {"time": 1789000200}}`)
		}))
		defer server.Close()

		_, err := newOpenMeteoSource(server.URL, "", time.Second).fetch(context.Background(), 52.1, 4.2)

		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("openMeteoMarineSource", func() {
	It("requests the sea state and converts it to Signal K units", func() {
		var requested string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requested = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"latitude": 52.5, "longitude": 3.5, "elevation": 0.0,
				"current": {
					"time": 1789000200,
					"wave_height": 1.92,
					"wave_direction": 294,
					"wave_period": 5.55,
					"wind_wave_height": 1.80,
					"wind_wave_direction": 298,
					"wind_wave_period": 4.95,
					"swell_wave_height": 0.58,
					"swell_wave_direction": 221,
					"swell_wave_period": 4.70,
					"sea_surface_temperature": 19.3,
					"ocean_current_velocity": 0.72,
					"ocean_current_direction": 90
				}
			}`)
		}))
		defer server.Close()

		observation, err := newOpenMeteoMarineSource(server.URL, "", time.Second).fetch(context.Background(), 52.5, 3.5)

		Expect(err).ToNot(HaveOccurred())
		Expect(requested).To(ContainSubstring("latitude=52.5000"))
		Expect(observation.isEmpty()).To(BeFalse())
		Expect(*observation.waveHeight).To(BeNumerically("~", 1.92, 1e-9))
		Expect(*observation.waveDirection).To(BeNumerically("~", degrees(294), 1e-9))
		Expect(*observation.wavePeriod).To(BeNumerically("~", 5.55, 1e-9))
		Expect(*observation.windWaveDirection).To(BeNumerically("~", degrees(298), 1e-9))
		Expect(*observation.swellHeight).To(BeNumerically("~", 0.58, 1e-9))
		Expect(*observation.seaSurfaceTemperature).To(BeNumerically("~", 292.45, 1e-9))
		// the marine API ignores a velocity unit and always answers in
		// km/h, 0.72 km/h is 0.2 m/s
		Expect(*observation.currentSpeed).To(BeNumerically("~", 0.2, 1e-9))
		Expect(*observation.currentDirection).To(BeNumerically("~", degrees(90), 1e-9))
	})

	It("returns an empty observation, not an error, away from the sea", func() {
		// what the API answers for a position on an inland waterway
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"latitude": 51.875, "longitude": 5.875, "elevation": 11.0,
				"current": {"time": 1789000200, "wave_height": null, "sea_surface_temperature": null}}`)
		}))
		defer server.Close()

		observation, err := newOpenMeteoMarineSource(server.URL, "", time.Second).fetch(context.Background(), 51.85, 5.85)

		Expect(err).ToNot(HaveOccurred())
		Expect(observation.isEmpty()).To(BeTrue())
	})

	It("reports the reason a request was rejected", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": true, "reason": "Cannot initialize WeatherVariable from invalid String value"}`)
		}))
		defer server.Close()

		_, err := newOpenMeteoMarineSource(server.URL, "", time.Second).fetch(context.Background(), 52.5, 3.5)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Cannot initialize WeatherVariable"))
	})
})
