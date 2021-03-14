package nmea

import (
	"errors"
	"fmt"

	goAIS "github.com/BertoldVdb/go-ais"
	"github.com/BertoldVdb/go-ais/aisnmea"

	goNMEA "github.com/adrianmo/go-nmea"
)

// Sentence interface for all NMEA sentence
type Sentence interface {
	goNMEA.Sentence
}

type (
	// DBS for corresponding NMEA sentences
	DBS goNMEA.DBS
	// DBT for corresponding NMEA sentences
	DBT goNMEA.DBT
	// DPT for corresponding NMEA sentences
	DPT goNMEA.DPT
	// GGA for corresponding NMEA sentences
	GGA goNMEA.GGA
	// GLL for corresponding NMEA sentences
	GLL goNMEA.GLL
	// GNS for corresponding NMEA sentences
	GNS goNMEA.GNS
	// GSA for corresponding NMEA sentences
	GSA goNMEA.GSA
	// GSV for corresponding NMEA sentences
	GSV goNMEA.GSV
	// HDT for corresponding NMEA sentences
	HDT goNMEA.HDT
	// MTK for corresponding NMEA sentences
	MTK goNMEA.MTK
	// PGRME for corresponding NMEA sentences
	PGRME goNMEA.PGRME
	// RMC for corresponding NMEA sentences
	RMC goNMEA.RMC
	// RTE for corresponding NMEA sentences
	RTE goNMEA.RTE
	// THS for corresponding NMEA sentences
	THS goNMEA.THS
	// VDMVDO for corresponding NMEA sentences
	VDMVDO struct {
		goNMEA.BaseSentence
		goAIS.Packet
	}
	// VHW for corresponding NMEA sentences
	VHW goNMEA.VHW
	// VTG for corresponding NMEA sentences
	VTG goNMEA.VTG
	// WPL for corresponding NMEA sentences
	WPL goNMEA.WPL
	// ZDA for corresponding NMEA sentences
	ZDA goNMEA.ZDA
)

var nmeaCodec *aisnmea.NMEACodec
var aisCodec *goAIS.Codec

func init() {
	aisCodec = goAIS.CodecNew(false, false)
	aisCodec.DropSpace = true
	nmeaCodec = aisnmea.NMEACodecNew(aisCodec)
}

// Parse is a wrapper around the original Parse function, it returns types defined in this package that implement the interfaces in this package
func Parse(raw string) (Sentence, error) {
	sentence, err := goNMEA.Parse(raw)
	if err != nil {
		return nil, err
	}
	switch sentence.DataType() {
	case goNMEA.TypeDBS:
		return DBS(sentence.(goNMEA.DBS)), nil
	case goNMEA.TypeDBT:
		return DBT(sentence.(goNMEA.DBT)), nil
	case goNMEA.TypeDPT:
		return DPT(sentence.(goNMEA.DPT)), nil
	case goNMEA.TypeGGA:
		return GGA(sentence.(goNMEA.GGA)), nil
	case goNMEA.TypeGLL:
		return GLL(sentence.(goNMEA.GLL)), nil
	case goNMEA.TypeGNS:
		return GNS(sentence.(goNMEA.GNS)), nil
	case goNMEA.TypeGSA:
		return GSA(sentence.(goNMEA.GSA)), nil
	case goNMEA.TypeGSV:
		return GSV(sentence.(goNMEA.GSV)), nil
	case goNMEA.TypeHDT:
		return HDT(sentence.(goNMEA.HDT)), nil
	case goNMEA.TypeMTK:
		return MTK(sentence.(goNMEA.MTK)), nil
	case goNMEA.TypePGRME:
		return PGRME(sentence.(goNMEA.PGRME)), nil
	case goNMEA.TypeRMC:
		return RMC(sentence.(goNMEA.RMC)), nil
	case goNMEA.TypeRTE:
		return RTE(sentence.(goNMEA.RTE)), nil
	case goNMEA.TypeTHS:
		return THS(sentence.(goNMEA.THS)), nil
	case goNMEA.TypeVDM:
		return ParseVDMVDO(sentence), nil
	case goNMEA.TypeVDO:
		return ParseVDMVDO(sentence), nil
	case goNMEA.TypeVHW:
		return VHW(sentence.(goNMEA.VHW)), nil
	case goNMEA.TypeVTG:
		return VTG(sentence.(goNMEA.VTG)), nil
	case goNMEA.TypeWPL:
		return WPL(sentence.(goNMEA.WPL)), nil
	case goNMEA.TypeZDA:
		return ZDA(sentence.(goNMEA.ZDA)), nil
	case TypeMDA, TypeMWV, TypeVWR:
		return sentence, nil
	}

	return nil, fmt.Errorf("Don't know how to handle %s", sentence.DataType())
}

// ParseVDMVDO parses the raw sentence
func ParseVDMVDO(sentence goNMEA.Sentence) VDMVDO {
	result, err := nmeaCodec.ParseSentence(sentence.String())
	if result == nil || err != nil {
		return VDMVDO{
			goNMEA.BaseSentence{},
			goAIS.Header{},
		}
	}
	return VDMVDO{
		goNMEA.BaseSentence{
			Talker: sentence.TalkerID(),
			Type:   result.MessageType,
			Raw:    sentence.String(),
		},
		result.Packet,
	}
}

func extractNumber(binaryData []byte, offset int, length int) (uint64, error) {
	var result uint64 = 0

	for _, value := range binaryData[offset : offset+length] {
		result <<= 1
		result |= uint64(value)
	}

	return result, nil
}

func extractString(binaryData []byte, offset int, length int) (string, error) {
	if (length)%6 != 0 {
		return "", errors.New("Length must be divisible by 6")
	}
	sixBitCharacters := make([]byte, length/6)
	var position int
	for index, value := range binaryData[offset : offset+length] {
		position = index / 6
		sixBitCharacters[position] <<= 1
		sixBitCharacters[position] |= value
	}
	for index, value := range sixBitCharacters {
		if value < 32 {
			sixBitCharacters[index] = value + 64
		}
	}
	return string(sixBitCharacters), nil
}
