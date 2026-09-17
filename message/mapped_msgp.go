package message

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// mappedMsgp, updateMsgp, sourceMsgp are Mapped/Update/Source's msgpack
// wire shape - map mode (field names still on the wire, so field
// reordering/additions stay wire-compatible, unlike msgp's faster but
// brittle tuple/positional mode). Values stores each Value struct as a
// whole, JSON-encoded blob rather than native msgp fields: Value.Value is
// interface{}, holding whichever of Position/Notification/Spectrum/a
// plain scalar/etc a given path's data actually is (see Decode in
// mapped_update_value_types.go), and msgp's codegen needs a static type
// per field just as easyjson's does - so this keeps that dynamic,
// self-describing behavior working completely unchanged (still calls
// Value's own existing UnmarshalJSON/Decode dispatch), at the cost of not
// speeding up that one field. Everything around it - Source, Timestamp,
// Path structure, the Values slice itself - gets the full msgp speedup,
// which is where profiling on node-hbr-rpa10 found the actual cost
// living, not inside Value's own dispatch.
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
			blob, err := json.Marshal(v)
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
			if err := values[j].UnmarshalJSON(blob); err != nil {
				return rest, err
			}
		}
		m.Updates[i] = Update{Source: source, Timestamp: wu.Timestamp, Values: values}
	}
	return rest, nil
}
