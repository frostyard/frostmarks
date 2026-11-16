package rpc

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// parentDir returns the directory of 1 socket path, or "" when the path carries
// no directory.
func parentDir(path string) string {
	dir := filepath.Dir(path)
	if dir == "." {
		return ""
	}
	return dir
}

// makeDir makes 1 directory and its parents, with the permission of a socket's
// directory. A directory that is there keeps its permission.
func makeDir(dir string) error {
	return makeDirMode(dir, dirPerm)
}

// makeDirMode makes 1 directory and its parents with the given permission. A
// directory that is there keeps its permission.
//
// os.MkdirAll masks the permission with the process's umask, and the lodge
// socket's directory is 0755 whatever the daemon's umask is: each directory that
// the call makes is set to perm once it exists.
func makeDirMode(dir string, perm os.FileMode) error {
	missing := missingDirs(dir)
	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("make the socket's directory %s: %w", dir, err)
	}
	for _, path := range missing {
		if err := os.Chmod(path, perm); err != nil {
			return fmt.Errorf("set the permission of %s: %w", path, err)
		}
	}
	return nil
}

// missingDirs returns the directories of 1 path that are not there, from the
// path itself up to the first directory that is.
func missingDirs(dir string) []string {
	var missing []string
	for path := filepath.Clean(dir); ; path = filepath.Dir(path) {
		if _, err := os.Lstat(path); err == nil {
			return missing
		}
		missing = append(missing, path)
		if parent := filepath.Dir(path); parent == path {
			return missing
		}
	}
}

// dialSocket dials 1 socket path, and it is the seam of the tests.
type dialSocket func(path string) (net.Conn, error)

// dialTimeoutSocket dials 1 socket path, and it waits dialTimeout.
func dialTimeoutSocket(path string) (net.Conn, error) {
	return net.DialTimeout("unix", path, dialTimeout)
}

// clearPath makes 1 socket path free for a new listener: an absent path is free,
// a path that holds a socket that the kernel refuses is stale and is removed, and
// a live socket, another kind of file, and a socket that fails for another reason
// are errors that leave the path as it is.
//
// The dial's error decides. Only ECONNREFUSED means that nothing listens at the
// path: a timeout, a permission error or a full backlog say nothing about what
// holds it, and a removed socket of a daemon that is merely slow would let a
// second daemon take its place.
func clearPath(path string, dial dialSocket) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists, and it is no socket", path)
	}
	conn, err := dial(path)
	switch {
	case err == nil:
		_ = conn.Close()
		return fmt.Errorf("another frostmarksd serves %s", path)
	case errors.Is(err, syscall.ECONNREFUSED):
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove the stale socket %s: %w", path, err)
		}
		return nil
	}
	return fmt.Errorf("dial the socket %s: %w", path, err)
}

// chmod sets the permission of 1 path.
func chmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("set the permission of %s: %w", path, err)
	}
	return nil
}
