package aprsutils

import "testing"

func TestPasscode(t *testing.T) {
	N0CALL := 13023

	withoutSSID := Passcode("N0CALL")
	withSSID := Passcode("N0CALL-10")

	if withoutSSID != N0CALL {
		t.Error("N0CALL passcode mismatch")
	}

	if withSSID != N0CALL {
		t.Error("N0CALL-10 passcode mismatch")
	}
}
