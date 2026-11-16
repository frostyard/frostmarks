package marks

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/hearth/authz"
)

// The app's limits, in bytes unless a rule says runes.
const (
	// maxFolderLen is the longest folder path, in bytes.
	maxFolderLen = 1024
	// maxURLLen is the longest normalized URL, in bytes.
	maxURLLen = 8192
	// maxTitleLen is the longest title, in bytes.
	maxTitleLen = 2048
	// maxNotesLen is the longest notes, in bytes.
	maxNotesLen = 65536
	// maxTags is how many tags 1 bookmark holds.
	maxTags = 64
	// maxTagRunes is how long 1 tag is, in runes.
	maxTagRunes = 64
)

// folderOf returns the canonical form of 1 request's folder value: the root
// path for an absent value, and invalid_argument for a value that is no folder
// path (see authz.CleanPath) or is longer than maxFolderLen.
func folderOf(value string) (string, error) {
	path := value
	if path == "" {
		path = authz.Root
	}
	if len(path) > maxFolderLen {
		return "", invalidArgument("the folder is %d bytes, and the limit is %d", len(path), maxFolderLen)
	}
	cleaned, err := authz.CleanPath(path)
	if err != nil {
		return "", invalidArgument("%s", err.Error())
	}
	return cleaned, nil
}

// parentOf returns the parent of 1 canonical path: the root for a path of 1
// segment, and the path without its last segment for a longer 1.
func parentOf(path string) string {
	if path == authz.Root {
		return authz.Root
	}
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return authz.Root
}

// isWithin reports whether 1 canonical path is the other path, or below it.
func isWithin(path, folder string) bool {
	if folder == authz.Root {
		return true
	}
	return path == folder || strings.HasPrefix(path, folder+"/")
}

// isBelow reports whether 1 canonical path is below the other path.
func isBelow(path, folder string) bool {
	return path != folder && isWithin(path, folder)
}

// normalizedURL returns the canonical form of 1 URL: an http or https URL with
// a lower-case scheme and host and no fragment, and the path and the query as
// they were given.
//
// A URL of another scheme is refused: a saved javascript: URL is a script in a
// later web UI.
func normalizedURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", invalidArgument("the URL %q is no URL: %s", value, err.Error())
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", invalidArgument("the URL %q must start with http:// or https://", value)
	}
	if parsed.Host == "" {
		return "", invalidArgument("the URL %q has no host", value)
	}
	parsed.Scheme = scheme
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.RawFragment = ""
	normalized := parsed.String()
	if len(normalized) > maxURLLen {
		return "", invalidArgument("the URL is %d bytes, and the limit is %d", len(normalized), maxURLLen)
	}
	return normalized, nil
}

// checkText checks 1 text of a request: it is valid UTF-8, and it is at most
// limit bytes long.
func checkText(name, value string, limit int) error {
	if !utf8.ValidString(value) {
		return invalidArgument("the %s is not valid UTF-8", name)
	}
	if len(value) > limit {
		return invalidArgument("the %s is %d bytes, and the limit is %d", name, len(value), limit)
	}
	return nil
}

// normalizedTag returns the canonical form of 1 tag: trimmed and lower-cased.
//
// A tag is 1 to maxTagRunes characters (runes), with no white space, no control
// character and no comma, so that a tag is 1 token of a list and 1 word of the
// search.
func normalizedTag(tag string) (string, error) {
	trimmed := strings.TrimSpace(tag)
	if !utf8.ValidString(trimmed) {
		return "", invalidTag(tag, "it is not valid UTF-8")
	}
	lowered := strings.ToLower(trimmed)
	switch {
	case lowered == "":
		return "", invalidTag(tag, "it is empty")
	case utf8.RuneCountInString(lowered) > maxTagRunes:
		return "", invalidTag(tag, "it is longer than "+strconv.Itoa(maxTagRunes)+" characters")
	}
	for _, r := range lowered {
		switch {
		case r == ',':
			return "", invalidTag(tag, "it holds a comma")
		case unicode.IsSpace(r):
			return "", invalidTag(tag, "it holds white space")
		case unicode.IsControl(r):
			return "", invalidTag(tag, "it holds a control character")
		}
	}
	return lowered, nil
}

// invalidTag returns the error of 1 refused tag, with the tag as it arrived.
func invalidTag(tag, reason string) error {
	return invalidArgument("invalid tag %q: %s", tag, reason)
}

// normalizedTags returns the canonical form of a bookmark's whole tag list:
// each tag canonical, sorted, and without a repeat.
func normalizedTags(tags []string) ([]string, error) {
	if len(tags) > maxTags {
		return nil, invalidArgument("the bookmark would hold %d tags, and the limit is %d", len(tags), maxTags)
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized, err := normalizedTag(tag)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// normalizeEach returns the canonical form of each tag of a list, in the
// order the caller gave, without a repeat: the set of tags that a change adds
// or removes.
func normalizeEach(tags []string) ([]string, error) {
	if len(tags) > maxTags {
		return nil, invalidArgument("the call names %d tags, and the limit is %d", len(tags), maxTags)
	}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized, err := normalizedTag(tag)
		if err != nil {
			return nil, err
		}
		if slices.Contains(out, normalized) {
			continue
		}
		out = append(out, normalized)
	}
	return out, nil
}

// mergedTags returns the tags of 1 bookmark after a change: the stored tags,
// with the added 1s and without the removed 1s, sorted and without a repeat.
func mergedTags(stored, add, remove []string) []string {
	set := make(map[string]struct{}, len(stored)+len(add))
	for _, tag := range stored {
		set[tag] = struct{}{}
	}
	for _, tag := range add {
		set[tag] = struct{}{}
	}
	for _, tag := range remove {
		delete(set, tag)
	}
	out := make([]string, 0, len(set))
	for tag := range set {
		out = append(out, tag)
	}
	slices.Sort(out)
	return out
}
