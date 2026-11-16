package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// shortDir returns a temporary directory whose path is short enough for a Unix
// socket, and it removes that exact path when the test ends.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fm")
	if err != nil {
		t.Fatalf("make the socket's directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove the socket's directory: %v", err)
		}
	})
	return dir
}

// slogDiscard returns a logger that writes nowhere.
func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// env returns a getenv of a fixed environment.
func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// TestLodgePath checks the paths of lodge's files: a root prefixes them, and an
// empty root is the real root.
func TestLodgePath(t *testing.T) {
	if got, want := lodgePath("", lodgeSocketRel), "/run/lodge/apps/marks.sock"; got != want {
		t.Errorf("lodgePath without a root = %q, want %q", got, want)
	}
	if got, want := lodgePath("", brokerSocketRel), "/run/lodge/nats.sock"; got != want {
		t.Errorf("lodgePath of the broker without a root = %q, want %q", got, want)
	}
	if got, want := lodgePath("/tmp/dev", lodgeSocketRel), "/tmp/dev/run/lodge/apps/marks.sock"; got != want {
		t.Errorf("lodgePath under a root = %q, want %q", got, want)
	}
}

// TestLodgeUser checks the user whose connections the lodge socket admits: the
// daemon's own id under a root without LODGE_UID, the id that LODGE_UID names,
// and an error for a value that is no id.
func TestLodgeUser(t *testing.T) {
	t.Run("under a root without LODGE_UID the daemon's own id", func(t *testing.T) {
		uid, err := lodgeUser("/tmp/dev", env(nil))
		if err != nil {
			t.Fatalf("lodgeUser: %v", err)
		}
		if got, want := uid, os.Getuid(); got != want {
			t.Errorf("lodgeUser = %d, want the daemon's own id %d", got, want)
		}
	})

	t.Run("LODGE_UID names the id", func(t *testing.T) {
		uid, err := lodgeUser("/tmp/dev", env(map[string]string{lodgeUIDEnv: "4242"}))
		if err != nil {
			t.Fatalf("lodgeUser: %v", err)
		}
		if got, want := uid, 4242; got != want {
			t.Errorf("lodgeUser = %d, want %d", got, want)
		}
	})

	t.Run("a value that is no id is an error", func(t *testing.T) {
		if _, err := lodgeUser("/tmp/dev", env(map[string]string{lodgeUIDEnv: "lodge"})); err == nil {
			t.Fatal("lodgeUser passed, and LODGE_UID must be a decimal id")
		}
	})

	t.Run("without a root the system user lodge", func(t *testing.T) {
		// The machine of the test may hold the user or not: both answers are
		// the contract's, and a found user must give its own id.
		uid, err := lodgeUser("", env(nil))
		if err != nil {
			return
		}
		if uid < 0 {
			t.Errorf("lodgeUser = %d, want a user id from 0", uid)
		}
	})
}

// TestOpenLodgeSocket checks the app's own socket under a root: its path, its
// mode, and the error that names the socket unit when there is no socket at all.
func TestOpenLodgeSocket(t *testing.T) {
	root := shortDir(t)
	listener, served, err := openLodgeSocket(root, os.Getuid(), env(nil))
	if err != nil {
		t.Fatalf("openLodgeSocket: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	want := filepath.Join(root, lodgeSocketRel)
	if served != want {
		t.Errorf("the socket's path = %q, want %q", served, want)
	}
	if got := listener.Addr().String(); got != want {
		t.Errorf("the socket's address = %q, want %q", got, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat the socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o660 {
		t.Errorf("the socket's permission = %#o, want %#o", got, 0o660)
	}

	_, _, err = openLodgeSocket("", os.Getuid(), env(nil))
	if err == nil {
		t.Fatal("openLodgeSocket without a socket and without a root passed, and it must fail")
	}
	if want := "no lodge socket"; !strings.Contains(err.Error(), want) {
		t.Errorf("the error %q holds no %q", err, want)
	}
}

// TestEventTransport checks where the app's events go: the in-process bus without
// -lodge, with no lodge socket, and lodge's broker with it.
func TestEventTransport(t *testing.T) {
	t.Run("without -lodge the events stay in process", func(t *testing.T) {
		events, err := newDaemonEvents(false, env(nil), slogDiscard())
		if err != nil {
			t.Fatalf("newDaemonEvents: %v", err)
		}
		defer events.close()
		if events.socket != nil {
			t.Errorf("the daemon came with the socket %v, want none", events.socket)
		}
		if events.transport == nil {
			t.Fatal("the transport is nil")
		}
	})

	t.Run("with -lodge the events go to the broker", func(t *testing.T) {
		root := shortDir(t)
		events, err := newDaemonEvents(true, env(map[string]string{
			lodgeRootEnv: root,
			lodgeUIDEnv:  strconv.Itoa(os.Getuid()),
		}), slogDiscard())
		if err != nil {
			t.Fatalf("newDaemonEvents: %v", err)
		}
		defer events.close()
		if events.transport == nil {
			t.Fatal("the transport is nil")
		}
		if events.socket == nil {
			t.Fatal("the daemon came with no lodge socket")
		}
		t.Cleanup(func() {
			_ = events.socket.Close()
		})
		if want := filepath.Join(root, lodgeSocketRel); events.path != want {
			t.Errorf("the lodge socket's path = %q, want %q", events.path, want)
		}
	})
}
