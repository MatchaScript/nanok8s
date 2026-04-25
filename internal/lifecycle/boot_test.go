package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/MatchaScript/nanok8s/internal/state"
	"github.com/MatchaScript/nanok8s/internal/testutil"
)

func TestShortID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "short"},
		{"exactly12chr", "exactly12chr"},
		{"longerthantwelvechars", "longerthantw"},
	}
	for _, tc := range cases {
		if got := shortID(tc.in); got != tc.want {
			t.Errorf("shortID(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestShortPair_JoinsWithUnderscore(t *testing.T) {
	got := shortPair("deployment-abcdef-1234", "boot-id-abcdef-5678")
	// First 12 chars of each, joined.
	if got != "deployment-a_boot-id-abcd" {
		t.Errorf("shortPair = %q", got)
	}
}

// bootFailed writes a human-readable last-event and returns the original
// error verbatim. Both branches (upgrade vs. steady-state) must be
// distinguishable because operators read last-event from MOTD and the
// phrasing drives their debugging.
func TestBootFailed_WritesUpgradeEventAndReturnsCause(t *testing.T) {
	testutil.UseTempPaths(t)

	cause := errors.New("kubelet refused to start")
	err := bootFailed(true, "v1.35.0", "v1.36.0", cause)
	if !errors.Is(err, cause) {
		t.Errorf("bootFailed returned %v; must wrap cause", err)
	}
	event, err := state.ReadLastEvent()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"boot failed upgrading", "v1.35.0", "v1.36.0", "kubelet refused"} {
		if !strings.Contains(event, want) {
			t.Errorf("event %q missing %q", event, want)
		}
	}
}

func TestBootFailed_WritesSteadyStateEvent(t *testing.T) {
	testutil.UseTempPaths(t)

	cause := errors.New("ensure: PKI gone")
	_ = bootFailed(false, "", "v1.35.0", cause)

	event, err := state.ReadLastEvent()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(event, "boot failed at v1.35.0") {
		t.Errorf("event %q missing 'boot failed at v1.35.0'", event)
	}
	if strings.Contains(event, "upgrading") {
		t.Errorf("event %q should not mention upgrade when not upgrading", event)
	}
}
