package mapper_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.einride.tech/can"
)

// canbusTestDBC mirrors the shape dbc.nix generates for a single engine
// (message IDs and signal bit layout match Raw_Data_0/TelMA_Data_0
// exactly), simplified to raw, unscaled digits since the tests only care
// about relative values, not physical units.
const canbusTestDBC = `VERSION ""

NS_ :
	NS_DESC_
	CM_
	BA_DEF_
	BA_
	VAL_
	CAT_DEF_
	CAT_
	FILTER
	BA_DEF_DEF_
	EV_DATA_
	ENVVAR_DATA_
	SGTYPE_
	SGTYPE_VAL_
	BA_DEF_SGTYPE_
	BA_SGTYPE_
	SIG_TYPE_REF_
	VAL_TABLE_
	SIG_GROUP_
	SIG_VALTYPE_
	SIGTYPE_VALTYPE_
	BO_TX_BU_
	BA_DEF_REL_
	BA_REL_
	BA_DEF_DEF_REL_
	BU_SG_REL_
	BU_EV_REL_
	BU_BO_REL_
	SG_MUL_VAL_

BS_:

BU_: TelMA_Node

BO_ 256 TelMA_Data_0: 8 TelMA_Node
	SG_ RPM : 39|16@0+ (1,0) [0|10000] "min-1" Vector__XXX

BO_ 257 Raw_Data_0: 8 Rohdaten
	SG_ Torque_Rotor_Module_2_Raw_Data : 39|16@0+ (1,0) [0|65535] "Digits" Vector__XXX
	SG_ Torque_Rotor_Module_1_Raw_Data : 23|16@0+ (1,0) [0|65535] "Digits" Vector__XXX
	SG_ Torque_Sum_Raw_Data : 7|16@0+ (1,0) [0|65535] "Digits" Vector__XXX
`

// writeCanbusTestDBC writes canbusTestDBC to a temp file and returns its
// path.
func writeCanbusTestDBC() string {
	path := filepath.Join(GinkgoT().TempDir(), "test.dbc")
	Expect(os.WriteFile(path, []byte(canbusTestDBC), 0o644)).To(Succeed())
	return path
}

// rawDataFrame builds a message.Raw carrying a Raw_Data_0 CAN frame (id
// 257) with the two rotor modules and their sum set to the given raw
// digits, at timestamp t.
func rawDataFrame(module1, module2, sum uint64, t time.Time) *message.Raw {
	frame := can.Frame{ID: 257, Length: 8}
	frame.Data.SetUnsignedBitsBigEndian(7, 16, sum)
	frame.Data.SetUnsignedBitsBigEndian(23, 16, module1)
	frame.Data.SetUnsignedBitsBigEndian(39, 16, module2)
	r := message.NewRaw().WithConnector("Shaft Power Meter").WithValue([]byte(frame.JSON()))
	r.Timestamp = t
	return r
}

// telmaDataFrame builds a message.Raw carrying a TelMA_Data_0 CAN frame
// (id 256) with the given RPM, at timestamp t.
func telmaDataFrame(rpm uint64, t time.Time) *message.Raw {
	frame := can.Frame{ID: 256, Length: 8}
	frame.Data.SetUnsignedBitsBigEndian(39, 16, rpm)
	r := message.NewRaw().WithConnector("Shaft Power Meter").WithValue([]byte(frame.JSON()))
	r.Timestamp = t
	return r
}

var _ = Describe("DoMap canbus", func() {
	It("raises the sensor health notification only once both sensors read nonzero", func() {
		healthCheck := []*config.NotificationMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "notifications.propulsion.mainEngine.drive.torque.sensorHealth",
					Expression: "abs(Raw_Data_0_Torque_Rotor_Module_1_Raw_Data - Raw_Data_0_Torque_Rotor_Module_2_Raw_Data) > 10",
				},
				When:    "Raw_Data_0_Torque_Rotor_Module_1_Raw_Data != 0 && Raw_Data_0_Torque_Rotor_Module_2_Raw_Data != 0",
				Message: "the two torque sensors disagree",
				State:   "alarm",
			},
		}
		m, err := NewCanBusMapper(
			config.CanBusMapperConfig{
				MapperConfig: config.MapperConfig{Context: "testingContext"},
				DbcFile:      writeCanbusTestDBC(),
			},
			nil,
			healthCheck,
		)
		Expect(err).ToNot(HaveOccurred())
		t0 := time.Now()

		// single-sensor case: module2 is 0, so the When gate blocks the
		// check even though module1 and module2 obviously disagree
		out, err := m.DoMap(rawDataFrame(500, 0, 500, t0))
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Updates).To(BeEmpty())

		// both sensors now nonzero and disagreeing by more than 10: fires
		out, err = m.DoMap(rawDataFrame(500, 100, 600, t0.Add(time.Second)))
		Expect(err).ToNot(HaveOccurred())
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(values[0].Path).To(Equal("notifications.propulsion.mainEngine.drive.torque.sensorHealth"))
		Expect(*values[0].Value.(message.Notification).State).To(Equal("alarm"))
	})

	It("cross-checks RPM against a peak-to-peak estimate derived from a different CAN message", func() {
		crossCheck := []*config.NotificationMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "notifications.propulsion.mainEngine.drive.revolutions.crossCheck",
					Expression: "abs(Raw_Data_0_estimatedRevolutions - TelMA_Data_0_RPM / 60.0) > 1",
				},
				When:    "Raw_Data_0_estimatedRevolutions > 0 && TelMA_Data_0_RPM > 0",
				Message: "reported RPM disagrees with the shaft power meter's own sensors",
				State:   "alarm",
			},
		}
		m, err := NewCanBusMapper(
			config.CanBusMapperConfig{
				MapperConfig: config.MapperConfig{Context: "testingContext"},
				DbcFile:      writeCanbusTestDBC(),
			},
			nil,
			crossCheck,
		)
		Expect(err).ToNot(HaveOccurred())
		t0 := time.Now()

		// establishes TelMA_Data_0_RPM = 0 from the start: an *undefined*
		// env var makes the When expression itself fail to evaluate
		// (expr-lang errors on comparing nil, it doesn't just read as
		// false), which the fail-safe design then treats as "notification
		// needed" - very different from a defined value of 0. Since RPM
		// and torque arrive on separate CAN messages, nothing else
		// guarantees RPM is defined this early the way BinaryMapper (one
		// message per frame) guarantees its own equivalent always is. This
		// one call briefly fail-safes on the still-undefined estimate (see
		// the "Could not evaluate" warnings this test logs) and confirms
		// notifying - but self-corrects the instant the first Raw_Data
		// frame below defines the estimate (even at 0, that's a real,
		// non-erroring value, so applies briefly flips to a defined false,
		// which itself clears the fail-safe alarm) - well before any of
		// the assertions below run.
		_, err = m.DoMap(telmaDataFrame(0, t0))
		Expect(err).ToNot(HaveOccurred())

		// a peak (10ms) then trough (30ms) on Torque_Sum, half a
		// revolution apart, confirmed by a 5th sample: estimatedRevolutions
		// becomes 0.5/0.02 = 25Hz (see the "DoMap binary" peak-to-peak
		// tests for the same math walked through sample by sample)
		_, err = m.DoMap(rawDataFrame(0, 0, 10, t0.Add(1*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		_, err = m.DoMap(rawDataFrame(0, 0, 20, t0.Add(11*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		_, err = m.DoMap(rawDataFrame(0, 0, 15, t0.Add(21*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		_, err = m.DoMap(rawDataFrame(0, 0, 5, t0.Add(31*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		out, err := m.DoMap(rawDataFrame(0, 0, 10, t0.Add(41*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		Expect(out.Updates).To(BeEmpty()) // RPM still 0 (engine "not running"): When gate blocks

		// a separate CAN message (TelMA_Data_0) reports 3000 RPM = 50Hz,
		// disagreeing with the 25Hz estimate derived above by more than 1
		out, err = m.DoMap(telmaDataFrame(3000, t0.Add(50*time.Millisecond)))
		Expect(err).ToNot(HaveOccurred())
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(values[0].Path).To(Equal("notifications.propulsion.mainEngine.drive.revolutions.crossCheck"))
		Expect(*values[0].Value.(message.Notification).State).To(Equal("alarm"))
	})

	It("lets a mapping expression reference another signal's persistent value, decoded from the same frame", func() {
		// mirrors mappingsForMannerCanbusBendingMoment in ../nix: it
		// triggers on Torque_Rotor_Module_1_Raw_Data (using "value") and
		// reaches across to Torque_Rotor_Module_2_Raw_Data's persistent
		// env value, relying on Module_2 being decoded first within the
		// same Raw_Data_0 frame (see dbc.nix's field order), scaling the
		// raw digit difference by rated torque the same way
		// mappingsForMannerTcpBendingMoment scales its raw byte
		// difference. A rated torque of 32768 (a nonsense unit but a
		// convenient number) makes the expected result easy to check by
		// hand: (600-100)/32768*1.1111111*32768 = 500*1.1111111.
		bendingMoment := []config.CanBusMappingConfig{
			{
				Name:   "Torque_Rotor_Module_1_Raw_Data",
				Origin: "Raw_Data_0",
				MappingConfig: config.MappingConfig{
					Path:       "propulsion.mainEngine.drive.bendingMoment",
					Expression: "(value - Raw_Data_0_Torque_Rotor_Module_2_Raw_Data) / 32768 * 1.1111111 * 32768",
				},
			},
		}
		m, err := NewCanBusMapper(
			config.CanBusMapperConfig{
				MapperConfig: config.MapperConfig{Context: "testingContext"},
				DbcFile:      writeCanbusTestDBC(),
			},
			bendingMoment,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())

		out, err := m.DoMap(rawDataFrame(600, 100, 700, time.Now()))
		Expect(err).ToNot(HaveOccurred())
		values := out.ToSingleValueMapped()
		Expect(values).To(HaveLen(1))
		Expect(values[0].Path).To(Equal("propulsion.mainEngine.drive.bendingMoment"))
		Expect(values[0].Value).To(Equal(500.0 * 1.1111111))
	})
})
