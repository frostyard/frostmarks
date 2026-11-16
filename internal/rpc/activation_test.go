package rpc

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// itoa returns 1 int as its decimal text.
func itoa(value int) string {
	return strconv.Itoa(value)
}

// env returns a getenv of a fixed environment.
func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// socketFile returns a real listening socket's file, and it closes both the
// listener and the file when the test ends.
func socketFile(t *testing.T) *os.File {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(shortDir(t), "systemd.sock"))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		t.Fatalf("the listener is a %T, and not a *net.UnixListener", listener)
	}
	file, err := unixListener.File()
	if err != nil {
		t.Fatalf("read the listener's file: %v", err)
	}
	t.Cleanup(func() {
		// Closing the listener of net.FileListener may have closed this file
		// already, and a second close is then no failure of the test.
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close the file: %v", err)
		}
		if err := listener.Close(); err != nil {
			t.Errorf("close the listener: %v", err)
		}
	})
	return file
}

// unusedFile is the seam of a test whose case never reads a descriptor: a read
// is a failure of the case.
func unusedFile(t *testing.T) func(uintptr) *os.File {
	return func(uintptr) *os.File {
		t.Helper()
		t.Fatal("the case read a descriptor, and it must not")
		return nil
	}
}

// TestListenActivated checks the rules of a socket that systemd passed: the
// variables name this process and the name of a descriptor, and no other case
// hands the daemon a socket.
func TestListenActivated(t *testing.T) {
	t.Run("no variables passed no socket", func(t *testing.T) {
		listener, ok, err := listenActivated("marks", env(nil), os.Getpid(), unusedFile(t))
		if listener != nil || ok || err != nil {
			t.Fatalf("listenActivated = (%v, %v, %v), want (nil, false, nil)", listener, ok, err)
		}
	})

	t.Run("another process's descriptors are not this process's", func(t *testing.T) {
		listener, ok, err := listenActivated("marks", env(map[string]string{
			"LISTEN_PID":     "1",
			"LISTEN_FDS":     "1",
			"LISTEN_FDNAMES": "marks",
		}), os.Getpid(), unusedFile(t))
		if listener != nil || ok || err != nil {
			t.Fatalf("listenActivated = (%v, %v, %v), want (nil, false, nil)", listener, ok, err)
		}
	})

	t.Run("the name is found among 2", func(t *testing.T) {
		file := socketFile(t)
		listener, ok, err := listenActivated("marks", env(map[string]string{
			"LISTEN_PID":     itoa(os.Getpid()),
			"LISTEN_FDS":     "2",
			"LISTEN_FDNAMES": "other:marks",
		}), os.Getpid(), func(uintptr) *os.File { return file })
		if err != nil {
			t.Fatalf("listenActivated: %v", err)
		}
		if !ok || listener == nil {
			t.Fatalf("listenActivated = (%v, %v), want the socket of the name", listener, ok)
		}
		if err := listener.Close(); err != nil {
			t.Errorf("close the listener: %v", err)
		}
	})

	t.Run("a name that no descriptor carries passed no socket", func(t *testing.T) {
		listener, ok, err := listenActivated("marks", env(map[string]string{
			"LISTEN_PID":     itoa(os.Getpid()),
			"LISTEN_FDS":     "2",
			"LISTEN_FDNAMES": "other:another",
		}), os.Getpid(), unusedFile(t))
		if listener != nil || ok || err != nil {
			t.Fatalf("listenActivated = (%v, %v, %v), want (nil, false, nil)", listener, ok, err)
		}
	})

	t.Run("malformed variables are errors", func(t *testing.T) {
		for _, test := range []struct {
			// name names the case.
			name string
			// values is the environment.
			values map[string]string
		}{
			{name: "a pid that is no number", values: map[string]string{"LISTEN_PID": "3a", "LISTEN_FDS": "1"}},
			{name: "a count that is no number", values: map[string]string{"LISTEN_PID": itoa(os.Getpid()), "LISTEN_FDS": "many"}},
		} {
			t.Run(test.name, func(t *testing.T) {
				listener, ok, err := listenActivated("marks", env(test.values), os.Getpid(), unusedFile(t))
				if err == nil {
					t.Fatalf("listenActivated = (%v, %v), and it must fail", listener, ok)
				}
			})
		}
	})
}
