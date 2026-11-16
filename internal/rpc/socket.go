package rpc

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("make the socket's directory %s: %w", dir, err)
	}
	return nil
}

// clearPath makes 1 socket path free for a new listener: an absent path is free,
// a path that holds a socket that refuses a connection is stale and is removed,
// and a live socket or another kind of file is an error.
func clearPath(path string) error {
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
	conn, err := net.DialTimeout("unix", path, dialTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("another frostmarksd serves %s", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove the stale socket %s: %w", path, err)
	}
	return nil
}

// chmod sets the permission of 1 path.
func chmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		return fmt.Errorf("set the permission of %s: %w", path, err)
	}
	return nil
}
