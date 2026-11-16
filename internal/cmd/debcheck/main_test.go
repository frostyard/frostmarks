package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// version is the version that the test packages name.
const version = "0.1.0"

// The names and the bodies of a good package.
const (
	// binaryBody is the body of each binary of a good package.
	binaryBody = "bin"
	// indexBody is the body of the app's page.
	indexBody = "<!doctype html><title>Bookmarks</title>"
	// iconBody is the body of the app's icon.
	iconBody = `<svg xmlns="http://www.w3.org/2000/svg"><title>Bookmarks</title></svg>`
	// unitBody is the body of each unit of a good package.
	unitBody = "[Unit]\nDescription=frostmarks\n"
	// fieldPackage and the fields beside it are the control file's fields.
	fieldPackage      = "Package"
	fieldVersion      = "Version"
	fieldArchitecture = "Architecture"
	fieldDepends      = "Depends"
)

// testMember is 1 member of a test's archive.
type testMember struct {
	// name is the member's name.
	name string
	// kind is the member's tar type flag.
	kind byte
	// perm is the member's permission bits.
	perm int64
	// uid and gid are the member's owner.
	uid, gid int
	// body is a regular file's bytes.
	body []byte
}

// archive is 1 test .deb: the fields of its control file, the extra members of
// its control archive, and its data members.
type archive struct {
	// fields are the control file's fields, in the order given.
	fields [][2]string
	// controlExtra are the control archive's members beside control and
	// md5sums.
	controlExtra []testMember
	// data are the data archive's members.
	data []testMember
}

// good returns 1 package that the check passes.
func good() archive {
	app, err := json.Marshal(map[string]any{
		"contract": 1,
		"id":       "marks",
		"version":  version,
	})
	if err != nil {
		panic(err)
	}
	data := []testMember{
		{name: "./", kind: tar.TypeDir, perm: 0o755},
		{name: "usr/", kind: tar.TypeDir, perm: 0o755},
		{name: "usr/bin/", kind: tar.TypeDir, perm: 0o755},
		{name: "usr/bin/frostmarksd", kind: tar.TypeReg, perm: 0o755, body: []byte(binaryBody)},
		{name: "usr/bin/marksctl", kind: tar.TypeReg, perm: 0o755, body: []byte(binaryBody)},
		{name: "usr/share/lodge/apps/marks/app.json", kind: tar.TypeReg, perm: 0o644, body: app},
		{name: "usr/share/lodge/apps/marks/manifest.json", kind: tar.TypeReg, perm: 0o644, body: []byte("{}")},
		{name: "usr/share/lodge/apps/marks/tools.json", kind: tar.TypeReg, perm: 0o644, body: []byte(`{"tools":[]}`)},
		{name: "usr/share/lodge/apps/marks/web/index.html", kind: tar.TypeReg, perm: 0o644, body: []byte(indexBody)},
		{name: "usr/share/lodge/apps/marks/web/icon.svg", kind: tar.TypeReg, perm: 0o644, body: []byte(iconBody)},
		{name: "usr/lib/systemd/system/frostmarks.service", kind: tar.TypeReg, perm: 0o644, body: []byte(unitBody)},
		{name: "usr/lib/systemd/system/frostmarks.socket", kind: tar.TypeReg, perm: 0o644, body: []byte(unitBody)},
		{name: "usr/lib/sysusers.d/frostmarks.conf", kind: tar.TypeReg, perm: 0o644, body: []byte("g lodge -\n")},
	}
	return archive{
		fields: [][2]string{
			{fieldPackage, wantPackage},
			{fieldVersion, version},
			{fieldArchitecture, wantArchitecture},
			{fieldDepends, wantDepends},
		},
		data: data,
	}
}

// controlFile returns the control file's text of 1 archive.
func (a archive) controlFile() []byte {
	var out strings.Builder
	for _, field := range a.fields {
		fmt.Fprintf(&out, "%s: %s\n", field[0], field[1])
	}
	return []byte(out.String())
}

// tarball returns 1 gzipped tar archive of the members given.
func tarball(t *testing.T, members []testMember) []byte {
	t.Helper()
	var raw bytes.Buffer
	uncompressed := gzip.NewWriter(&raw)
	writer := tar.NewWriter(uncompressed)
	for _, m := range members {
		header := &tar.Header{
			Name:     m.name,
			Typeflag: m.kind,
			Mode:     m.perm,
			Uid:      m.uid,
			Gid:      m.gid,
			Size:     int64(len(m.body)),
		}
		if m.kind == tar.TypeDir {
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write the header of %s: %v", m.name, err)
		}
		if header.Size > 0 {
			if _, err := writer.Write(m.body); err != nil {
				t.Fatalf("write %s: %v", m.name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close the data archive: %v", err)
	}
	if err := uncompressed.Close(); err != nil {
		t.Fatalf("close the compressor: %v", err)
	}
	return raw.Bytes()
}

// arArchive returns 1 ar archive of the members given, as the ar format writes
// them: 1 header of 60 bytes, the data, and 1 padding byte for odd data.
func arArchive(t *testing.T, members []testMember) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString(arMagic)
	for _, m := range members {
		header := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10s%s",
			m.name, "0", "0", "0", "100644", fmt.Sprint(len(m.body)), arMagicEnd)
		if len(header) != arHeaderLen {
			t.Fatalf("the header of %s is %d bytes, want %d", m.name, len(header), arHeaderLen)
		}
		out.WriteString(header)
		out.Write(m.body)
		if len(m.body)%2 == 1 {
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}

// write writes 1 test .deb to a temporary file, and it returns its path.
func write(t *testing.T, a archive) string {
	t.Helper()
	control := []testMember{
		{name: "./control", kind: tar.TypeReg, perm: 0o644, body: a.controlFile()},
		{name: "./md5sums", kind: tar.TypeReg, perm: 0o644, body: []byte("00000000000000000000000000000000  usr/bin/frostmarksd\n")},
	}
	control = append(control, a.controlExtra...)
	members := []testMember{
		{name: "debian-binary", body: []byte("2.0\n")},
		{name: controlArchive, body: tarball(t, control)},
		{name: dataArchive, body: tarball(t, a.data)},
	}
	name := filepath.Join(t.TempDir(), "frostyard-frostmarks_"+version+"_amd64.deb")
	if err := os.WriteFile(name, arArchive(t, members), 0o644); err != nil {
		t.Fatalf("write the package: %v", err)
	}
	return name
}

// TestCheckGood checks the package that a good build writes: no problem, and 1
// line for each member.
func TestCheckGood(t *testing.T) {
	pkg, err := readDeb(write(t, good()))
	if err != nil {
		t.Fatalf("readDeb: %v", err)
	}
	if problems := pkg.problems(version); len(problems) > 0 {
		t.Fatalf("the good package has the problems %v", problems)
	}
	listing := pkg.listing()
	if len(listing) != len(good().data)+1 {
		t.Errorf("the listing holds %d lines, want %d", len(listing), len(good().data)+1)
	}
	if !strings.HasPrefix(listing[0], wantPackage+" "+version) {
		t.Errorf("the listing starts %q, want the package's name and version", listing[0])
	}
}

// TestCheckProblems checks each problem that the check reports: 1 package for
// each rule, with the problem's words.
func TestCheckProblems(t *testing.T) {
	for _, test := range []struct {
		// name names the case.
		name string
		// change makes the package that the case checks.
		change func(*archive)
		// want is a part of the 1 problem.
		want string
	}{
		{
			name:   "another package name",
			change: func(a *archive) { a.fields[0][1] = "frostyard-notes" },
			want:   "the control field Package",
		},
		{
			name:   "another version",
			change: func(a *archive) { a.fields[1][1] = "9.9.9" },
			want:   "the control field Version",
		},
		{
			name:   "another architecture",
			change: func(a *archive) { a.fields[2][1] = "arm64" },
			want:   "the control field Architecture",
		},
		{
			name:   "another dependency",
			change: func(a *archive) { a.fields[3][1] = "libc6" },
			want:   "the control field Depends",
		},
		{
			name: "a maintainer script",
			change: func(a *archive) {
				a.controlExtra = []testMember{{name: "./postinst", kind: tar.TypeReg, perm: 0o755, body: []byte("#!/bin/sh\n")}}
			},
			want: "the control archive holds postinst",
		},
		{
			name: "a link",
			change: func(a *archive) {
				a.data = append(a.data, testMember{name: "usr/bin/marksctl", kind: tar.TypeSymlink, perm: 0o777, body: nil})
			},
			want: "which is no directory and no regular file",
		},
		{
			name: "a file outside usr",
			change: func(a *archive) {
				a.data = append(a.data, testMember{name: "etc/frostmarks.conf", kind: tar.TypeReg, perm: 0o644, body: []byte("x")})
			},
			want: "which is outside /usr",
		},
		{
			name: "a name that climbs out",
			change: func(a *archive) {
				a.data = append(a.data, testMember{name: "usr/../etc/frostmarks.conf", kind: tar.TypeReg, perm: 0o644, body: []byte("x")})
			},
			want: "which climbs out of its directory",
		},
		{
			name:   "an absolute name",
			change: func(a *archive) { a.data[3].name = "/usr/bin/frostmarksd" },
			want:   "the data archive holds the absolute name",
		},
		{
			name:   "another owner",
			change: func(a *archive) { a.data[3].uid = 1000 },
			want:   "usr/bin/frostmarksd is owned by 1000:0",
		},
		{
			name:   "another group",
			change: func(a *archive) { a.data[4].gid = 1000 },
			want:   "usr/bin/marksctl is owned by 0:1000",
		},
		{
			name:   "a group-writable file",
			change: func(a *archive) { a.data[5].perm = 0o664 },
			want:   "usr/share/lodge/apps/marks/app.json has the mode 0664",
		},
		{
			name:   "a binary that is no 0755",
			change: func(a *archive) { a.data[3].perm = 0o644 },
			want:   "usr/bin/frostmarksd has the mode 0644, want 0755",
		},
		{
			name:   "an empty binary",
			change: func(a *archive) { a.data[3].body = nil },
			want:   "usr/bin/frostmarksd is empty",
		},
		{
			name: "no icon",
			change: func(a *archive) {
				for i, m := range a.data {
					if m.name == "usr/share/lodge/apps/marks/web/icon.svg" {
						a.data = append(a.data[:i], a.data[i+1:]...)
						return
					}
				}
			},
			want: "the data archive holds no usr/share/lodge/apps/marks/web/icon.svg",
		},
		{
			name: "no unit",
			change: func(a *archive) {
				for i, m := range a.data {
					if m.name == "usr/lib/systemd/system/frostmarks.socket" {
						a.data = append(a.data[:i], a.data[i+1:]...)
						return
					}
				}
			},
			want: "the data archive holds no usr/lib/systemd/system/frostmarks.socket",
		},
		{
			name: "another version in app.json",
			change: func(a *archive) {
				body, err := json.Marshal(map[string]any{"version": "0.0.9"})
				if err != nil {
					panic(err)
				}
				a.data[5].body = body
			},
			want: "usr/share/lodge/apps/marks/app.json names the version",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := good()
			test.change(&a)
			pkg, err := readDeb(write(t, a))
			if err != nil {
				t.Fatalf("readDeb: %v", err)
			}
			problems := pkg.problems(version)
			if len(problems) == 0 {
				t.Fatalf("the package passed, and it must give the problem %q", test.want)
			}
			if !contains(problems, test.want) {
				t.Errorf("the problems %v hold no %q", problems, test.want)
			}
		})
	}
}

// TestReadDebErrors checks the archives that the check cannot read: another
// compression, no control archive, and a file that is no ar archive.
func TestReadDebErrors(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]struct {
		// name is the file's name.
		name string
		// body is the file's content.
		body []byte
		// want is a part of the error.
		want string
	}{
		"another compression": {
			name: "data.tar.xz",
			body: arArchive(t, []testMember{
				{name: "debian-binary", body: []byte("2.0\n")},
				{name: controlArchive, body: tarball(t, []testMember{{name: "./control", kind: tar.TypeReg, perm: 0o644, body: good().controlFile()}})},
				{name: "data.tar.xz", body: []byte("not an archive")},
			}),
			want: "the package holds no data.tar.gz",
		},
		"no control archive": {
			name: "no-control.deb",
			body: arArchive(t, []testMember{
				{name: "debian-binary", body: []byte("2.0\n")},
				{name: dataArchive, body: tarball(t, good().data)},
			}),
			want: "the package holds no control.tar.gz",
		},
		"no ar magic": {
			name: "plain.deb",
			body: []byte("this is no package"),
			want: "an ar archive starts",
		},
	}
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			test := cases[name]
			path := filepath.Join(dir, test.name)
			if err := os.WriteFile(path, test.body, 0o644); err != nil {
				t.Fatalf("write the package: %v", err)
			}
			if _, err := readDeb(path); err == nil {
				t.Fatalf("readDeb passed, and it must fail with %q", test.want)
			} else if !strings.Contains(err.Error(), test.want) {
				t.Errorf("the error %q holds no %q", err, test.want)
			}
		})
	}
}

// contains reports whether 1 of the problems holds 1 text.
func contains(problems []string, want string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, want) {
			return true
		}
	}
	return false
}
