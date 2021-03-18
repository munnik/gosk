package mapper

import (
	"reflect"
	"testing"
	"time"

	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/nanomsg"
)

func TestDeltaFromData(t *testing.T) {
	const invalidDataType string = "INVALID"
	type args struct {
		data     []byte
		dataType string
	}
	tests := []struct {
		name    string
		args    args
		want    signalk.Delta
		wantErr bool
	}{
		{name: "Invalid data type", args: args{data: []byte{}, dataType: invalidDataType}, want: signalk.DeltaWithContext{}, wantErr: true},
		// TODO should errors from the nmea library result in an error or just an empty delta?
		{name: "Empty bytes NMEA message", args: args{data: []byte{}, dataType: NMEA0183Type}, want: signalk.DeltaWithContext{}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := nanomsg.NewMessage(test.args.data, time.Now(), []byte("collector"), []byte(test.args.dataType), []byte("test"))
			got, err := KeyValueFromData(m)
			if (err != nil) != test.wantErr {
				t.Errorf("DeltaFromData() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("DeltaFromData() = %v, want %v", got, test.want)
			}
		})
	}
}
