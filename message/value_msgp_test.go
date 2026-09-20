package message

import (
	"reflect"
	"testing"
)

// TestValueMsgpRoundTrip exercises every valueTag marshalValueMsg can
// produce, plus the JSON fallback for a type with no tag of its own -
// this is what guarantees Decode()'s candidate list and value_msgp.go's
// switches can't silently drift apart without a test catching it.
func TestValueMsgpRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		// want overrides the expected decoded value when it legitimately
		// differs from v.Value (the JSON-fallback case below: Decode()
		// converts the loosely-typed input into its canonical Go type,
		// same as it always did for JSON). Leave nil to assert v.Value
		// round-trips unchanged.
		want interface{}
	}{
		{name: "nil", v: Value{Path: "notifications.cleared", Value: nil}},
		{name: "float64", v: Value{Path: "propulsion.mainEngine.drive.power", Value: 1234.5}},
		{name: "int64", v: Value{Path: "some.int.path", Value: int64(42)}},
		{name: "string", v: Value{Path: "some.string.path", Value: "hello"}},
		{name: "bool", v: Value{Path: "some.bool.path", Value: true}},
		{name: "Position", v: Value{Path: "navigation.position", Value: Position{Latitude: float64Ptr(52.1), Longitude: float64Ptr(5.9)}}},
		{name: "VesselInfo", v: Value{Path: "name", Value: VesselInfo{Name: stringPtr("Amazing Grace")}}},
		{name: "VesselType", v: Value{Path: "design.aisShipType", Value: VesselType{Id: intPtr(37), Description: stringPtr("Pleasure Craft")}}},
		{name: "Length", v: Value{Path: "design.length", Value: Length{Overall: float64Ptr(42.0)}}},
		{name: "Notification", v: Value{Path: "notifications.test.alarm", Value: Notification{State: stringPtr("alarm"), Method: []string{"sound", "visual"}, Message: stringPtr("test")}}},
		{name: "Notification with nil fields", v: Value{Path: "notifications.test.cleared", Value: Notification{}}},
		{name: "[]Notification empty", v: Value{Path: "notifications.test.list", Value: []Notification{}}},
		{name: "[]Notification", v: Value{Path: "notifications.test.list", Value: []Notification{
			{State: stringPtr("alarm")},
			{State: stringPtr("warn")},
		}}},
		{name: "Draft", v: Value{Path: "design.draft", Value: Draft{Current: float64Ptr(3.2), CurrentPort: []float64{3.1, 3.2}}}},
		{name: "Spectrum", v: Value{Path: "propulsion.mainEngine.drive.torqueSpectrum", Value: Spectrum{
			NumberOfSamples:   16384,
			FrequencyStepSize: 0.5,
			Duration:          8.192,
			Coefficients:      []Coefficient{{Magnitude: 1.0, Phase: 0.5}, {Magnitude: 2.0, Phase: 1.5}},
		}}},
		{name: "Vector3D", v: Value{Path: "some.vector.path", Value: Vector3D{X: 1, Y: 2, Z: 3}}},
		{name: "Current", v: Value{Path: "environment.current", Value: Current{Drift: float64Ptr(0.35), SetTrue: float64Ptr(1.57)}}},
		// exercises the fallback branch itself: value_msgp.go has no case
		// for map[string]interface{} (this is what an expr literal like
		// mapper/modbus_test.yaml's '{"state": ..., "message": ...}'
		// produces before Decode() sees it), so this round-trips through
		// the same JSON+Decode() path Value always used - landing on the
		// canonical Notification, not the original map.
		{
			name: "JSON fallback (map decodes via Decode())",
			v:    Value{Path: "notifications.test", Value: map[string]interface{}{"state": "alarm", "message": "test"}},
			want: Notification{State: stringPtr("alarm"), Message: stringPtr("test")},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := c.want
			if want == nil {
				want = c.v.Value
			}

			b, err := marshalValueMsg(c.v, nil)
			if err != nil {
				t.Fatalf("marshalValueMsg: %v", err)
			}
			decoded, rest, err := unmarshalValueMsg(b)
			if err != nil {
				t.Fatalf("unmarshalValueMsg: %v", err)
			}
			if len(rest) != 0 {
				t.Fatalf("expected no leftover bytes, got %d", len(rest))
			}
			if decoded.Path != c.v.Path {
				t.Fatalf("path differs: got %q, want %q", decoded.Path, c.v.Path)
			}
			if !reflect.DeepEqual(decoded.Value, want) {
				t.Fatalf("value differs:\n got:  %#v (%T)\n want: %#v (%T)", decoded.Value, decoded.Value, want, want)
			}
		})
	}
}

func intPtr(i int) *int { return &i }
