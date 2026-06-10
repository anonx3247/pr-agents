package core

import (
	"testing"
)

func TestEnvConstantsArePrefixed(t *testing.T) {
	for _, name := range []string{
		EnvSession, EnvHarness, EnvLauncher,
	} {
		if len(name) < 5 || name[:4] != "PRA_" {
			t.Errorf("env constant %q is not PRA_-prefixed", name)
		}
	}
}
