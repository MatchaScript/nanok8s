package main

import (
	"testing"

	"github.com/MatchaScript/nanok8s/internal/state"
	"github.com/MatchaScript/nanok8s/internal/testutil"
)

// seedStateAsExisting repoints the paths package at t.TempDir() and seeds
// the last-event file, matching what state.Exists() will report after a
// previous bootstrap.
func seedStateAsExisting(t *testing.T) {
	t.Helper()
	testutil.UseTempPaths(t)
	if err := state.WriteLastEvent("pre-existing"); err != nil {
		t.Fatal(err)
	}
}
