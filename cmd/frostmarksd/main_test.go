package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunExceptions checks the daemon's exit codes that need no socket: an
// unknown flag and an argument are usage errors, -version passes, and a host
// without a state directory is an error.
func TestRunExceptions(t *testing.T) {
	noEnv := func(string) string { return "" }

	t.Run("an unknown flag", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := run([]string{"-bogus"}, noEnv, &stderr); code != exitUsage {
			t.Errorf("run(-bogus) = %d, want %d", code, exitUsage)
		}
	})

	t.Run("an argument", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := run([]string{"serve"}, noEnv, &stderr); code != exitUsage {
			t.Errorf("run(serve) = %d, want %d", code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "unexpected argument") {
			t.Errorf("the stderr = %q, and it holds no error", stderr.String())
		}
	})

	t.Run("the version", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := run([]string{"-version"}, noEnv, &stderr); code != exitOK {
			t.Fatalf("run(-version) = %d, want %d", code, exitOK)
		}
		if want := "frostmarksd " + version; !strings.Contains(stderr.String(), want) {
			t.Errorf("the stderr = %q, want %q", stderr.String(), want)
		}
	})

	t.Run("no state directory", func(t *testing.T) {
		var stderr bytes.Buffer
		if code := run(nil, noEnv, &stderr); code != exitError {
			t.Fatalf("run with no environment = %d, want %d", code, exitError)
		}
		if !strings.Contains(stderr.String(), "no state directory") {
			t.Errorf("the stderr = %q, and it holds no state directory error", stderr.String())
		}
	})
}
