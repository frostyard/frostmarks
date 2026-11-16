// Command stage writes the install tree of frostmarks' .deb, under the root that
// nFPM packs into /usr: the app's 2 binaries, its lodge files (app.json, the
// hearth manifest and the tool catalog), its web bundle, its 2 units and its
// sysusers.d entry (docs/specs/lodge-app.md, and lodge's
// docs/specs/app-contract.md, Install layout).
//
//	go run ./internal/cmd/stage -version 0.1.0 -bin build/deb/bin -web web/dist -out build/deb/root
//
// The command reads the repository's own files, so it runs from the repository's
// root. It writes nothing outside ROOT/usr, and it refuses a web directory with a
// symlink in it: lodge serves the bundle from root-owned directories, and a link
// could point anywhere.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	marksv1 "github.com/frostyard/frostmarks/gen/go/frostyard/frostmarks/v1"
	"github.com/frostyard/frostmarks/internal/catalog"
)

// The files of the install tree, relative to ROOT/usr, and their sources in the
// repository.
const (
	// usrDir is the tree's only directory: an install writes /usr.
	usrDir = "usr"
	// appDir is the app's directory under /usr/share/lodge/apps.
	appDir = "share/lodge/apps/marks"
	// manifestSource is the hearth manifest that the install copies.
	manifestSource = "gen/manifest.json"
	// appJSONSource is the app's lodge manifest, whose version the command
	// sets.
	appJSONSource = "packaging/lodge/app.json"
	// systemdSource is the directory of the app's 2 units.
	systemdSource = "packaging/systemd"
	// sysusersSource is the app's sysusers.d entry.
	sysusersSource = "packaging/sysusers.d/frostmarks.conf"
	// binaryPerm is the permission of a binary.
	binaryPerm = 0o755
	// filePerm is the permission of every other file.
	filePerm = 0o644
	// dirPerm is the permission of every directory of the tree.
	dirPerm = 0o755
)

// binaries are the app's 2 binaries, from -bin into usr/bin.
var binaries = []string{"frostmarksd", "marksctl"}

// units are the app's 2 units, from packaging/systemd into the units' directory.
var units = []string{"frostmarks.service", "frostmarks.socket"}

// webEntry is the file that a web bundle must hold, and its icon.
const (
	// webIndex is the bundle's page.
	webIndex = "index.html"
	// webIcon is the app's icon, which app.json names.
	webIcon = "icon.svg"
)

func main() {
	version := flag.String("version", "", "the app's version, which app.json names")
	binDir := flag.String("bin", "", "the directory of the 2 built binaries")
	webDir := flag.String("web", "", "the directory of the built web bundle")
	out := flag.String("out", "", "the root of the install tree: the command writes OUT/usr")
	flag.Parse()
	if err := stage(*version, *binDir, *webDir, *out); err != nil {
		fmt.Fprintf(os.Stderr, "stage: %v\n", err)
		os.Exit(1)
	}
}

// stage writes the install tree of 1 version under out.
func stage(version, binDir, webDir, out string) error {
	switch {
	case version == "":
		return errors.New("-version is required")
	case binDir == "":
		return errors.New("-bin is required")
	case webDir == "":
		return errors.New("-web is required")
	case out == "":
		return errors.New("-out is required")
	}
	// A web directory without its page or its icon would install an app that
	// lodge disables, so the tree is refused instead.
	for _, name := range []string{webIndex, webIcon} {
		if err := requireFile(filepath.Join(webDir, name)); err != nil {
			return fmt.Errorf("the web directory %s: %w", webDir, err)
		}
	}
	usr := filepath.Join(out, usrDir)
	for _, rel := range []string{
		"bin",
		appDir,
		filepath.Join(appDir, "web"),
		"lib/systemd/system",
		"lib/sysusers.d",
	} {
		if err := makeDir(filepath.Join(usr, rel)); err != nil {
			return err
		}
	}
	for _, name := range binaries {
		if err := copyFile(filepath.Join(binDir, name), filepath.Join(usr, "bin", name), binaryPerm); err != nil {
			return err
		}
	}
	if err := writeAppJSON(filepath.Join(usr, appDir, "app.json"), version); err != nil {
		return err
	}
	if err := copyFile(manifestSource, filepath.Join(usr, appDir, "manifest.json"), filePerm); err != nil {
		return err
	}
	tools, err := catalog.Tools(marksv1.MarksTools(nil))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(usr, appDir, "tools.json"), tools, filePerm); err != nil {
		return fmt.Errorf("write the tool catalog: %w", err)
	}
	if err := os.Chmod(filepath.Join(usr, appDir, "tools.json"), filePerm); err != nil {
		return fmt.Errorf("set the permission of the tool catalog: %w", err)
	}
	for _, name := range units {
		if err := copyFile(filepath.Join(systemdSource, name), filepath.Join(usr, "lib/systemd/system", name), filePerm); err != nil {
			return err
		}
	}
	if err := copyFile(sysusersSource, filepath.Join(usr, "lib/sysusers.d/frostmarks.conf"), filePerm); err != nil {
		return err
	}
	return copyTree(webDir, filepath.Join(usr, appDir, "web"))
}

// makeDir makes 1 directory, and its parents, with the tree's permission.
//
// os.MkdirAll masks the permission with the process's umask, and the install
// tree's modes are exact: each directory that the call makes is set to the tree's
// mode once it exists.
func makeDir(dir string) error {
	missing, err := missingDirs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("make the directory %s: %w", dir, err)
	}
	for _, path := range missing {
		if err := os.Chmod(path, dirPerm); err != nil {
			return fmt.Errorf("set the permission of %s: %w", path, err)
		}
	}
	return nil
}

// missingDirs returns the directories of 1 path that are not there, from the path
// itself up to the first directory that is.
func missingDirs(dir string) ([]string, error) {
	var missing []string
	for path := filepath.Clean(dir); ; path = filepath.Dir(path) {
		if _, err := os.Lstat(path); err == nil {
			return missing, nil
		}
		missing = append(missing, path)
		if parent := filepath.Dir(path); parent == path {
			return missing, nil
		}
	}
}

// requireFile checks that 1 path is a regular file.
func requireFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is no regular file", path)
	}
	return nil
}

// copyFile copies 1 regular file to 1 path, with the given permission.
func copyFile(src, dst string, perm os.FileMode) error {
	if err := requireFile(src); err != nil {
		return err
	}
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	defer func() {
		_ = source.Close()
	}()
	target, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	// The open file's permission is the umask's, and the tree's is exact.
	if err := os.Chmod(dst, perm); err != nil {
		return fmt.Errorf("set the permission of %s: %w", dst, err)
	}
	return nil
}

// copyTree copies the directory src into dst: its directories (0755) and its
// regular files (0644). A symlink, a device or a named pipe is an error, because
// the bundle is root's, and lodge serves it from there.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case entry.IsDir():
			return makeDir(target)
		case entry.Type().IsRegular():
			return copyFile(path, target, filePerm)
		}
		return fmt.Errorf("%s is no directory and no regular file, and a web bundle holds neither links nor devices", path)
	})
}

// writeAppJSON writes the app's lodge manifest at dst: the repository's app.json,
// with the version set, indented, and ending with 1 newline.
//
// The members are written in the order of their names, which is no change that
// lodge reads: it parses the file as a JSON object.
func writeAppJSON(dst, version string) error {
	data, err := os.ReadFile(appJSONSource)
	if err != nil {
		return fmt.Errorf("read %s: %w", appJSONSource, err)
	}
	var app map[string]json.RawMessage
	if err := json.Unmarshal(data, &app); err != nil {
		return fmt.Errorf("read %s: %w", appJSONSource, err)
	}
	encoded, err := json.Marshal(version)
	if err != nil {
		return fmt.Errorf("write the version %q: %w", version, err)
	}
	app["version"] = encoded
	out, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if err := os.WriteFile(dst, append(out, '\n'), filePerm); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if err := os.Chmod(dst, filePerm); err != nil {
		return fmt.Errorf("set the permission of %s: %w", dst, err)
	}
	return nil
}
