// Code generated by easyjson for marshaling/unmarshaling. DO NOT EDIT.

package signalk

import (
	json "encoding/json"
	easyjson "github.com/mailru/easyjson"
	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
)

// suppress unused package warning
var (
	_ *json.RawMessage
	_ *jlexer.Lexer
	_ *jwriter.Writer
	_ easyjson.Marshaler
)

func easyjson89aae3efDecodeGithubComMunnikGoskSignalk(in *jlexer.Lexer, out *Value) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "context":
			out.Context = string(in.String())
		case "path":
			if in.IsNull() {
				in.Skip()
				out.Path = nil
			} else {
				in.Delim('[')
				if out.Path == nil {
					if !in.IsDelim(']') {
						out.Path = make([]string, 0, 4)
					} else {
						out.Path = []string{}
					}
				} else {
					out.Path = (out.Path)[:0]
				}
				for !in.IsDelim(']') {
					var v1 string
					v1 = string(in.String())
					out.Path = append(out.Path, v1)
					in.WantComma()
				}
				in.Delim(']')
			}
		case "value":
			if m, ok := out.Value.(easyjson.Unmarshaler); ok {
				m.UnmarshalEasyJSON(in)
			} else if m, ok := out.Value.(json.Unmarshaler); ok {
				_ = m.UnmarshalJSON(in.Raw())
			} else {
				out.Value = in.Interface()
			}
		case "source":
			(out.Source).UnmarshalEasyJSON(in)
		case "timestamp":
			out.Timestamp = string(in.String())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk(out *jwriter.Writer, in Value) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Context != "" {
		const prefix string = ",\"context\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Context))
	}
	if len(in.Path) != 0 {
		const prefix string = ",\"path\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		{
			out.RawByte('[')
			for v2, v3 := range in.Path {
				if v2 > 0 {
					out.RawByte(',')
				}
				out.String(string(v3))
			}
			out.RawByte(']')
		}
	}
	{
		const prefix string = ",\"value\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		if m, ok := in.Value.(easyjson.Marshaler); ok {
			m.MarshalEasyJSON(out)
		} else if m, ok := in.Value.(json.Marshaler); ok {
			out.Raw(m.MarshalJSON())
		} else {
			out.Raw(json.Marshal(in.Value))
		}
	}
	{
		const prefix string = ",\"source\":"
		out.RawString(prefix)
		(in.Source).MarshalEasyJSON(out)
	}
	{
		const prefix string = ",\"timestamp\":"
		out.RawString(prefix)
		out.String(string(in.Timestamp))
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Value) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Value) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Value) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Value) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk(l, v)
}
func easyjson89aae3efDecodeGithubComMunnikGoskSignalk1(in *jlexer.Lexer, out *Update) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "source":
			(out.Source).UnmarshalEasyJSON(in)
		case "timestamp":
			out.Timestamp = string(in.String())
		case "values":
			if in.IsNull() {
				in.Skip()
				out.Values = nil
			} else {
				in.Delim('[')
				if out.Values == nil {
					if !in.IsDelim(']') {
						out.Values = make([]Value, 0, 0)
					} else {
						out.Values = []Value{}
					}
				} else {
					out.Values = (out.Values)[:0]
				}
				for !in.IsDelim(']') {
					var v4 Value
					(v4).UnmarshalEasyJSON(in)
					out.Values = append(out.Values, v4)
					in.WantComma()
				}
				in.Delim(']')
			}
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk1(out *jwriter.Writer, in Update) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"source\":"
		out.RawString(prefix[1:])
		(in.Source).MarshalEasyJSON(out)
	}
	{
		const prefix string = ",\"timestamp\":"
		out.RawString(prefix)
		out.String(string(in.Timestamp))
	}
	{
		const prefix string = ",\"values\":"
		out.RawString(prefix)
		if in.Values == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v5, v6 := range in.Values {
				if v5 > 0 {
					out.RawByte(',')
				}
				(v6).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Update) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk1(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Update) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk1(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Update) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk1(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Update) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk1(l, v)
}
func easyjson89aae3efDecodeGithubComMunnikGoskSignalk2(in *jlexer.Lexer, out *Source) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "label":
			out.Label = string(in.String())
		case "type":
			out.Type = string(in.String())
		case "talker":
			out.Talker = string(in.String())
		case "sentence":
			out.Sentence = string(in.String())
		case "aisType":
			out.AisType = uint8(in.Uint8())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk2(out *jwriter.Writer, in Source) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"label\":"
		out.RawString(prefix[1:])
		out.String(string(in.Label))
	}
	{
		const prefix string = ",\"type\":"
		out.RawString(prefix)
		out.String(string(in.Type))
	}
	if in.Talker != "" {
		const prefix string = ",\"talker\":"
		out.RawString(prefix)
		out.String(string(in.Talker))
	}
	if in.Sentence != "" {
		const prefix string = ",\"sentence\":"
		out.RawString(prefix)
		out.String(string(in.Sentence))
	}
	if in.AisType != 0 {
		const prefix string = ",\"aisType\":"
		out.RawString(prefix)
		out.Uint8(uint8(in.AisType))
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Source) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk2(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Source) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk2(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Source) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk2(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Source) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk2(l, v)
}
func easyjson89aae3efDecodeGithubComMunnikGoskSignalk3(in *jlexer.Lexer, out *Position) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "longitude":
			out.Longitude = float64(in.Float64())
		case "latitude":
			out.Latitude = float64(in.Float64())
		case "altitude":
			out.Altitude = float64(in.Float64())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk3(out *jwriter.Writer, in Position) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"longitude\":"
		out.RawString(prefix[1:])
		out.Float64(float64(in.Longitude))
	}
	{
		const prefix string = ",\"latitude\":"
		out.RawString(prefix)
		out.Float64(float64(in.Latitude))
	}
	if in.Altitude != 0 {
		const prefix string = ",\"altitude\":"
		out.RawString(prefix)
		out.Float64(float64(in.Altitude))
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Position) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk3(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Position) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk3(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Position) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk3(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Position) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk3(l, v)
}
func easyjson89aae3efDecodeGithubComMunnikGoskSignalk4(in *jlexer.Lexer, out *Length) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "overall":
			out.Overall = float64(in.Float64())
		case "hull":
			out.Hull = float64(in.Float64())
		case "waterline":
			out.Waterline = float64(in.Float64())
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk4(out *jwriter.Writer, in Length) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Overall != 0 {
		const prefix string = ",\"overall\":"
		first = false
		out.RawString(prefix[1:])
		out.Float64(float64(in.Overall))
	}
	if in.Hull != 0 {
		const prefix string = ",\"hull\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Float64(float64(in.Hull))
	}
	if in.Waterline != 0 {
		const prefix string = ",\"waterline\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		out.Float64(float64(in.Waterline))
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Length) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk4(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Length) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk4(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Length) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk4(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Length) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk4(l, v)
}
func easyjson89aae3efDecodeGithubComMunnikGoskSignalk5(in *jlexer.Lexer, out *Full) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "version":
			out.Version = string(in.String())
		case "self":
			out.Self = string(in.String())
		case "vessels":
			if in.IsNull() {
				in.Skip()
			} else {
				in.Delim('{')
				out.Vessels = make(map[string]*VesselValues)
				for !in.IsDelim('}') {
					key := string(in.String())
					in.WantColon()
					var v7 *VesselValues
					if in.IsNull() {
						in.Skip()
						v7 = nil
					} else {
						if v7 == nil {
							v7 = new(VesselValues)
						}
						if in.IsNull() {
							in.Skip()
						} else {
							in.Delim('{')
							*v7 = make(VesselValues)
							for !in.IsDelim('}') {
								key := string(in.String())
								in.WantColon()
								var v8 interface{}
								if m, ok := v8.(easyjson.Unmarshaler); ok {
									m.UnmarshalEasyJSON(in)
								} else if m, ok := v8.(json.Unmarshaler); ok {
									_ = m.UnmarshalJSON(in.Raw())
								} else {
									v8 = in.Interface()
								}
								(*v7)[key] = v8
								in.WantComma()
							}
							in.Delim('}')
						}
					}
					(out.Vessels)[key] = v7
					in.WantComma()
				}
				in.Delim('}')
			}
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk5(out *jwriter.Writer, in Full) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"version\":"
		out.RawString(prefix[1:])
		out.String(string(in.Version))
	}
	{
		const prefix string = ",\"self\":"
		out.RawString(prefix)
		out.String(string(in.Self))
	}
	{
		const prefix string = ",\"vessels\":"
		out.RawString(prefix)
		if in.Vessels == nil && (out.Flags&jwriter.NilMapAsEmpty) == 0 {
			out.RawString(`null`)
		} else {
			out.RawByte('{')
			v9First := true
			for v9Name, v9Value := range in.Vessels {
				if v9First {
					v9First = false
				} else {
					out.RawByte(',')
				}
				out.String(string(v9Name))
				out.RawByte(':')
				if v9Value == nil {
					out.RawString("null")
				} else {
					if *v9Value == nil && (out.Flags&jwriter.NilMapAsEmpty) == 0 {
						out.RawString(`null`)
					} else {
						out.RawByte('{')
						v10First := true
						for v10Name, v10Value := range *v9Value {
							if v10First {
								v10First = false
							} else {
								out.RawByte(',')
							}
							out.String(string(v10Name))
							out.RawByte(':')
							if m, ok := v10Value.(easyjson.Marshaler); ok {
								m.MarshalEasyJSON(out)
							} else if m, ok := v10Value.(json.Marshaler); ok {
								out.Raw(m.MarshalJSON())
							} else {
								out.Raw(json.Marshal(v10Value))
							}
						}
						out.RawByte('}')
					}
				}
			}
			out.RawByte('}')
		}
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Full) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk5(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Full) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk5(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Full) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk5(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Full) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk5(l, v)
}
func easyjson89aae3efDecodeGithubComMunnikGoskSignalk6(in *jlexer.Lexer, out *Delta) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(false)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "context":
			out.Context = string(in.String())
		case "updates":
			if in.IsNull() {
				in.Skip()
				out.Updates = nil
			} else {
				in.Delim('[')
				if out.Updates == nil {
					if !in.IsDelim(']') {
						out.Updates = make([]Update, 0, 0)
					} else {
						out.Updates = []Update{}
					}
				} else {
					out.Updates = (out.Updates)[:0]
				}
				for !in.IsDelim(']') {
					var v11 Update
					(v11).UnmarshalEasyJSON(in)
					out.Updates = append(out.Updates, v11)
					in.WantComma()
				}
				in.Delim(']')
			}
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson89aae3efEncodeGithubComMunnikGoskSignalk6(out *jwriter.Writer, in Delta) {
	out.RawByte('{')
	first := true
	_ = first
	if in.Context != "" {
		const prefix string = ",\"context\":"
		first = false
		out.RawString(prefix[1:])
		out.String(string(in.Context))
	}
	{
		const prefix string = ",\"updates\":"
		if first {
			first = false
			out.RawString(prefix[1:])
		} else {
			out.RawString(prefix)
		}
		if in.Updates == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v12, v13 := range in.Updates {
				if v12 > 0 {
					out.RawByte(',')
				}
				(v13).MarshalEasyJSON(out)
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v Delta) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk6(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v Delta) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson89aae3efEncodeGithubComMunnikGoskSignalk6(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *Delta) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk6(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *Delta) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson89aae3efDecodeGithubComMunnikGoskSignalk6(l, v)
}
