package writer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/munnik/gosk/message"
)

func TestSignalKDeltaLeavesOutDiagnostics(t *testing.T) {
	u := message.NewUpdate().WithSource(*message.NewSource().WithLabel("l")).WithTimestamp(time.Now())
	u.AddValue(message.NewValue().WithPath("a.b").WithValue(nil).WithDiagnostics(map[string]any{message.DiagnosticRejected: "all zeros"}))
	u.AddValue(message.NewValue().WithPath("c.d").WithValue(1.0))
	in := message.NewMapped().WithContext("c").WithOrigin("c").AddUpdate(u)

	out := withoutDiagnostics(in)
	blob, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "diagnostics") {
		t.Fatalf("a Signal K delta carries diagnostics: %s", blob)
	}
	if len(out.Updates[0].Values) != 2 || out.Updates[0].Values[1].Value != 1.0 {
		t.Fatalf("values lost: %s", blob)
	}
	if in.Updates[0].Values[0].Diagnostics == nil {
		t.Fatal("the shared message was modified")
	}

	plain := message.NewMapped().WithContext("c").WithOrigin("c")
	if withoutDiagnostics(plain) != plain {
		t.Fatal("a message without diagnostics should not be copied")
	}
}
