package rpc

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// This file takes the socket that systemd passed to the daemon (sd_listen_fds):
// the socket unit owns the path, its owner and its mode, and the daemon serves
// the descriptor instead of creating the socket itself.

// The environment of a socket that systemd passed.
const (
	// listenPIDEnv is the process id that the descriptors belong to.
	listenPIDEnv = "LISTEN_PID"
	// listenFDsEnv is how many descriptors systemd passed, from 3.
	listenFDsEnv = "LISTEN_FDS"
	// listenFDNAMESEnv is the name of each descriptor, colon-separated, in the
	// order FileDescriptorName gives them.
	listenFDNAMESEnv = "LISTEN_FDNAMES"
	// firstListenFD is the first descriptor that systemd passes.
	firstListenFD = 3
)

// ListenActivated returns the socket that systemd passed to this process under
// name, such as the socket unit's FileDescriptorName.
//
// ok is false when systemd passed no socket: the variables are absent, they name
// another process, or no descriptor carries the name. The daemon then creates the
// socket itself. A variable that is there and malformed is an error.
func ListenActivated(name string, getenv func(string) string) (listener net.Listener, ok bool, err error) {
	return listenActivated(name, getenv, os.Getpid(), func(fd uintptr) *os.File { return os.NewFile(fd, name) })
}

// listenActivated is ListenActivated with the process id and the descriptor
// source of a caller, so that a test gives a real listener's file instead of a
// descriptor number and a process id of its own.
func listenActivated(name string, getenv func(string) string, pid int, file func(fd uintptr) *os.File) (net.Listener, bool, error) {
	assigned := getenv(listenPIDEnv)
	count := getenv(listenFDsEnv)
	if assigned == "" || count == "" {
		return nil, false, nil
	}
	owner, err := strconv.Atoi(strings.TrimSpace(assigned))
	if err != nil {
		return nil, false, fmt.Errorf("the socket activation's %s is %q, which is no process id", listenPIDEnv, assigned)
	}
	fds, err := strconv.Atoi(strings.TrimSpace(count))
	if err != nil {
		return nil, false, fmt.Errorf("the socket activation's %s is %q, which is no number of descriptors", listenFDsEnv, count)
	}
	if owner != pid {
		// The variables belong to another process, such as a parent that
		// passed its environment on: the descriptors are not this process's.
		return nil, false, nil
	}
	if fds <= 0 {
		return nil, false, nil
	}
	names := strings.Split(getenv(listenFDNAMESEnv), ":")
	for i := range fds {
		if i >= len(names) || names[i] != name {
			continue
		}
		socket := file(uintptr(firstListenFD + i))
		if socket == nil {
			return nil, false, fmt.Errorf("the socket activation's descriptor %d is no file", firstListenFD+i)
		}
		listener, listenErr := net.FileListener(socket)
		// The listener holds its own copy of the descriptor, so the file that
		// net.FileListener read is closed here.
		closeErr := socket.Close()
		if listenErr != nil {
			return nil, false, fmt.Errorf("read the socket activation's descriptor %d as a listener: %w", firstListenFD+i, listenErr)
		}
		if closeErr != nil {
			_ = listener.Close()
			return nil, false, fmt.Errorf("close the socket activation's descriptor %d: %w", firstListenFD+i, closeErr)
		}
		return listener, true, nil
	}
	return nil, false, nil
}
