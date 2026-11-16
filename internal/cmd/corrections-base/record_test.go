package main

import (
	"strings"
	"testing"
)

// surrogateRecord returns the text of 1 record file whose correction holds the
// JSON escape escape, and whose promoted_to is promoted ("" or a path). The
// escape stays an escape in the file: the canonical form of a record keeps a
// lone surrogate escaped, and no Go string holds 1.
func surrogateRecord(escape, promoted string) []byte {
	return []byte(`{
  "date": "2026-11-16",
  "scope": "ci",
  "correction": "Pin ` + escape + ` the key",
  "evidence": "e",
  "promoted_to": "` + promoted + `"
}
`)
}

// TestReadRecordKeepsEscapes checks that readRecord returns the bytes of each
// member as the file has them: an escaped lone surrogate stays escaped, and it
// does not become U+FFFD, so 2 records are compared as their files have them.
func TestReadRecordKeepsEscapes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		escape string
	}{{
		name:   "a lone high surrogate",
		escape: `\ud800`,
	}, {
		name:   "a lone low surrogate",
		escape: `\udc00`,
	}} {
		text := surrogateRecord(tc.escape, "")
		members, err := readRecord(text)
		if err != nil {
			t.Fatalf("%s: readRecord: %v", tc.name, err)
		}
		want := `"Pin ` + tc.escape + ` the key"`
		if got := string(members["correction"]); got != want {
			t.Errorf("%s: the correction's raw text = %q, want %q", tc.name, got, want)
		}
		if strings.ContainsRune(string(members["correction"]), '\uFFFD') {
			t.Errorf("%s: the correction's raw text holds U+FFFD, and the escape must stay as it is: %q", tc.name, members["correction"])
		}
	}
}

// TestRecordProblemsRawMembers checks the 2 changes of a record that holds an
// escaped lone surrogate: a filled empty promoted_to is permitted, a changed
// member is not, and 2 records that differ only in the surrogate do not compare
// equal (frostyard/core ADR-0081, row B2).
func TestRecordProblemsRawMembers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before []byte
		after  []byte
		want   string // a substring of the 1 problem; empty for no problem
	}{
		{
			name:   "a filled promoted_to beside a lone high surrogate",
			before: surrogateRecord(`\ud800`, ""),
			after:  surrogateRecord(`\ud800`, "a_test.go"),
		},
		{
			name:   "a filled promoted_to beside a lone low surrogate",
			before: surrogateRecord(`\udc00`, ""),
			after:  surrogateRecord(`\udc00`, "a_test.go"),
		},
		{
			name:   "no change beside a lone high surrogate",
			before: surrogateRecord(`\ud800`, ""),
			after:  surrogateRecord(`\ud800`, ""),
		},
		{
			name:   "the correction changed",
			before: surrogateRecord(`\ud800`, ""),
			after: []byte(`{
  "date": "2026-11-16",
  "scope": "ci",
  "correction": "Pin the key",
  "evidence": "e",
  "promoted_to": ""
}
`),
			want: "correction changed",
		},
		{
			name:   "the surrogate itself changed",
			before: surrogateRecord(`\ud800`, ""),
			after:  surrogateRecord(`\udc00`, ""),
			want:   "correction changed",
		},
		{
			name:   "the surrogate's case changed",
			before: surrogateRecord(`\ud800`, ""),
			after:  surrogateRecord(`\uD800`, ""),
			want:   "correction changed",
		},
	} {
		problems := recordProblems("a.json", tc.before, tc.after)
		switch {
		case tc.want == "" && len(problems) > 0:
			t.Errorf("%s: recordProblems = %v, want none", tc.name, problems)
		case tc.want != "" && !strings.Contains(strings.Join(problems, "\n"), tc.want):
			t.Errorf("%s: recordProblems = %v, want a problem with %q", tc.name, problems, tc.want)
		}
	}
}
