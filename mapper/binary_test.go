package mapper_test

import (
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DoMap binary", func() {
	sensorHealthCheck := []*config.NotificationMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "notifications.propulsion.mainEngine.drive.torque.sensorHealth",
				Expression: "abs(float(toUInt(value[0], value[1])) - float(toUInt(value[2], value[3]))) > 10",
			},
			Message: "the two torque sensors disagree",
			State:   "alarm",
		},
	}
	raw := func(sensor1, sensor2 uint16) *message.Raw {
		return message.NewRaw().WithConnector("Shaft Power Meter").WithValue([]byte{
			byte(sensor1 >> 8), byte(sensor1),
			byte(sensor2 >> 8), byte(sensor2),
		})
	}
	rawAt := func(sensor1, sensor2 uint16, t time.Time) *message.Raw {
		r := raw(sensor1, sensor2)
		r.Timestamp = t
		return r
	}

	It("still maps regular values alongside the notification check", func() {
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{
				{Path: "propulsion.mainEngine.drive.torque.sensor1", Expression: "toUInt(value[0], value[1])"},
			},
			sensorHealthCheck,
		)
		Expect(err).ToNot(HaveOccurred())

		out, err := m.DoMap(raw(100, 100))
		Expect(err).ToNot(HaveOccurred())

		// two values: the regular mapping, plus the notification check's
		// very first evaluation, which always confirms and publishes its
		// baseline state (here: cleared, since the readings agree)
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(2))
		var sensor1 *message.SingleValueMapped
		for i, v := range values {
			if v.Path == "propulsion.mainEngine.drive.torque.sensor1" {
				sensor1 = &values[i]
			}
		}
		Expect(sensor1).ToNot(BeNil())
		Expect(sensor1.Value).To(Equal(uint16(100)))
	})

	It("raises the sensor health notification once the two readings diverge, without ever publishing either reading", func() {
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{}, // no plain mappings: sensor1/sensor2 are never published
			sensorHealthCheck,
		)
		Expect(err).ToNot(HaveOccurred())

		out, err := m.DoMap(raw(100, 200))
		Expect(err).ToNot(HaveOccurred())

		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(values[0].Path).To(Equal("notifications.propulsion.mainEngine.drive.torque.sensorHealth"))
		notification, ok := values[0].Value.(message.Notification)
		Expect(ok).To(BeTrue())
		Expect(*notification.State).To(Equal("alarm"))
		Expect(*notification.Message).To(Equal("the two torque sensors disagree"))
	})

	It("does not republish the notification while its state stays the same", func() {
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{},
			sensorHealthCheck,
		)
		Expect(err).ToNot(HaveOccurred())

		first, err := m.DoMap(raw(100, 200))
		Expect(err).ToNot(HaveOccurred())
		Expect(first.Updates).ToNot(BeEmpty())

		// still diverging and unchanged: nothing to publish, and since
		// there are no plain mappings configured either, DoMap reports it
		// the same way it always has when a raw message maps to nothing
		second, err := m.DoMap(raw(101, 201))
		Expect(err).To(HaveOccurred())
		Expect(second).To(BeNil())
	})

	It("clears the notification once the readings agree again", func() {
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{},
			sensorHealthCheck,
		)
		Expect(err).ToNot(HaveOccurred())

		_, err = m.DoMap(raw(100, 200))
		Expect(err).ToNot(HaveOccurred())

		out, err := m.DoMap(raw(100, 100))
		Expect(err).ToNot(HaveOccurred())

		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(values[0].Path).To(Equal("notifications.propulsion.mainEngine.drive.torque.sensorHealth"))
		Expect(values[0].Value).To(BeNil())
	})

	It("errors when neither a mapping nor a notification produced anything", func() {
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{
				{Path: "some.path", Expression: "value[99]"}, // out of range: always fails
			},
			nil,
		)
		Expect(err).ToNot(HaveOccurred())

		_, err = m.DoMap(message.NewRaw().WithConnector("test").WithValue([]byte{1, 2}))
		Expect(err).To(HaveOccurred())
	})

	It("respects SetDelay: a divergence has to persist before the notification confirms", func() {
		delayedCheck := []*config.NotificationMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "notifications.propulsion.mainEngine.drive.torque.sensorHealth",
					Expression: "abs(float(toUInt(value[0], value[1])) - float(toUInt(value[2], value[3]))) > 10",
				},
				Message:  "the two torque sensors disagree",
				State:    "alarm",
				SetDelay: time.Minute,
			},
		}
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{},
			delayedCheck,
		)
		Expect(err).ToNot(HaveOccurred())
		t0 := time.Now()

		// the first observation always confirms immediately (see
		// applyHysteresis: !state.hasConfirmedState), establishing a
		// confirmed "not notifying" baseline rather than testing the delay
		_, err = m.DoMap(rawAt(100, 100, t0))
		Expect(err).ToNot(HaveOccurred())

		// diverges, but SetDelay hasn't elapsed yet: no update at all
		out, err := m.DoMap(rawAt(100, 200, t0.Add(time.Second)))
		Expect(err).To(HaveOccurred())
		Expect(out).To(BeNil())

		// still diverging, now well past SetDelay since the divergence
		// started: confirms and publishes the alarm
		out, err = m.DoMap(rawAt(100, 200, t0.Add(90*time.Second)))
		Expect(err).ToNot(HaveOccurred())
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(*values[0].Value.(message.Notification).State).To(Equal("alarm"))
	})

	It("feeds a peak-to-peak estimate into env for a notification to cross-check against a reference reading", func() {
		// bytes 0-1 and 2-3: two sensor readings whose difference is the
		// periodic signal to estimate speed from (see peakToPeakRpm), by
		// timing consecutive local peaks/troughs - these are the exact
		// bytes mannerSensorDiffExpression is hardcoded to read. bytes 6-7:
		// a reference reading (Hz * 100) to compare the estimate against -
		// standing in for e.g. the frame's own RPM field.
		rpmFrame := func(diff int, referenceHzTimes100 uint16, t time.Time) *message.Raw {
			sensor2 := uint16(100)
			sensor1 := uint16(int(sensor2) + diff)
			r := message.NewRaw().WithConnector("Shaft Power Meter").WithValue([]byte{
				0, 0,
				byte(sensor1 >> 8), byte(sensor1),
				byte(sensor2 >> 8), byte(sensor2),
				byte(referenceHzTimes100 >> 8), byte(referenceHzTimes100),
			})
			r.Timestamp = t
			return r
		}
		mismatchCheck := []*config.NotificationMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "notifications.propulsion.mainEngine.drive.revolutions.mismatch",
					Expression: "abs(estimatedRevolutions - float(toUInt(value[6], value[7]))/100.0) > 1",
				},
				When:    "estimatedRevolutions > 0",
				Message: "the estimated and reported speed disagree",
				State:   "alarm",
			},
		}
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{},
			mismatchCheck,
		)
		Expect(err).ToNot(HaveOccurred())
		t0 := time.Now()

		// rises to a peak (10ms), falls to a trough (30ms), rises again
		// (40ms): the second reversal is the first one with a prior
		// extremum to time against, so the When gate blocks every frame
		// until then
		_, err = m.DoMap(rpmFrame(10, 500, t0))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(20, 500, t0.Add(10*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(15, 500, t0.Add(20*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(5, 500, t0.Add(30*time.Millisecond)))
		Expect(err).To(HaveOccurred())

		// the peak (10ms) and trough (30ms) are 20ms apart, one half
		// revolution: estimatedHz becomes 0.5/0.02 = 25Hz, which disagrees
		// with a reference reading of 50Hz by more than the 1Hz threshold
		out, err := m.DoMap(rpmFrame(10, 5000, t0.Add(40*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(values[0].Path).To(Equal("notifications.propulsion.mainEngine.drive.revolutions.mismatch"))
		Expect(*values[0].Value.(message.Notification).State).To(Equal("alarm"))
	})

	It("discards an implausible reading instead of letting it corrupt the estimate", func() {
		rpmFrame := func(diff int, t time.Time) *message.Raw {
			sensor2 := uint16(100)
			sensor1 := uint16(int(sensor2) + diff)
			r := message.NewRaw().WithConnector("Shaft Power Meter").WithValue([]byte{
				0, 0,
				byte(sensor1 >> 8), byte(sensor1),
				byte(sensor2 >> 8), byte(sensor2),
			})
			r.Timestamp = t
			return r
		}
		// alarms if estimatedRevolutions is anywhere but around the 5Hz
		// the sequence below establishes, so an unexpected value - e.g.
		// from the implausible reading corrupting the estimate instead of
		// being discarded - shows up as this firing when it shouldn't
		sanityCheck := []*config.NotificationMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "notifications.propulsion.mainEngine.drive.revolutions.sanity",
					Expression: "abs(estimatedRevolutions - 5) > 1",
				},
				When:    "estimatedRevolutions > 0",
				Message: "estimatedRevolutions drifted away from the expected 5Hz",
				State:   "alarm",
			},
		}
		m, err := NewBinaryMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.MappingConfig{},
			sanityCheck,
		)
		Expect(err).ToNot(HaveOccurred())
		t0 := time.Now()

		// two full 100ms half-periods (peak at 95ms, trough at 195ms, peak
		// at 295ms), each confirmed by a sample shortly after: establishes
		// and then reconfirms an estimate of 0.5/0.1 = 5Hz
		_, err = m.DoMap(rpmFrame(10, t0))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(20, t0.Add(95*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(15, t0.Add(100*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(5, t0.Add(195*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(10, t0.Add(200*time.Millisecond))) // estimate now 5Hz
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(20, t0.Add(295*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		out, err := m.DoMap(rpmFrame(15, t0.Add(300*time.Millisecond))) // reconfirms 5Hz
		Expect(err).To(HaveOccurred())
		Expect(out).To(BeNil())

		// a blip only 5ms after the last real peak (295ms): if accepted at
		// face value this implies 0.5/0.005 = 100Hz, 20x the established
		// estimate, which isPlausible rejects - so estimatedHz must still
		// be ~5Hz and the sanity check must not fire
		_, err = m.DoMap(rpmFrame(20, t0.Add(305*time.Millisecond)))
		Expect(err).To(HaveOccurred())
	})

	It("falls back to the single combined torque reading when only one sensor is installed", func() {
		// bytes 2-3/4-5 (the two-sensor pair) are held constant, so if the
		// mapper wrongly used their difference instead of falling back,
		// estimatedRevolutions would never leave 0 and this check would
		// never fire. bytes 10-11 carry the varying single-sensor signal.
		rpmFrame := func(torque int, t time.Time) *message.Raw {
			value := uint16(100 + torque)
			r := message.NewRaw().WithConnector("Shaft Power Meter").WithValue([]byte{
				0, 0,
				0, 100, // sensor1, constant
				0, 100, // sensor2, constant
				0, 0,
				0, 0,
				byte(value >> 8), byte(value),
			})
			r.Timestamp = t
			return r
		}
		mismatchCheck := []*config.NotificationMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "notifications.propulsion.mainEngine.drive.revolutions.mismatch",
					Expression: "abs(estimatedRevolutions - 50) > 1",
				},
				When:    "estimatedRevolutions > 0",
				Message: "the estimated and reported speed disagree",
				State:   "alarm",
			},
		}
		m, err := NewBinaryMapper(
			config.MapperConfig{
				Context:         "testingContext",
				ProtocolOptions: map[string]string{config.ProtocolOptionBinarySingleTorqueSensor: "true"},
			},
			[]config.MappingConfig{},
			mismatchCheck,
		)
		Expect(err).ToNot(HaveOccurred())
		t0 := time.Now()

		// same peak (10ms), trough (30ms) sequence as the two-sensor test,
		// just carried by bytes 10-11 instead of bytes 2-5: the estimate
		// becomes 0.5/0.02 = 25Hz, which disagrees with 50 by more than 1
		_, err = m.DoMap(rpmFrame(10, t0))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(20, t0.Add(10*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(15, t0.Add(20*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		_, err = m.DoMap(rpmFrame(5, t0.Add(30*time.Millisecond)))
		Expect(err).To(HaveOccurred())
		out, err := m.DoMap(rpmFrame(10, t0.Add(40*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(*values[0].Value.(message.Notification).State).To(Equal("alarm"))
	})
})
