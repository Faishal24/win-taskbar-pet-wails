package main

import "testing"

// Roundtrips the real Run entry but always restores the prior state, so the
// machine is left exactly as found.
func TestAutostartRoundtrip(t *testing.T) {
	before := autostartEnabled()
	set := func(want bool) {
		t.Helper()
		if err := setAutostart(want); err != nil {
			t.Fatalf("setAutostart(%v): %v", want, err)
		}
		if got := autostartEnabled(); got != want {
			t.Fatalf("after setAutostart(%v): enabled=%v", want, got)
		}
	}
	set(true)
	set(false)
	if before {
		set(true)
	}
}
