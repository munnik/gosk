package database

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsConcurrentCatalogChange(t *testing.T) {
	// What golang-migrate hands back: the server's message wrapped in the
	// driver's, with the migration's text in between.
	wrapped := fmt.Errorf("migration failed: %w", errors.New("tuple concurrently deleted in line 0: -- The count-based transfer protocol ..."))
	for _, c := range []struct {
		err  error
		want bool
	}{
		{wrapped, true},
		{errors.New("tuple concurrently updated"), true},
		{errors.New("cannot drop desired object(s) because other objects depend on them"), false},
		{errors.New("Dirty database version 20261017120100. Fix and force version."), false},
	} {
		if got := isConcurrentCatalogChange(c.err); got != c.want {
			t.Errorf("isConcurrentCatalogChange(%q) = %v, want %v", c.err, got, c.want)
		}
	}
}
