// Command corrections-base checks the corrections inbox against the base of a
// pull request: no record file is deleted or renamed since then, and no record
// changes in a field other than an empty promoted_to that is now filled
// (frostyard/core ADR-0081, row B2 of its repository baseline).
//
//	make ci BASE=origin/main
//
// The command passes when the base has no corrections folder, because such a
// base has no record to keep.
package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// correctionsDir is the repository's corrections inbox.
const correctionsDir = ".memory/corrections"

func main() {
	base := flag.String("base", "", "the git reference to compare with, such as origin/main")
	flag.Parse()
	if *base == "" {
		fmt.Fprintf(os.Stderr, "%s: -base REF is required\n", os.Args[0])
		os.Exit(2)
	}
	if err := run(context.Background(), *base, correctionsDir, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "corrections-base: %v\n", err)
		os.Exit(1)
	}
}

// run checks the records of dir against base. It writes what it checked to
// stdout, and returns an error that names each problem.
func run(ctx context.Context, base, dir string, stdout, stderr io.Writer) error {
	present, err := hasDir(ctx, base, dir)
	if err != nil {
		return err
	}
	if !present {
		fmt.Fprintf(stdout, "%s has no %s: nothing to compare\n", base, dir)
		return nil
	}
	output, err := git(ctx, "diff", "--name-status", "-M", base, "--", dir)
	if err != nil {
		return err
	}
	changes := parseChanges(output)
	if len(changes) == 0 {
		fmt.Fprintf(stdout, "corrections-base: no record changed since %s\n", base)
		return nil
	}
	var problems []string
	records := 0
	for _, c := range changes {
		switch {
		case c.Status == "A":
			records++
		case len(c.Paths) == 2:
			problems = append(problems, fmt.Sprintf("%s was renamed to %s; a record file is permanent", c.Paths[0], c.Paths[1]))
		case c.Status == "D":
			problems = append(problems, c.Paths[0]+" was deleted; a record file is permanent")
		case c.Status == "M" || c.Status == "T":
			records++
			before, err := git(ctx, "show", base+":"+c.Paths[0])
			if err != nil {
				return err
			}
			after, err := os.ReadFile(c.Paths[0])
			if err != nil {
				return fmt.Errorf("read %s: %w", c.Paths[0], err)
			}
			problems = append(problems, recordProblems(c.Paths[0], []byte(before), after)...)
		default:
			problems = append(problems, c.Status+" "+strings.Join(c.Paths, " ")+" is not a change this check knows")
		}
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(stderr, "corrections-base: "+problem)
		}
		return fmt.Errorf("%d problem(s) with the corrections inbox since %s", len(problems), base)
	}
	fmt.Fprintf(stdout, "corrections-base: %d record file(s) checked since %s\n", records, base)
	return nil
}

// hasDir reports whether base has the folder dir.
func hasDir(ctx context.Context, base, dir string) (bool, error) {
	output, err := git(ctx, "ls-tree", "--name-only", base, dir)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// git runs 1 git command in the current directory and returns its standard
// output.
func git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// change is 1 line of git diff --name-status -M.
type change struct {
	// Status is the change's letter: A, M, D, T, or R### or C### with the
	// similarity score.
	Status string
	// Paths is the changed path, or the old path and the new path of a rename
	// or a copy.
	Paths []string
}

// parseChanges reads the output of git diff --name-status -M.
func parseChanges(output string) []change {
	var changes []change
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) < 2 {
			continue
		}
		changes = append(changes, change{Status: fields[0], Paths: fields[1:]})
	}
	return changes
}

// recordMembers names the 5 members of 1 correction record, in the order of its
// canonical form (frostyard/core ADR-0081, row B2).
var recordMembers = []string{"date", "scope", "correction", "evidence", "promoted_to"}

// promotedTo is the member that a pull request may fill, when the base leaves it
// empty.
const promotedTo = "promoted_to"

// recordValues holds the raw text of each member of 1 record file: the bytes of
// the member's value as the file has them, quotes, escapes and all.
type recordValues map[string]jsontext.Value

// readRecord returns the raw text of each member of 1 record's data.
//
// The text of a member is kept byte for byte (jsontext.AllowInvalidUTF8). The
// canonical form of a record may hold an escaped lone surrogate, such as
// "\ud800", which a decoder into a Go string either refuses or turns into
// U+FFFD, and 2 such records must still compare as their files have them.
func readRecord(data []byte) (recordValues, error) {
	members := make(recordValues, len(recordMembers))
	if err := json.Unmarshal(data, &members, jsontext.AllowInvalidUTF8(true)); err != nil {
		return nil, fmt.Errorf("read the record: %w", err)
	}
	return members, nil
}

// recordProblems returns each change of 1 record file that the inbox does not
// allow: a changed member, except an empty promoted_to that is now filled.
func recordProblems(path string, before, after []byte) []string {
	old, err := readRecord(before)
	if err != nil {
		return []string{fmt.Sprintf("%s: the base's record cannot be read: %v", path, err)}
	}
	next, err := readRecord(after)
	if err != nil {
		return []string{fmt.Sprintf("%s: the record cannot be read: %v", path, err)}
	}
	var problems []string
	for _, name := range recordMembers {
		if name == promotedTo {
			continue
		}
		if !bytes.Equal(old[name], next[name]) {
			problems = append(problems, fmt.Sprintf("%s: %s changed from %s to %s; a record's text is permanent",
				path, name, textOf(old[name]), textOf(next[name])))
		}
	}
	// Only an empty promoted_to may be filled. A member that the base's record
	// does not have is no empty 1.
	if !bytes.Equal(old[promotedTo], next[promotedTo]) && (string(old[promotedTo]) != `""` || string(next[promotedTo]) == `""` || len(next[promotedTo]) == 0) {
		problems = append(problems, fmt.Sprintf("%s: promoted_to changed from %s to %s; only an empty promoted_to may be filled",
			path, textOf(old[promotedTo]), textOf(next[promotedTo])))
	}
	return problems
}

// textOf returns the raw text of 1 member for a message, or a marker when the
// record has no such member.
func textOf(value jsontext.Value) string {
	if len(value) == 0 {
		return "<missing>"
	}
	return string(value)
}
