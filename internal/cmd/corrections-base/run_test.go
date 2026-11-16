package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tests of run make their own git repositories: 1 base commit, and the
// change in the working tree, as a pull request's base and head have them. The
// commits carry a fixed author, committer and date, so that the repositories
// are the same on every run.

// gitDate is the date of every commit of the tests, in git's environment form.
const gitDate = "2026-11-16T00:00:00+00:00"

// testRepo is 1 temporary git repository of the tests.
type testRepo struct {
	t   *testing.T
	dir string
}

// newTestRepo makes 1 empty git repository with a fixed identity and date.
func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH, so run cannot be tested")
	}
	repo := &testRepo{t: t, dir: t.TempDir()}
	repo.git("init", "-q", ".")
	return repo
}

// git runs 1 git command in the repository and returns its trimmed output.
func (repo *testRepo) git(args ...string) string {
	repo.t.Helper()
	full := append([]string{
		"-c", "user.name=frostmarks",
		"-c", "user.email=frostmarks@example.com",
		"-c", "commit.gpgsign=false",
	}, args...)
	command := exec.Command("git", full...)
	command.Dir = repo.dir
	// A git of the test's own: no global or system configuration.
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	out, err := command.CombinedOutput()
	if err != nil {
		repo.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// write writes 1 file of the repository, with its parent directories.
func (repo *testRepo) write(path, text string) {
	repo.t.Helper()
	full := filepath.Join(repo.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		repo.t.Fatalf("make the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
		repo.t.Fatalf("write %s: %v", path, err)
	}
}

// remove deletes 1 file of the repository.
func (repo *testRepo) remove(path string) {
	repo.t.Helper()
	if err := os.Remove(filepath.Join(repo.dir, filepath.FromSlash(path))); err != nil {
		repo.t.Fatalf("remove %s: %v", path, err)
	}
}

// rename moves 1 file of the repository.
func (repo *testRepo) rename(from, to string) {
	repo.t.Helper()
	if err := os.Rename(filepath.Join(repo.dir, filepath.FromSlash(from)), filepath.Join(repo.dir, filepath.FromSlash(to))); err != nil {
		repo.t.Fatalf("rename %s to %s: %v", from, to, err)
	}
}

// commit stages every change of the repository and commits it, and returns the
// commit id.
func (repo *testRepo) commit(message string) string {
	repo.t.Helper()
	repo.git("add", "-A")
	repo.git("commit", "-q", "-m", message)
	return repo.git("rev-parse", "HEAD")
}

// stage stages every change of the repository, so that a diff against a commit
// holds it.
func (repo *testRepo) stage() {
	repo.t.Helper()
	repo.git("add", "-A")
}

// check calls run in the repository, and returns its standard output, its
// standard error and its error.
func (repo *testRepo) check(base string) (string, string, error) {
	repo.t.Helper()
	repo.t.Chdir(repo.dir)
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), base, correctionsDir, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

// recordFile is 1 record file of the tests: a valid record whose correction and
// promoted_to the cases change.
func recordFile(correction, promoted string) string {
	return `{
  "date": "2026-11-16",
  "scope": "ci",
  "correction": "` + correction + `",
  "evidence": "e",
  "promoted_to": "` + promoted + `"
}
`
}

// anotherRecordFile is a second valid record file, so that a case can tell a
// deletion from a change.
const anotherRecordFile = `{
  "date": "2026-11-16",
  "scope": "gate",
  "correction": "Keep the gate 1 command.",
  "evidence": "e",
  "promoted_to": ""
}
`

// recordPath is 1 record file's path in the repository, and inboxKeep keeps the
// corrections folder of a base commit that has no record.
const (
	recordPath = ".memory/corrections/2026-11-16-pr-12-a-record.json"
	inboxKeep  = ".memory/corrections/.gitkeep"
)

// TestRun checks run against real git repositories: a deleted record, a renamed
// 1, a changed member, a permitted filled promoted_to, a base with no
// corrections folder, a base ref that does not exist, and a git that fails
// (frostyard/core ADR-0081, row B2 of its repository baseline).
func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		// prepare makes the base commit and the change, and returns the base
		// ref that run compares with.
		prepare func(t *testing.T, repo *testRepo) string
		// wantError is a substring of the error; empty means that run passes.
		wantError string
		// wantStderr is a substring of the standard error, which holds the
		// problem of each record that the inbox refuses.
		wantStderr string
		// wantOutput is a substring of the standard output.
		wantOutput string
		// gitStub replaces git on PATH with 1 that fails, when it is true.
		gitStub bool
	}{{
		name: "a record added since the base",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			base := repo.commit("the base")
			repo.write(recordPath, recordFile("c", ""))
			repo.stage()
			return base
		},
		wantOutput: "1 record file(s) checked",
	}, {
		name: "a deleted record",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			base := repo.commit("the base")
			repo.remove(recordPath)
			return base
		},
		wantError:  "1 problem(s) with the corrections inbox",
		wantStderr: recordPath + " was deleted",
	}, {
		name: "a renamed record",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			base := repo.commit("the base")
			repo.rename(recordPath, ".memory/corrections/2026-11-16-pr-12-b-record.json")
			repo.stage()
			return base
		},
		wantError:  "1 problem(s) with the corrections inbox",
		wantStderr: "was renamed to",
	}, {
		name: "a changed correction",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			base := repo.commit("the base")
			repo.write(recordPath, recordFile("other", ""))
			return base
		},
		wantError:  "1 problem(s) with the corrections inbox",
		wantStderr: "correction changed",
	}, {
		name: "a filled empty promoted_to",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			base := repo.commit("the base")
			repo.write(recordPath, recordFile("c", "a_test.go"))
			return base
		},
		wantOutput: "1 record file(s) checked",
	}, {
		name: "a second record added beside a changed 1",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			base := repo.commit("the base")
			repo.write(recordPath, recordFile("c", "a_test.go"))
			repo.write(".memory/corrections/2026-11-16-pr-12-b-record.json", anotherRecordFile)
			repo.stage()
			return base
		},
		wantOutput: "2 record file(s) checked",
	}, {
		name: "no corrections folder at the base",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write("README.md", "a repository\n")
			base := repo.commit("the base")
			repo.write(recordPath, recordFile("c", ""))
			repo.stage()
			return base
		},
		wantOutput: "nothing to compare",
	}, {
		name: "a base that is no ref",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			repo.commit("the base")
			return "no-such-ref"
		},
		wantError: "no-such-ref",
	}, {
		name: "a git that fails",
		prepare: func(t *testing.T, repo *testRepo) string {
			t.Helper()
			repo.write(inboxKeep, "")
			repo.write(recordPath, recordFile("c", ""))
			return repo.commit("the base")
		},
		gitStub:   true,
		wantError: "git ls-tree",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_AUTHOR_DATE", gitDate)
			t.Setenv("GIT_COMMITTER_DATE", gitDate)
			repo := newTestRepo(t)
			base := tc.prepare(t, repo)
			if tc.gitStub {
				t.Setenv("PATH", stubGit(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
			}
			stdout, stderr, err := repo.check(base)
			switch {
			case tc.wantError == "" && err != nil:
				t.Fatalf("run = %v, want no error\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			case tc.wantError != "" && err == nil:
				t.Fatalf("run = nil, want an error with %q\nstdout:\n%s", tc.wantError, stdout)
			case tc.wantError != "" && !strings.Contains(err.Error(), tc.wantError):
				t.Errorf("run = %v, want an error with %q", err, tc.wantError)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("the standard error = %q, want it to hold %q", stderr, tc.wantStderr)
			}
			if tc.wantOutput != "" && !strings.Contains(stdout, tc.wantOutput) {
				t.Errorf("the standard output = %q, want it to hold %q", stdout, tc.wantOutput)
			}
		})
	}
}

// stubGit writes 1 directory that holds a git that fails, and returns it, so
// that the test reads a git failure as run reads it.
func stubGit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n# A git that fails, for the test of run.\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the stub git: %v", err)
	}
	return dir
}
