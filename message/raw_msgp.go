package message

import (
	"time"

	"github.com/munnik/uuid/v5"
)

// rawMsgp is Raw's msgpack wire shape - map mode, see mapped_msgp.go's
// doc comment for why. Unlike Mapped's Value, Raw's own Value is already
// a plain []byte with no polymorphism to work around, so this needs no
// JSON-blob indirection at all.
//
//go:generate msgp -tests=false -o=raw_msgp_gen.go -unexported
type rawMsgp struct {
	Connector string    `msg:"connector"`
	Timestamp time.Time `msg:"timestamp"`
	Type      string    `msg:"type"`
	Uuid      string    `msg:"uuid"`
	Value     []byte    `msg:"value"`
}

// MarshalMsg implements msgp.Marshaler - see Mapped.MarshalMsg's doc
// comment on b's reused-buffer convention.
func (r Raw) MarshalMsg(b []byte) ([]byte, error) {
	value := r.Value
	if value == nil {
		value = []byte{}
	}
	wire := rawMsgp{
		Connector: r.Connector,
		Timestamp: r.Timestamp.UTC(),
		Type:      r.Type,
		Uuid:      r.Uuid.String(),
		Value:     value,
	}
	return wire.MarshalMsg(b)
}

// UnmarshalMsg implements msgp.Unmarshaler.
func (r *Raw) UnmarshalMsg(bts []byte) ([]byte, error) {
	var wire rawMsgp
	rest, err := wire.UnmarshalMsg(bts)
	if err != nil {
		return rest, err
	}

	id, err := uuid.FromString(wire.Uuid)
	if err != nil {
		return rest, err
	}

	r.Connector = wire.Connector
	r.Timestamp = wire.Timestamp
	r.Type = wire.Type
	r.Uuid = id
	r.Value = wire.Value
	return rest, nil
}
