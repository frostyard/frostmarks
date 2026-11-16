package main

import (
	"bytes"
	"strings"
	"testing"
)

// noEnv is the environment of a CLI that has no variable: no socket and no state
// directory.
func noEnv(string) string {
	return ""
}

// TestUsageErrors checks that each usage error prints the usage and exits 2.
func TestUsageErrors(t *testing.T) {
	tests := []struct {
		// name names the case.
		name string
		// args are the CLI's arguments.
		args []string
	}{
		{name: "no command", args: nil},
		{name: "an unknown command", args: []string{"nonsense"}},
		{name: "no URL", args: []string{"create-bookmark"}},
		{name: "no id", args: []string{"get-bookmark"}},
		{name: "an extra argument", args: []string{"get-bookmark", "a", "b"}},
		{name: "a missing argument", args: []string{"rename-folder", "/a"}},
		{name: "a missing folder", args: []string{"move-bookmark", "an-id"}},
		{name: "a bad flag", args: []string{"list-bookmarks", "-bogus"}},
		{name: "no query", args: []string{"search-bookmarks"}},
		{name: "an argument of mcp", args: []string{"mcp", "extra"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(test.args, strings.NewReader(""), &stdout, &stderr, noEnv)
			if code != exitUsage {
				t.Fatalf("run(%v) = %d, want %d (stderr %q)", test.args, code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), "usage: marksctl") {
				t.Errorf("the stderr = %q, and it holds no usage", stderr.String())
			}
		})
	}
}

// TestHelp checks that help exits 0 with the usage, and that it needs no
// environment and no socket.
func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, strings.NewReader(""), &stdout, &stderr, noEnv); code != exitOK {
		t.Fatalf("run(help) = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "usage: marksctl") {
		t.Errorf("the stderr = %q, and it holds no usage", stderr.String())
	}
}

// TestNoSocket checks that a command without a socket path prints the error of
// the path and exits 1.
func TestNoSocket(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"list-bookmarks"}, strings.NewReader(""), &stdout, &stderr, noEnv); code != exitError {
		t.Fatalf("run(list-bookmarks) = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "no socket path") {
		t.Errorf("the stderr = %q, and it holds no socket error", stderr.String())
	}
}
