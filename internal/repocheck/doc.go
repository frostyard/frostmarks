// Package repocheck holds tests that keep repository-wide rules that no
// compiler checks: the documentation rules of AGENTS.md (every relative link
// resolves, every doc is indexed), the repository boundary (no private host
// names or addresses), and frostmarks' import boundary (frostmarks imports no
// lodge, frostyard app or Pocket code, in Go or in Protobuf, and its copy of
// hearth's options file is the file of the hearth version that go.mod
// requires).
package repocheck
