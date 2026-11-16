// Command debcheck checks the .deb that make deb built, in Go only: the ar
// archive, its control archive and its data archive (docs/specs/lodge-app.md).
//
//	go run ./internal/cmd/debcheck -deb dist/frostyard-frostmarks_0.1.0_amd64.deb -version 0.1.0
//
// It fails, with 1 line for each problem, unless the package's name, version,
// architecture and dependency are frostmarks', the control archive holds no
// maintainer script, every member of the data archive is 1 root-owned file or
// directory under /usr, the 2 binaries are 0755 and not empty, the app's lodge
// files and its 2 units are there, and app.json names the version.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

// The names of the archives of 1 .deb.
const (
	// arMagic starts every ar archive.
	arMagic = "!<arch>\n"
	// arHeaderLen is the length of 1 member's header.
	arHeaderLen = 60
	// arMagicEnd ends every member's header.
	arMagicEnd = "`\n"
	// controlArchive is the name of the control archive.
	controlArchive = "control.tar.gz"
	// dataArchive is the name of the data archive.
	dataArchive = "data.tar.gz"
	// controlFile is the control archive's file of the package's fields.
	controlFile = "control"
	// md5sumsFile is the control archive's file of the members' checksums.
	md5sumsFile = "md5sums"
)

// The control fields that the check reads, and their values for frostmarks.
const (
	// packageField is the package's name.
	packageField = "Package"
	// versionField is the package's version.
	versionField = "Version"
	// architectureField is the package's architecture.
	architectureField = "Architecture"
	// dependsField is what the package needs installed.
	dependsField = "Depends"
	// wantPackage is the package's name.
	wantPackage = "frostyard-frostmarks"
	// wantArchitecture is the package's architecture.
	wantArchitecture = "amd64"
	// wantDepends is what the package needs installed.
	wantDepends = "systemd"
)

// The data archive's rules and its files.
const (
	// usrDir is the 1 directory that an install writes.
	usrDir = "usr"
	// binaryPerm is the permission of a binary.
	binaryPerm = 0o755
	// sharedWrite is the group and world write bits: no member carries 1.
	sharedWrite = 0o022
	// appJSONPath is the app's lodge manifest, whose version the check reads.
	appJSONPath = "usr/share/lodge/apps/marks/app.json"
)

// binaryFiles are the app's 2 binaries: regular files, 0755, and not empty.
var binaryFiles = []string{"usr/bin/frostmarksd", "usr/bin/marksctl"}

// requiredFiles are the regular, nonempty files that the data archive must hold:
// the app's lodge files, its web bundle's 2 files, its 2 units and its
// sysusers.d entry.
var requiredFiles = []string{
	"usr/share/lodge/apps/marks/app.json",
	"usr/share/lodge/apps/marks/manifest.json",
	"usr/share/lodge/apps/marks/tools.json",
	"usr/share/lodge/apps/marks/web/index.html",
	"usr/share/lodge/apps/marks/web/icon.svg",
	"usr/lib/systemd/system/frostmarks.service",
	"usr/lib/systemd/system/frostmarks.socket",
	"usr/lib/sysusers.d/frostmarks.conf",
}

func main() {
	deb := flag.String("deb", "", "the .deb file to check")
	version := flag.String("version", "", "the version that the control file and app.json must name")
	flag.Parse()
	if *deb == "" || *version == "" {
		fmt.Fprintln(os.Stderr, "debcheck: -deb FILE and -version V are required")
		flag.Usage()
		os.Exit(2)
	}
	pkg, err := readDeb(*deb)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debcheck: %v\n", err)
		os.Exit(1)
	}
	problems := pkg.problems(*version)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "debcheck: %s\n", problem)
		}
		os.Exit(1)
	}
	for _, line := range pkg.listing() {
		fmt.Println(line)
	}
}

// member is 1 member of an ar archive.
type member struct {
	// name is the member's name, without ar's trailing "/".
	name string
	// data is the member's bytes.
	data []byte
}

// pkg is 1 read .deb: its control fields and its 2 archives' members.
type pkg struct {
	// control maps each field of the control file to its value.
	control map[string]string
	// controlNames are the names of the control archive's members.
	controlNames []string
	// data are the members of the data archive, in the archive's order.
	data []entry
}

// entry is 1 member of the data archive.
type entry struct {
	// name is the member's name, as the archive spells it.
	path string
	// kind is the member's tar type flag.
	kind byte
	// perm is the member's permission bits.
	perm int64
	// uid and gid are the member's owner.
	uid, gid int
	// size is the member's size.
	size int64
	// data is a regular file's bytes.
	data []byte
}

// regular reports whether 1 entry is a regular file. The tar reader spells the
// legacy type flag for a regular file as TypeReg, so 1 case reads both.
func (e entry) regular() bool {
	return e.kind == tar.TypeReg
}

// dir reports whether 1 entry is a directory.
func (e entry) dir() bool {
	return e.kind == tar.TypeDir
}

// normalized returns 1 entry's name without the prefix a tar writer may add.
func (e entry) normalized() string {
	return strings.TrimPrefix(e.path, "./")
}

// readDeb reads 1 .deb: its ar members, the fields of its control file and the
// members of its data archive.
func readDeb(name string) (*pkg, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	members, err := readAr(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	control, data, err := archives(members)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	fields, names, err := readControl(control)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	entries, err := readData(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &pkg{control: fields, controlNames: names, data: entries}, nil
}

// readAr reads the members of 1 ar archive.
func readAr(r io.Reader) ([]member, error) {
	head := make([]byte, len(arMagic))
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, fmt.Errorf("read the ar header: %w", err)
	}
	if string(head) != arMagic {
		return nil, fmt.Errorf("the file starts %q, and an ar archive starts %q", head, arMagic)
	}
	var out []member
	header := make([]byte, arHeaderLen)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			if err == io.EOF {
				return out, nil
			}
			return nil, fmt.Errorf("read the header of member %d: %w", len(out)+1, err)
		}
		if string(header[58:60]) != arMagicEnd {
			return nil, fmt.Errorf("the header of member %d does not end with %q", len(out)+1, arMagicEnd)
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("the header of member %d has the size %q", len(out)+1, strings.TrimSpace(string(header[48:58])))
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("read member %d: %w", len(out)+1, err)
		}
		if size%2 == 1 {
			// A member's data is padded to an even length.
			padding := make([]byte, 1)
			if _, err := io.ReadFull(r, padding); err != nil {
				return nil, fmt.Errorf("read the padding of member %d: %w", len(out)+1, err)
			}
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[0:16])), "/")
		if rest, ok := strings.CutPrefix(name, "#1/"); ok {
			length, err := strconv.Atoi(rest)
			if err != nil || length < 0 || length > len(data) {
				return nil, fmt.Errorf("the header of member %d has the long name %q", len(out)+1, name)
			}
			name = string(data[:length])
			data = data[length:]
		}
		out = append(out, member{name: name, data: data})
	}
}

// archives returns the control archive and the data archive of 1 .deb.
func archives(members []member) (control, data []byte, err error) {
	for _, m := range members {
		switch m.name {
		case controlArchive:
			control = m.data
		case dataArchive:
			data = m.data
		}
	}
	if control == nil {
		return nil, nil, fmt.Errorf("the package holds no %s", controlArchive)
	}
	if data == nil {
		return nil, nil, fmt.Errorf("the package holds no %s", dataArchive)
	}
	return control, data, nil
}

// readControl reads the fields and the member names of 1 control archive.
func readControl(data []byte) (map[string]string, []string, error) {
	entries, err := readTarGz(data)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", controlArchive, err)
	}
	fields := map[string]string{}
	var names []string
	for _, entry := range entries {
		name := entry.normalized()
		if name == "" || entry.dir() {
			continue
		}
		names = append(names, name)
		if name == controlFile {
			fields = parseControl(entry.data)
		}
	}
	if _, ok := fields[packageField]; !ok {
		return nil, nil, fmt.Errorf("the control archive holds no %s", controlFile)
	}
	return fields, names, nil
}

// readData reads the members of 1 data archive.
func readData(data []byte) ([]entry, error) {
	entries, err := readTarGz(data)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dataArchive, err)
	}
	return entries, nil
}

// readTarGz reads the members of 1 gzipped tar archive.
func readTarGz(data []byte) ([]entry, error) {
	unzipped, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = unzipped.Close()
	}()
	reader := tar.NewReader(unzipped)
	var out []entry
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		next := entry{
			path: header.Name,
			kind: header.Typeflag,
			perm: header.Mode & 0o777,
			uid:  header.Uid,
			gid:  header.Gid,
			size: header.Size,
		}
		if next.regular() {
			body, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}
			next.data = body
		}
		out = append(out, next)
	}
}

// parseControl reads the fields of 1 control file: 1 field per line, and the
// lines that start with a space continue the field before them.
func parseControl(data []byte) map[string]string {
	fields := map[string]string{}
	var last string
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if last != "" {
				fields[last] += "\n" + strings.TrimRight(line, " \t")
			}
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		last = strings.TrimSpace(name)
		fields[last] = strings.TrimSpace(value)
	}
	return fields
}

// problems returns each problem of 1 package for the version given.
func (p *pkg) problems(version string) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	for _, want := range []struct {
		// field is the control field.
		field string
		// value is the value it must have.
		value string
	}{
		{packageField, wantPackage},
		{versionField, version},
		{architectureField, wantArchitecture},
		{dependsField, wantDepends},
	} {
		if got := p.control[want.field]; got != want.value {
			add("the control field %s is %q, want %q", want.field, got, want.value)
		}
	}

	// The control archive holds no maintainer script: a postinst or a prerm is
	// code that runs as root on the user's host.
	for _, name := range p.controlNames {
		if name != controlFile && name != md5sumsFile {
			add("the control archive holds %s, and its only files are %s and %s", name, controlFile, md5sumsFile)
		}
	}

	files := map[string]entry{}
	for _, member := range p.data {
		name := member.normalized()
		if name == "" || path.Clean(strings.TrimSuffix(name, "/")) == "." {
			// The archive's own root, such as "./": 1 entry, and no file.
			continue
		}
		if strings.HasPrefix(name, "/") {
			add("the data archive holds the absolute name /%s", strings.TrimPrefix(name, "/"))
			continue
		}
		for _, segment := range strings.Split(name, "/") {
			if segment == ".." {
				add("the data archive holds the name %s, which climbs out of its directory", name)
			}
		}
		cleaned := path.Clean(strings.TrimSuffix(name, "/"))
		if cleaned != usrDir && !strings.HasPrefix(cleaned, usrDir+"/") {
			add("the data archive holds %s, which is outside /%s", name, usrDir)
		}
		if !member.regular() && !member.dir() {
			add("the data archive holds %s, which is no directory and no regular file", name)
		}
		if member.uid != 0 || member.gid != 0 {
			add("%s is owned by %d:%d, want 0:0", name, member.uid, member.gid)
		}
		if member.perm&sharedWrite != 0 {
			add("%s has the mode %#o, and no member is group- or world-writable", name, member.perm)
		}
		if member.regular() {
			files[cleaned] = member
		}
	}

	for _, name := range binaryFiles {
		member, ok := files[name]
		switch {
		case !ok:
			add("the data archive holds no %s", name)
		case member.perm != binaryPerm:
			add("%s has the mode %#o, want %#o", name, member.perm, binaryPerm)
		case member.size == 0:
			add("%s is empty", name)
		}
	}
	for _, name := range requiredFiles {
		member, ok := files[name]
		if !ok {
			add("the data archive holds no %s", name)
			continue
		}
		if member.size == 0 {
			add("%s is empty", name)
		}
	}

	// The app's manifest names the version of the build, and lodge shows it.
	if member, ok := files[appJSONPath]; ok && member.size > 0 {
		var app struct {
			// Version is the app's version.
			Version string `json:"version"`
		}
		if err := json.Unmarshal(member.data, &app); err != nil {
			add("%s does not parse: %v", appJSONPath, err)
		} else if app.Version != version {
			add("%s names the version %q, want %q", appJSONPath, app.Version, version)
		}
	}
	return problems
}

// listing returns 1 line for each member of the data archive, in the archive's
// order.
func (p *pkg) listing() []string {
	out := make([]string, 0, len(p.data)+1)
	out = append(out, fmt.Sprintf("%s %s %d members", p.control[packageField], p.control[versionField], len(p.data)))
	for _, member := range p.data {
		kind := "-"
		switch {
		case member.dir():
			kind = "d"
		case member.regular():
			kind = "f"
		}
		out = append(out, fmt.Sprintf("%s %04o %d %s", kind, member.perm, member.size, member.normalized()))
	}
	return out
}
