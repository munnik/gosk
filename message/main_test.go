package message_test

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Raw", func() {
	var (
		raw       *Raw
		expected  *Raw
		marshaled []byte
		err       error
	)
	Describe("Marshal", func() {
		JustBeforeEach(func() {
			// 2022-02-09T12:03:57.431272983Z
			raw.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
			raw.Uuid = uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389")
			marshaled, err = json.Marshal(raw)
		})
		Context("with an empty value", func() {
			BeforeEach(func() {
				raw = NewRaw().WithCollector("CAT 3512").WithValue([]byte{})
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				Expect(marshaled).To(Equal([]byte(`{"collector":"CAT 3512","timestamp":"2022-02-09T12:03:57.431272983Z","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":""}`)))
			})
		})
		Context("with a set value", func() {
			BeforeEach(func() {
				raw = NewRaw().WithCollector("GPS").WithValue([]byte("$GPGLL,3723.2475,N,12158.3416,W,161229.487,A,A*41"))
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				Expect(marshaled).To(Equal([]byte(`{"collector":"GPS","timestamp":"2022-02-09T12:03:57.431272983Z","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":"JEdQR0xMLDM3MjMuMjQ3NSxOLDEyMTU4LjM0MTYsVywxNjEyMjkuNDg3LEEsQSo0MQ=="}`)))
			})
		})
	})
	Describe("Unmarshal", func() {
		JustBeforeEach(func() {
			// 2022-02-09T12:03:57.431272983Z
			err = json.Unmarshal(marshaled, &raw)
		})
		Context("with an empty value", func() {
			BeforeEach(func() {
				expected = NewRaw().WithCollector("CAT 3512").WithValue([]byte{})
				expected.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				expected.Uuid = uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389")
				marshaled = []byte(`{"collector":"CAT 3512","timestamp":"2022-02-09T12:03:57.431272983Z","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":""}`)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a valid Raw struct", func() {
				Expect(raw).To(Equal(expected))
			})
		})
		Context("with a set value", func() {
			BeforeEach(func() {
				expected = NewRaw().WithCollector("GPS").WithValue([]byte("$GPGLL,3723.2475,N,12158.3416,W,161229.487,A,A*41"))
				expected.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				expected.Uuid = uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389")
				marshaled = []byte(`{"collector":"GPS","timestamp":"2022-02-09T12:03:57.431272983Z","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":"JEdQR0xMLDM3MjMuMjQ3NSxOLDEyMTU4LjM0MTYsVywxNjEyMjkuNDg3LEEsQSo0MQ=="}`)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a valid Raw struct", func() {
				Expect(raw).To(Equal(expected))
			})
		})
	})
})
var _ = Describe("Mapped", func() {
	var (
		mapped    *Mapped
		expected  *Mapped
		marshaled []byte
		err       error
	)
	Describe("Marshal", func() {
		JustBeforeEach(func() {
			mapped.Context = "vessels.urn:mrn:imo:mmsi:234567890"
			marshaled, err = json.Marshal(mapped)
		})
		Context("with no updates", func() {
			BeforeEach(func() {
				mapped = NewMapped()
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				j := `
				{
					"context": "vessels.urn:mrn:imo:mmsi:234567890",
  					"updates": []
				}`
				Expect(marshaled).To(MatchJSON(j))
			})
		})
		Context("with a single update with a single value", func() {
			BeforeEach(func() {
				mapped = NewMapped()
				s := NewSource().WithLabel("CAT 3512").WithType(config.ModbusType)
				u := NewUpdate().WithSource(s)
				v := NewValue().WithPath("propulsion.0.revolutions").WithValue(16.341667).WithUuid(uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389"))
				u.AddValue(v)
				// 2022-02-09T12:03:57.431272983Z
				u.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				mapped.AddUpdate(u)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				j := `
				{
        			"context": "vessels.urn:mrn:imo:mmsi:234567890",
        			"updates": [
          				{
            				"source": {
              					"label": "CAT 3512",
              					"type": "modbus",
            				},
            				"timestamp": "2022-02-09T12:03:57.431272983Z",
            				"values": [
								{
									"path": "propulsion.0.revolutions",
          							"value": 16.341667,
									"uuid": "496aa0fb-d838-4631-a12f-dbad3cb27389"
								}
							]
          				}
        			]
      			}`
				Expect(marshaled).To(MatchJSON(j))
			})
		})
	})
	Describe("Unmarshal", func() {
		JustBeforeEach(func() {
			err = json.Unmarshal(marshaled, &mapped)
		})
		Context("with no updates", func() {
			BeforeEach(func() {
				expected = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890")
				marshaled = []byte(`
				{
					"context": "vessels.urn:mrn:imo:mmsi:234567890",
  					"updates": []
				}`)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a valid Mapped struct", func() {
				Expect(mapped).To(Equal(expected))
			})
		})
		Context("with multiple updates", func() {
			BeforeEach(func() {
				s1 := NewSource().WithLabel("CAT 3512").WithType(config.ModbusType)
				s2 := NewSource().WithLabel("AIS").WithType(config.NMEA0183Type)
				v1 := NewValue().WithPath("propulsion.0.revolutions").WithValue(16.341667).WithUuid(uuid.MustParse("84679362-f963-405f-aa37-a6a8ed961417"))
				v2 := NewValue().WithPath("propulsion.0.boostPressure").WithValue(45500.0).WithUuid(uuid.MustParse("84679362-f963-405f-aa37-a6a8ed961417"))
				v3 := NewValue().WithPath("navigation.position").WithValue(Position{Altitude: 0.0, Latitude: 37.81479, Longitude: -122.44880152}).WithUuid(uuid.MustParse("84679362-f963-405f-aa37-a6a8ed961417"))
				v4 := NewValue().WithPath("navigation.state").WithValue("motoring").WithUuid(uuid.MustParse("84679362-f963-405f-aa37-a6a8ed961417"))
				u1 := NewUpdate().WithSource(s1).AddValue(v1).AddValue(v2)
				u1.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				u2 := NewUpdate().WithSource(s2).AddValue(v3).AddValue(v4)
				u2.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				expected = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890").AddUpdate(u1).AddUpdate(u2)
				marshaled = []byte(`
				{
  					"context": "vessels.urn:mrn:imo:mmsi:234567890",
  					"updates": [
    					{
      						"source": {
        						"label": "CAT 3512",
        						"type": "modbus",
      						},
      						"timestamp": "2022-02-09T12:03:57.431272983Z",
      						"values": [
        						{
          							"path": "propulsion.0.revolutions",
          							"value": 16.341667, 
									"uuid": "84679362-f963-405f-aa37-a6a8ed961417"
        						},
        						{
          							"path": "propulsion.0.boostPressure",
          							"value": 45500,
									"uuid": "84679362-f963-405f-aa37-a6a8ed961417"
        						}
      						]
    					}, 
    					{
      						"source": {
        						"label": "AIS",
        						"type": "nmea0183",
      						},
      						"timestamp": "2022-02-09T12:03:57.431272983Z",
      						"values": [
        						{
          							"path": "navigation.position",
          							"value": {
            							"altitude": 0.0,
            							"latitude": 37.81479,
            							"longitude": -122.44880152
          							},
									"uuid": "84679362-f963-405f-aa37-a6a8ed961417"
        						},
        						{
          							"path": "navigation.state",
          							"value": "motoring",
									"uuid": "84679362-f963-405f-aa37-a6a8ed961417"
        						}
      						]
    					}
  					]
				}`)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a valid Mapped struct", func() {
				Expect(mapped).To(Equal(expected))
			})
		})
	})
})
