// Package paths is the app's default locations: the state directory that holds
// the database, and the private socket.
//
// Both follow the XDG base directory specification, and both are overridable by
// the daemon's flags: the packaging of a host that runs frostmarksd as a service
// passes its own paths.
package paths

import (
	"errors"
	"path/filepath"
)

// The environment variables that name the app's 2 directories.
const (
	// stateHomeEnv is the variable of the state directory's parent.
	stateHomeEnv = "XDG_STATE_HOME"
	// runtimeDirEnv is the variable of the runtime directory.
	runtimeDirEnv = "XDG_RUNTIME_DIR"
	// homeEnv is the variable of the user's home directory.
	homeEnv = "HOME"
)

// The app's names in those directories.
const (
	// dirName is the app's directory, and its file name stem.
	dirName = "frostmarks"
	// socketName is the private socket's file name.
	socketName = "frostmarks.sock"
)

// StateDir returns the app's state directory: $XDG_STATE_HOME/frostmarks, or
// $HOME/.local/state/frostmarks when the variable is unset.
func StateDir(getenv func(string) string) (string, error) {
	if dir := getenv(stateHomeEnv); dir != "" {
		return filepath.Join(dir, dirName), nil
	}
	if home := getenv(homeEnv); home != "" {
		return filepath.Join(home, ".local", "state", dirName), nil
	}
	return "", errors.New("no state directory: set " + stateHomeEnv + " or " + homeEnv + ", or pass -state-dir")
}

// Socket returns the app's private socket: $XDG_RUNTIME_DIR/frostmarks.sock.
//
// A host without a runtime directory has no default: its packaging passes the
// socket's path.
func Socket(getenv func(string) string) (string, error) {
	if dir := getenv(runtimeDirEnv); dir != "" {
		return filepath.Join(dir, socketName), nil
	}
	return "", errors.New("no socket path: set " + runtimeDirEnv + ", or pass -socket")
}
