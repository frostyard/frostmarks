// Package workflowcheck holds the tests that keep the repository's own files
// in line with frostyard/core: the GitHub Actions workflows within ADR-0021
// (external actions pinned to a full commit SHA, permissions declared at the
// top, and checkouts that do not keep the token), the files of the repository
// baseline (ADR-0071, rows B1, B2, B6 and B13), the buf workspace of
// frostyard/core ADR-0085 (no remote plugin, and every module dependency
// pinned by digest), and the permission hooks of marks/, which must stay a
// risk path at L1 or L2 (ADR-0068).
package workflowcheck
