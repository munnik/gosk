package nmea_test

import (
	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("Main", func() {
	Describe("Parsing NMEA0183 sentence", func() {
		var (
			input  string
			parsed goNMEA.Sentence
			err    error
		)

		JustBeforeEach(func() {
			parsed, err = nmea.Parse(input)
		})
		Context("When a sentence has a wrong checksum", func() {
			BeforeEach(func() {
				input = `$GPVTG,234.6,T,232.8,M,6.5,N,12.0,K,D*A2`
			})
			Specify("an error is returned", func() {
				Expect(err).To(HaveOccurred())
			})
			Specify("the parsed data is nil", func() {
				Expect(parsed).To(BeNil())
			})
		})

		Context("When a sentence has no checksum", func() {
			BeforeEach(func() {
				input = `$GPVTG,234.6,T,232.8,M,6.5,N,12.0,K,D`
			})
			Specify("an error is returned", func() {
				Expect(err).To(HaveOccurred())
			})
			Specify("the parsed data is nil", func() {
				Expect(parsed).To(BeNil())
			})
		})

		Context("When the sentence is unknown", func() {
			BeforeEach(func() {
				input = `$PGRMZ,2.7,f,3*00`
			})
			Specify("an error is returned", func() {
				Expect(err).To(HaveOccurred())
			})
			Specify("the parsed data is nil", func() {
				Expect(parsed).To(BeNil())
			})
		})

		Context("When a sentence has leading whitespaces", func() {
			BeforeEach(func() {
				input = `    $GPVTG,234.6,T,232.8,M,6.5,N,12.0,K,D*1E`
			})
			Specify("no error is returned", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			Specify("the parsed data is not nil", func() {
				Expect(parsed).NotTo(BeNil())
			})
		})

		Context("When a sentence has trailing whitespaces", func() {
			BeforeEach(func() {
				input = `$GPVTG,234.6,T,232.8,M,6.5,N,12.0,K,D*1E   `
			})
			Specify("no error is returned", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			Specify("the parsed data is not nil", func() {
				Expect(parsed).NotTo(BeNil())
			})
		})

		Context("When as sentence has a tag block", func() {
			BeforeEach(func() {
				input = `\s:r003669961,c:1153612428*77\$HEHDT,119.9,T*2F`
			})
			Specify("no error is returned", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			Specify("the parsed data is not nil", func() {
				Expect(parsed).NotTo(BeNil())
			})
		})

		Context("When the message is missing data", func() {
			BeforeEach(func() {
				input = `$GPVTG,,T,,M,,N,,K,D*26`
			})
			Specify("an error is returned", func() {
				cast, _ := parsed.(nmea.TrueCourseOverGround)
				_, err := cast.GetTrueCourseOverGround()
				Expect(err).To(HaveOccurred())
			})
		})
	})
	Describe("Value", func() {
		Describe("Float64", func() {
			Context("No value", func() {
				Specify("an error should be returned", func() {
					nilF := nmea.NewFloat64()
					f, err := nilF.GetValue()
					Expect(f).To(BeZero())
					Expect(err).To(HaveOccurred())
				})
			})
			Context("With a value", func() {
				Specify("a value should be returned", func() {
					nilF := nmea.NewFloat64WithValue(4.2)
					f, err := nilF.GetValue()
					Expect(f).To(Equal(float64(4.2)))
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
		Describe("Int64", func() {
			Context("No value", func() {
				Specify("an error should be returned", func() {
					nilInt := nmea.NewInt64()
					i, err := nilInt.GetValue()
					Expect(i).To(BeZero())
					Expect(err).To(HaveOccurred())
				})
			})
			Context("With a value", func() {
				Specify("a value should be returned", func() {
					nilInt := nmea.NewInt64WithValue(-75)
					i, err := nilInt.GetValue()
					Expect(i).To(Equal(int64(-75)))
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	})
})
