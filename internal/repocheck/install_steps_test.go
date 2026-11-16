package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installDocs are the docs that give the administrator's steps after the
// install of the .deb (docs/specs/lodge-app.md, The units).
var installDocs = []string{
	"README.md",
	"docs/specs/lodge-app.md",
	"docs/adr/0007-run-on-lodge-from-a-hand-installed-deb.md",
	"site/content/guides/lodge.md",
}

// installSteps are the administrator's steps after the install, in their order.
// The package runs no maintainer script, so no step happens by itself: the
// socket unit names SocketUser=frostmarks, and systemd refuses to start it
// before systemd-sysusers makes that user.
var installSteps = []string{
	"systemd-sysusers",
	"systemctl daemon-reload",
	"systemctl enable --now frostmarks.socket",
	"lodgectl reload",
}

// installStepProblems returns the steps that text leaves out, or gives out of
// order. A line break in prose is a space, so the text is read with its
// whitespace collapsed.
func installStepProblems(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	var problems []string
	from := 0
	for _, step := range installSteps {
		at := strings.Index(text[from:], step)
		if at < 0 {
			if strings.Contains(text, step) {
				problems = append(problems, "the step "+step+" comes before the step that it must follow")
			} else {
				problems = append(problems, "the step "+step+" is missing")
			}
			continue
		}
		from += at + len(step)
	}
	return problems
}

// TestInstallSteps checks that every doc that tells the administrator how to
// start the package gives all the steps, in order.
func TestInstallSteps(t *testing.T) {
	for _, name := range installDocs {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range installStepProblems(string(data)) {
			t.Errorf("%s: %s", name, problem)
		}
	}
}

// TestInstallStepRules checks that the rule refuses a missing step and a step
// out of order.
func TestInstallStepRules(t *testing.T) {
	complete := "systemd-sysusers\nsystemctl daemon-reload\nsystemctl enable --now frostmarks.socket\nlodgectl reload\n"
	for _, tc := range []struct {
		name, text string
		problems   int
	}{
		{"complete", complete, 0},
		{"no sysusers", strings.Replace(complete, "systemd-sysusers\n", "", 1), 1},
		{"no daemon-reload", strings.Replace(complete, "systemctl daemon-reload\n", "", 1), 1},
		{"reload first", "lodgectl reload\nsystemd-sysusers\nsystemctl daemon-reload\nsystemctl enable --now frostmarks.socket\n", 1},
		{"enable first", "systemctl enable --now frostmarks.socket\nsystemd-sysusers\nsystemctl daemon-reload\nlodgectl reload\n", 1},
		{"wrapped", "systemd-sysusers, systemctl\ndaemon-reload, systemctl enable --now\nfrostmarks.socket and lodgectl reload", 0},
		{"empty", "", 4},
	} {
		if got := installStepProblems(tc.text); len(got) != tc.problems {
			t.Errorf("%s: %d problems %q, want %d", tc.name, len(got), got, tc.problems)
		}
	}
}
