package message

import (
	"encoding/json"
	"fmt"

	"github.com/tinylib/msgp/msgp"
)

// valueTag identifies which concrete type Value.Value holds, written by
// marshalValueMsg's type switch (which knows the real type, no guessing)
// and read back by unmarshalValueMsg to dispatch straight to the matching
// decoder - this is what lets gosk's internal nanomsg transport skip
// Decode()'s linear trial-and-error entirely. It has no relation to any
// SignalK/JSON wire format: Value's JSON MarshalJSON/UnmarshalJSON is
// untouched, so external consumers (writer/signalk*.go, Postgres storage)
// still see plain, untagged SignalK JSON.
type valueTag byte

const (
	valueTagNil valueTag = iota
	valueTagFloat64
	valueTagInt64
	valueTagString
	valueTagBool
	valueTagPosition
	valueTagVesselInfo
	valueTagVesselType
	valueTagLength
	valueTagNotification
	valueTagNotificationSlice
	valueTagDraft
	valueTagSpectrum
	valueTagVector3D
	// valueTagJSONFallback covers anything not listed above: a type added
	// to Decode()'s candidates but not (yet) to the switches below, or a
	// genuinely dynamic value (e.g. the map-shaped expression literal
	// mapper/modbus_test.yaml exercises) that Decode() itself would have
	// to guess at anyway. Round-trips via the same JSON+Decode() path
	// Value used exclusively before this file existed - never a hard
	// failure, just not the fast path.
	valueTagJSONFallback
)

// marshalValueMsg appends v's msgpack encoding to b (the usual
// reused-buffer convention) and returns the result.
func marshalValueMsg(v Value, b []byte) ([]byte, error) {
	b = msgp.AppendString(b, v.Path)

	switch val := v.Value.(type) {
	case nil:
		return msgp.AppendByte(b, byte(valueTagNil)), nil
	case float64:
		b = msgp.AppendByte(b, byte(valueTagFloat64))
		return msgp.AppendFloat64(b, val), nil
	case int64:
		b = msgp.AppendByte(b, byte(valueTagInt64))
		return msgp.AppendInt64(b, val), nil
	case string:
		b = msgp.AppendByte(b, byte(valueTagString))
		return msgp.AppendString(b, val), nil
	case bool:
		b = msgp.AppendByte(b, byte(valueTagBool))
		return msgp.AppendBool(b, val), nil
	case Position:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagPosition)))
	case VesselInfo:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagVesselInfo)))
	case VesselType:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagVesselType)))
	case Length:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagLength)))
	case Notification:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagNotification)))
	case []Notification:
		b = msgp.AppendByte(b, byte(valueTagNotificationSlice))
		b = msgp.AppendArrayHeader(b, uint32(len(val)))
		var err error
		for _, n := range val {
			if b, err = n.MarshalMsg(b); err != nil {
				return b, err
			}
		}
		return b, nil
	case Draft:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagDraft)))
	case Spectrum:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagSpectrum)))
	case Vector3D:
		return val.MarshalMsg(msgp.AppendByte(b, byte(valueTagVector3D)))
	default:
		blob, err := json.Marshal(v.Value)
		if err != nil {
			return b, err
		}
		b = msgp.AppendByte(b, byte(valueTagJSONFallback))
		return msgp.AppendBytes(b, blob), nil
	}
}

// unmarshalValueMsg reads a Value from the front of bts (as written by
// marshalValueMsg) and returns it along with the unconsumed tail.
func unmarshalValueMsg(bts []byte) (Value, []byte, error) {
	path, bts, err := msgp.ReadStringBytes(bts)
	if err != nil {
		return Value{}, bts, err
	}
	tagByte, bts, err := msgp.ReadByteBytes(bts)
	if err != nil {
		return Value{}, bts, err
	}

	v := Value{Path: path}
	switch valueTag(tagByte) {
	case valueTagNil:
		v.Value = nil
	case valueTagFloat64:
		var f float64
		f, bts, err = msgp.ReadFloat64Bytes(bts)
		v.Value = f
	case valueTagInt64:
		var i int64
		i, bts, err = msgp.ReadInt64Bytes(bts)
		v.Value = i
	case valueTagString:
		var s string
		s, bts, err = msgp.ReadStringBytes(bts)
		v.Value = s
	case valueTagBool:
		var bo bool
		bo, bts, err = msgp.ReadBoolBytes(bts)
		v.Value = bo
	case valueTagPosition:
		var p Position
		bts, err = p.UnmarshalMsg(bts)
		v.Value = p
	case valueTagVesselInfo:
		var vi VesselInfo
		bts, err = vi.UnmarshalMsg(bts)
		v.Value = vi
	case valueTagVesselType:
		var vt VesselType
		bts, err = vt.UnmarshalMsg(bts)
		v.Value = vt
	case valueTagLength:
		var l Length
		bts, err = l.UnmarshalMsg(bts)
		v.Value = l
	case valueTagNotification:
		var n Notification
		bts, err = n.UnmarshalMsg(bts)
		v.Value = n
	case valueTagNotificationSlice:
		var sz uint32
		sz, bts, err = msgp.ReadArrayHeaderBytes(bts)
		if err != nil {
			return Value{}, bts, err
		}
		ns := make([]Notification, sz)
		for i := range ns {
			if bts, err = ns[i].UnmarshalMsg(bts); err != nil {
				return Value{}, bts, err
			}
		}
		v.Value = ns
	case valueTagDraft:
		var d Draft
		bts, err = d.UnmarshalMsg(bts)
		v.Value = d
	case valueTagSpectrum:
		var s Spectrum
		bts, err = s.UnmarshalMsg(bts)
		v.Value = s
	case valueTagVector3D:
		var vec Vector3D
		bts, err = vec.UnmarshalMsg(bts)
		v.Value = vec
	case valueTagJSONFallback:
		var blob []byte
		blob, bts, err = msgp.ReadBytesBytes(bts, nil)
		if err != nil {
			return Value{}, bts, err
		}
		var raw interface{}
		if err = json.Unmarshal(blob, &raw); err != nil {
			return Value{}, bts, err
		}
		v.Value, err = Decode(raw)
	default:
		return Value{}, bts, fmt.Errorf("unknown value tag %d", tagByte)
	}

	return v, bts, err
}
