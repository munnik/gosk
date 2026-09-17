package message

// SingleValueMapped's msgpack encoding: see singleValueMappedMsgp in
// mapped_msgp.go for the wire shape (map mode, same reasoning as
// Mapped/Update/Source there) and mapped_msgp_gen.go for its generated
// codec. Value reuses value_msgp.go's tagged Path+Value encoding (the
// same one Mapped's Updates use) rather than duplicating it, so
// SingleValueMapped gets the same no-guessing decode that codec exists
// for in the first place.

// MarshalMsg implements msgp.Marshaler - see Mapped.MarshalMsg's doc
// comment on b's reused-buffer convention.
func (s SingleValueMapped) MarshalMsg(b []byte) ([]byte, error) {
	valueBlob, err := marshalValueMsg(Value{Path: s.Path, Value: s.Value}, nil)
	if err != nil {
		return b, err
	}
	wire := singleValueMappedMsgp{
		Context:   s.Context,
		Origin:    s.Origin,
		Source:    sourceToMsgp(s.Source),
		Timestamp: s.Timestamp,
		Value:     valueBlob,
	}
	return wire.MarshalMsg(b)
}

// UnmarshalMsg implements msgp.Unmarshaler.
func (s *SingleValueMapped) UnmarshalMsg(bts []byte) ([]byte, error) {
	var wire singleValueMappedMsgp
	rest, err := wire.UnmarshalMsg(bts)
	if err != nil {
		return rest, err
	}

	source, err := sourceFromMsgp(wire.Source)
	if err != nil {
		return rest, err
	}
	v, _, err := unmarshalValueMsg(wire.Value)
	if err != nil {
		return rest, err
	}

	s.Context = wire.Context
	s.Origin = wire.Origin
	s.Source = source
	s.Timestamp = wire.Timestamp
	s.Path = v.Path
	s.Value = v.Value
	return rest, nil
}
