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

func float(value float64) *float64 {
	return &value
}

func degrees(value float64) float64 {
	return value * math.Pi / 180
}

const weatherTestContext = "vessels.urn:mrn:imo:mmsi:123456789"

func weatherTestConfig() config.WeatherMapperConfig {
	c := config.DefaultWeatherMapperConfig()
	c.Context = weatherTestContext
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
		windDirection:    float(degrees(45)),
	}
}

func navigationUpdate(timestamp time.Time, values ...*message.Value) *message.Mapped {
	update := message.NewUpdate().WithTimestamp(timestamp)
	for _, value := range values {
		update.AddValue(value)
	}
	return message.NewMapped().WithContext(weatherTestContext).AddUpdate(update)
}

func positionValue(latitude float64, longitude float64) *message.Value {
	return message.NewValue().WithPath(weatherPathPosition).WithValue(message.Position{Latitude: &latitude, Longitude: &longitude})
}

// valuesByPath collapses an update into a path to value map, every value
// this mapper publishes is a float64 on a unique path.
func valuesByPath(mapped *message.Mapped) map[string]float64 {
	result := make(map[string]float64)
	for _, update := range mapped.Updates {
		for _, value := range update.Values {
			result[value.Path] = value.Value.(float64)
		}
	}
	return result
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
		state.update(message.SingleValueMapped{Path: weatherPathPosition, Timestamp: now, Value: message.Position{Latitude: float(52), Longitude: float(4)}})
		Expect(state.interval).To(BeZero())

		state.update(message.SingleValueMapped{Path: weatherPathPosition, Timestamp: now.Add(30 * time.Second), Value: message.Position{Latitude: float(52), Longitude: float(4)}})
		Expect(state.interval).To(Equal(30 * time.Second))

		// a repeated timestamp says nothing about the rate
		state.update(message.SingleValueMapped{Path: weatherPathPosition, Timestamp: now.Add(30 * time.Second), Value: message.Position{Latitude: float(52), Longitude: float(4)}})
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

var _ = Describe("weatherCache", func() {
	var now time.Time

	BeforeEach(func() {
		now = time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	})

	It("serves every position within the same grid cell", func() {
		cache := newWeatherCache(0.1, time.Hour, 10)
		cache.put(52.01, 4.01, now, fullObservation(now))

		_, ok := cache.get(52.02, 4.02, now)
		Expect(ok).To(BeTrue())

		_, ok = cache.get(52.4, 4.01, now)
		Expect(ok).To(BeFalse())
	})

	It("does not serve an observation that aged past the maximum age", func() {
		cache := newWeatherCache(0.1, time.Hour, 10)
		cache.put(52, 4, now, fullObservation(now))

		_, ok := cache.get(52, 4, now.Add(59*time.Minute))
		Expect(ok).To(BeTrue())

		_, ok = cache.get(52, 4, now.Add(61*time.Minute))
		Expect(ok).To(BeFalse())
	})

	It("keeps at most the configured number of cells, dropping the oldest", func() {
		cache := newWeatherCache(0.1, time.Hour, 2)
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

var _ = Describe("WeatherMapper", func() {
	var now time.Time
	var source *fakeWeatherSource
	var m *WeatherMapper

	// tick runs a single evaluation and waits for any request it started
	// to finish, so the next tick sees the result of that request.
	tick := func(at time.Time) *message.Mapped {
		before := source.callCount()
		result := m.refreshMap(at)
		Eventually(func() bool {
			m.fetchMutex.Lock()
			defer m.fetchMutex.Unlock()
			return !m.fetching
		}).Should(BeTrue())
		Expect(source.callCount()).To(BeNumerically(">=", before))
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
		m = newWeatherMapper(weatherTestConfig(), source)
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

			Expect(result.Context).To(Equal(weatherTestContext))
			Expect(result.Updates).To(HaveLen(1))
			Expect(result.Updates[0].Source.Type).To(Equal(config.WeatherType))
			values := valuesByPath(result)
			Expect(values).To(HaveKeyWithValue(weatherPathOutsideTemperature, 283.15))
			Expect(values).To(HaveKeyWithValue(weatherPathOutsideDewPoint, 278.15))
			Expect(values).To(HaveKeyWithValue(weatherPathOutsideRelativeHumidity, 0.72))
			Expect(values).To(HaveKeyWithValue(weatherPathOutsidePressure, 101300.0))
			Expect(values).To(HaveKeyWithValue(weatherPathWindSpeedOverGround, 10.0))
			Expect(values).To(HaveKeyWithValue(weatherPathWindDirectionTrue, degrees(45)))
		})

		It("skips the apparent values without a heading and without movement", func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			tick(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values).To(HaveKey(weatherPathWindDirectionTrue))
			Expect(values).ToNot(HaveKey(weatherPathWindAngleApparent))
			Expect(values).ToNot(HaveKey(weatherPathWindSpeedApparent))
			Expect(values).ToNot(HaveKey(weatherPathWindAngleTrueGround))
		})

		It("calculates the apparent values against the heading of a vessel that is not moving", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(weatherPathHeadingTrue).WithValue(degrees(45)),
				message.NewValue().WithPath(weatherPathSpeedOverGround).WithValue(0.0),
				// a course over ground that must be ignored at this speed
				message.NewValue().WithPath(weatherPathCourseOverGroundTrue).WithValue(degrees(180)),
			))
			tick(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// the wind comes from dead ahead and the vessel adds nothing
			// to it
			Expect(values).To(HaveKeyWithValue(weatherPathWindSpeedApparent, BeNumerically("~", 10, 1e-9)))
			Expect(values[weatherPathWindAngleApparent]).To(BeNumerically("~", 0, 1e-9))
			Expect(values[weatherPathWindAngleTrueGround]).To(BeNumerically("~", 0, 1e-9))
		})

		It("adds the movement of the vessel to the apparent wind", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(weatherPathHeadingTrue).WithValue(degrees(45)),
				message.NewValue().WithPath(weatherPathCourseOverGroundTrue).WithValue(degrees(45)),
				message.NewValue().WithPath(weatherPathSpeedOverGround).WithValue(5.0),
			))
			tick(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// sailing straight into a 10 m/s wind at 5 m/s
			Expect(values[weatherPathWindSpeedApparent]).To(BeNumerically("~", 15, 1e-9))
			Expect(values[weatherPathWindAngleApparent]).To(BeNumerically("~", 0, 1e-9))
		})

		It("publishes the magnetic wind direction when the variation is known", func() {
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(weatherPathMagneticVariation).WithValue(degrees(5)),
			))
			tick(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			Expect(values[weatherPathWindDirectionMagnetic]).To(BeNumerically("~", degrees(40), 1e-9))
		})

		It("publishes the wind chill of a cold day", func() {
			source.observation = &weatherObservation{timestamp: now, temperature: float(273.15), windSpeed: float(10), windDirection: float(0)}
			m.DoMap(navigationUpdate(now,
				positionValue(52.1, 4.2),
				message.NewValue().WithPath(weatherPathHeadingTrue).WithValue(0.0),
				message.NewValue().WithPath(weatherPathSpeedOverGround).WithValue(5.0),
				message.NewValue().WithPath(weatherPathCourseOverGroundTrue).WithValue(0.0),
			))
			tick(now)

			values := valuesByPath(tick(now.Add(10 * time.Second)))

			// the theoretical wind chill follows the true wind, the
			// apparent one the stronger wind the moving vessel feels
			Expect(values).To(HaveKey(weatherPathOutsideTheoreticalWindChill))
			Expect(values).To(HaveKey(weatherPathOutsideApparentWindChill))
			Expect(values[weatherPathOutsideApparentWindChill]).To(BeNumerically("<", values[weatherPathOutsideTheoreticalWindChill]))
		})
	})

	Describe("publish interval", func() {
		JustBeforeEach(func() {
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			tick(now)
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
			c := weatherTestConfig()
			c.MinPublishInterval = time.Second
			m = newWeatherMapper(c, source)
			m.DoMap(navigationUpdate(now, positionValue(52.1, 4.2)))
			m.navigation.speedOverGround.set(6, now)
			tick(now)

			Expect(m.publishInterval(now)).To(BeNumerically(">=", config.MinAllowedPublishInterval))
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
			tick(now)
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
			tick(now)
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
			tick(now)
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

			tick(now)
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
			c := weatherTestConfig()
			c.MaxDataAge = 30 * time.Minute
			m = newWeatherMapper(c, source)

			report(now)
			tick(now)
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
					"wind_speed_10m": 7.5,
					"wind_direction_10m": 225
				}
			}`)
		}))
		defer server.Close()

		observation, err := newOpenMeteoSource(server.URL, time.Second).fetch(context.Background(), 52.1, 4.2)

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
		Expect(*observation.windDirection).To(BeNumerically("~", degrees(225), 1e-9))
		Expect(observation.timestamp).To(Equal(time.Unix(1789000200, 0).UTC()))
	})

	It("falls back on the sea level pressure", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"current": {"time": 1789000200, "temperature_2m": 12.4, "pressure_msl": 1013.4}}`)
		}))
		defer server.Close()

		observation, err := newOpenMeteoSource(server.URL, time.Second).fetch(context.Background(), 52.1, 4.2)

		Expect(err).ToNot(HaveOccurred())
		Expect(*observation.pressure).To(BeNumerically("~", 101340, 1e-6))
	})

	It("reports the reason a request was rejected", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": true, "reason": "Latitude must be in range of -90 to 90"}`)
		}))
		defer server.Close()

		_, err := newOpenMeteoSource(server.URL, time.Second).fetch(context.Background(), 152.1, 4.2)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Latitude must be in range"))
	})

	It("treats a response without any usable value as a failed request", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"current": {"time": 1789000200}}`)
		}))
		defer server.Close()

		_, err := newOpenMeteoSource(server.URL, time.Second).fetch(context.Background(), 52.1, 4.2)

		Expect(err).To(HaveOccurred())
	})
})
