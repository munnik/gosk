package message

import (
	"time"

	"github.com/google/uuid"
)

// mappedMsgp, updateMsgp, sourceMsgp are Mapped/Update/Source's msgpack
// wire shape - map mode (field names still on the wire, so field
// reordering/additions stay wire-compatible, unlike msgp's faster but
// brittle tuple/positional mode). Each element of Values is one Value,
// encoded by value_msgp.go's marshalValueMsg/unmarshalValueMsg: a type
// tag written from an actual Go type switch (not a guess) followed by
// that type's own native msgp encoding, so Value.Value's polymorphism
// (see Decode in mapped_update_value_types.go) costs a single byte and a
// direct dispatch instead of Decode()'s linear trial-and-error - the
// second profiling round on node-hbr-rpa10 found that guessing, not the
// outer envelope, was what kept aggregate's CPU cost high after the
// first msgp pass.
//
//go:generate msgp -tests=false -o=mapped_msgp_gen.go -unexported
type sourceMsgp struct {
	Label        string `msg:"label"`
	Type         string `msg:"type"`
	Uuid         string `msg:"uuid"`
	TransferUuid string `msg:"transferUuid"`
}

type updateMsgp struct {
	Source    sourceMsgp `msg:"source"`
	Timestamp time.Time  `msg:"timestamp"`
	Values    [][]byte   `msg:"values"`
}

type mappedMsgp struct {
	Context string       `msg:"context"`
	Origin  string       `msg:"origin"`
	Updates []updateMsgp `msg:"updates"`
}

func sourceToMsgp(s Source) sourceMsgp {
	return sourceMsgp{Label: s.Label, Type: s.Type, Uuid: s.Uuid.String(), TransferUuid: s.TransferUuid.String()}
}

func sourceFromMsgp(w sourceMsgp) (Source, error) {
	id, err := uuid.Parse(w.Uuid)
	if err != nil {
		return Source{}, err
	}
	transferID, err := uuid.Parse(w.TransferUuid)
	if err != nil {
		return Source{}, err
	}
	return Source{Label: w.Label, Type: w.Type, Uuid: id, TransferUuid: transferID}, nil
}

// MarshalMsg implements msgp.Marshaler. b is msgp's usual append-target
// convention: pass a reused, truncated buffer (buf[:0]) to avoid
// allocating a fresh one on every call, the same way json.Marshal never
// could - see nanomsg's Publisher, the only real caller of this.
func (m Mapped) MarshalMsg(b []byte) ([]byte, error) {
	wire := mappedMsgp{Context: m.Context, Origin: m.Origin, Updates: make([]updateMsgp, len(m.Updates))}
	for i, u := range m.Updates {
		values := make([][]byte, len(u.Values))
		for j, v := range u.Values {
			blob, err := marshalValueMsg(v, nil)
			if err != nil {
				return b, err
			}
			values[j] = blob
		}
		wire.Updates[i] = updateMsgp{Source: sourceToMsgp(u.Source), Timestamp: u.Timestamp, Values: values}
	}
	return wire.MarshalMsg(b)
}

// UnmarshalMsg implements msgp.Unmarshaler. Per that interface's
// contract, it returns the unconsumed tail of bts (nanomsg's Subscriber
// always passes a single whole message and ignores this, but the
// contract requires returning it regardless).
func (m *Mapped) UnmarshalMsg(bts []byte) ([]byte, error) {
	var wire mappedMsgp
	rest, err := wire.UnmarshalMsg(bts)
	if err != nil {
		return rest, err
	}

	m.Context = wire.Context
	m.Origin = wire.Origin
	m.Updates = make([]Update, len(wire.Updates))
	for i, wu := range wire.Updates {
		source, err := sourceFromMsgp(wu.Source)
		if err != nil {
			return rest, err
		}
		values := make([]Value, len(wu.Values))
		for j, blob := range wu.Values {
			decoded, _, err := unmarshalValueMsg(blob)
			if err != nil {
				return rest, err
			}
			values[j] = decoded
		}
		m.Updates[i] = Update{Source: source, Timestamp: wu.Timestamp, Values: values}
	}
	return rest, nil
}
