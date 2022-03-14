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
			raw.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
			raw.Uuid = uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389")
			marshaled, err = json.Marshal(raw)
		})
		Context("with an empty value", func() {
			BeforeEach(func() {
				raw = NewRaw().WithCollector("CAT 3512").WithValue([]byte{}).WithType(config.ModbusType)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				Expect(marshaled).To(Equal([]byte(`{"collector":"CAT 3512","timestamp":"2022-02-09T12:03:57.431272983Z","type":"modbus","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":""}`)))
			})
		})
		Context("with a set value", func() {
			BeforeEach(func() {
				raw = NewRaw().WithCollector("GPS").WithValue([]byte("$GPGLL,3723.2475,N,12158.3416,W,161229.487,A,A*41")).WithType(config.NMEA0183Type)
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				Expect(marshaled).To(Equal([]byte(`{"collector":"GPS","timestamp":"2022-02-09T12:03:57.431272983Z","type":"nmea0183","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":"JEdQR0xMLDM3MjMuMjQ3NSxOLDEyMTU4LjM0MTYsVywxNjEyMjkuNDg3LEEsQSo0MQ=="}`)))
			})
		})
	})
	Describe("Unmarshal", func() {
		JustBeforeEach(func() {
			err = json.Unmarshal(marshaled, &raw)
		})
		Context("with an empty value", func() {
			BeforeEach(func() {
				expected = NewRaw().WithCollector("CAT 3512").WithValue([]byte{}).WithType(config.ModbusType)
				expected.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				expected.Uuid = uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389")
				marshaled = []byte(`{"collector":"CAT 3512","timestamp":"2022-02-09T12:03:57.431272983Z","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":"","type":"modbus"}`)
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
				expected = NewRaw().WithCollector("GPS").WithValue([]byte("$GPGLL,3723.2475,N,12158.3416,W,161229.487,A,A*41")).WithType(config.NMEA0183Type)
				expected.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				expected.Uuid = uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389")
				marshaled = []byte(`{"collector":"GPS","timestamp":"2022-02-09T12:03:57.431272983Z","uuid":"496aa0fb-d838-4631-a12f-dbad3cb27389","value":"JEdQR0xMLDM3MjMuMjQ3NSxOLDEyMTU4LjM0MTYsVywxNjEyMjkuNDg3LEEsQSo0MQ==","type":"nmea0183"}`)
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
			marshaled, err = json.Marshal(mapped)
		})
		Context("with no updates", func() {
			BeforeEach(func() {
				mapped = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789")
			})
			It("returns no errors", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			It("equals a correct json document", func() {
				j := `
				{
					"context": "vessels.urn:mrn:imo:mmsi:234567890",
					"origin": "vessels.urn:mrn:imo:mmsi:123456789",
  					"updates": []
				}`
				Expect(marshaled).To(MatchJSON(j))
			})
		})
		Context("with a single update with a single value", func() {
			BeforeEach(func() {
				mapped = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789")
				s := NewSource().WithLabel("CAT 3512").WithType(config.ModbusType).WithUuid(uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389"))
				u := NewUpdate().WithSource(*s)
				v := NewValue().WithPath("propulsion.0.revolutions").WithValue(16.341667)
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
					"origin": "vessels.urn:mrn:imo:mmsi:123456789",
        			"updates": [
          				{
            				"source": {
              					"label": "CAT 3512",
              					"type": "modbus",
								"uuid": "496aa0fb-d838-4631-a12f-dbad3cb27389"
            				},
            				"timestamp": "2022-02-09T12:03:57.431272983Z",
            				"values": [
								{
									"path": "propulsion.0.revolutions",
          							"value": 16.341667
								}
							]
          				}
        			]
      			}`
				Expect(marshaled).To(MatchJSON(j))
			})
		})
		Context("with a single update with a position value", func() {
			BeforeEach(func() {
				mapped = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789")
				s := NewSource().WithLabel("GPS").WithType(config.NMEA0183Type).WithUuid(uuid.MustParse("496aa0fb-d838-4631-a12f-dbad3cb27389"))
				u := NewUpdate().WithSource(*s)
				lat := 52.150099
				lon := 5.921749
				v := NewValue().WithPath("navigation.position").WithValue(Position{Latitude: &lat, Longitude: &lon})
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
					"origin": "vessels.urn:mrn:imo:mmsi:123456789",
        			"updates": [
          				{
            				"source": {
              					"label": "GPS",
              					"type": "nmea0183",
								"uuid": "496aa0fb-d838-4631-a12f-dbad3cb27389"
            				},
            				"timestamp": "2022-02-09T12:03:57.431272983Z",
            				"values": [
								{
									"path": "navigation.position",
          							"value": {
										  "latitude": 52.150099,
										  "longitude": 5.921749
									  }
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
			err = json.Unmarshal(marshaled, mapped)
		})
		Context("with no updates", func() {
			BeforeEach(func() {
				expected = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789")
				marshaled = []byte(`
				{
					"context": "vessels.urn:mrn:imo:mmsi:234567890",
					"origin": "vessels.urn:mrn:imo:mmsi:123456789",
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
				alt := 0.0
				lat := 37.81479
				lon := -122.44880152
				t := true
				m := "AIS: Antenna VSWR exceeds limit"
				s := NewSource().WithLabel("AIS").WithType(config.NMEA0183Type).WithUuid(uuid.MustParse("84679362-f963-405f-aa37-a6a8ed961417"))
				v1 := NewValue().WithPath("navigation.position").WithValue(Position{Altitude: &alt, Latitude: &lat, Longitude: &lon})
				v2 := NewValue().WithPath("navigation.state").WithValue("motoring")
				v3 := NewValue().WithPath("notifications.ais").WithValue(Alarm{State: &t, Message: &m})
				u := NewUpdate().WithSource(*s).AddValue(v1).AddValue(v2).AddValue(v3)
				u.Timestamp = time.Date(2022, time.Month(2), 9, 12, 3, 57, 431272983, time.UTC)
				expected = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:234567890").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789").AddUpdate(u)
				marshaled = []byte(`
				{
  					"context": "vessels.urn:mrn:imo:mmsi:234567890",
					"origin": "vessels.urn:mrn:imo:mmsi:123456789",
  					"updates": [
    					{
      						"source": {
        						"label": "AIS",
        						"type": "nmea0183",
								"uuid": "84679362-f963-405f-aa37-a6a8ed961417"
      						},
      						"timestamp": "2022-02-09T12:03:57.431272983Z",
      						"values": [
        						{
          							"path": "navigation.position",
          							"value": {
            							"altitude": 0.0,
            							"latitude": 37.81479,
            							"longitude": -122.44880152
          							}
        						},
        						{
          							"path": "navigation.state",
          							"value": "motoring"
        						},
        						{
          							"path": "notifications.ais",
          							"value": {
										  "state": true,
										  "message": "AIS: Antenna VSWR exceeds limit"
									}
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
		Context("real data", func() {
			BeforeEach(func() {
				lat := 51.89874666666666
				lon := 4.487056666666667
				s := NewSource().WithLabel("AIS").WithType(config.NMEA0183Type).WithUuid(uuid.UUID{104, 113, 49, 233, 41, 50, 66, 74, 170, 51, 99, 11, 36, 116, 203, 160})
				v1 := NewValue().WithPath("mmsi").WithValue("244700143")
				v2 := NewValue().WithPath("navigation.state").WithValue("motoring")
				v3 := NewValue().WithPath("navigation.position").WithValue(Position{Latitude: &lat, Longitude: &lon})
				v4 := NewValue().WithPath("navigation.speedOverGround").WithValue(0.0)
				u := NewUpdate().WithSource(*s).AddValue(v1).AddValue(v2).AddValue(v3).AddValue(v4)
				u.Timestamp = time.Date(2022, time.Month(2), 21, 23, 9, 33, 756165025, time.UTC)
				expected = NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:244700143").WithOrigin("vessels.urn:mrn:imo:mmsi:244620991").AddUpdate(u)
				marshaled = []byte(`{"context":"vessels.urn:mrn:imo:mmsi:244700143","origin":"vessels.urn:mrn:imo:mmsi:244620991","updates":[{"source":{"label":"AIS","uuid":"687131e9-2932-424a-aa33-630b2474cba0","type":"nmea0183"},"timestamp":"2022-02-21T23:09:33.756165025Z","values":[{"path":"mmsi","value":"244700143"},{"path":"navigation.state","value":"motoring"},{"path":"navigation.position","value":{"latitude":51.89874666666666,"longitude":4.487056666666667}},{"path":"navigation.speedOverGround","value":0}]}]}`)
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
