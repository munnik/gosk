package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
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
		{errors.New("Dirty database version 20261031130000. Fix and force version."), false},
	} {
		if got := isConcurrentCatalogChange(c.err); got != c.want {
			t.Errorf("isConcurrentCatalogChange(%q) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestCheckSchemaVersion(t *testing.T) {
	const newest = 20261031130500
	for _, c := range []struct {
		name        string
		version     uint
		dirty, none bool
		ok          bool
	}{
		{"at the newest", newest, false, false, true},
		{"no version", 0, false, true, false},
		{"dirty at the newest", newest, true, false, false},
		{"behind", 20261015120000, false, false, false},
		{"ahead", 20261031130600, false, false, false},
	} {
		if err := checkSchemaVersion(c.version, c.dirty, c.none, newest); (err == nil) != c.ok {
			t.Errorf("%s: checkSchemaVersion = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestNewestMigrationIsTheLastEmbedded(t *testing.T) {
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	newest, err := newestMigration(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Next(newest); err == nil {
		t.Errorf("newestMigration = %d, but there is a migration after it", newest)
	}
	if newest < 20261031130500 {
		t.Errorf("newestMigration = %d, older than migrations known to exist", newest)
	}
}
