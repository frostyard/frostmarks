package netscape

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The limits of a bookmark, which mirror the limits of the app's API
// (docs/specs/api.md).
const (
	// maxURLLen is the longest normalized URL, in bytes.
	maxURLLen = 8192
	// maxTitleLen is the longest title, in bytes.
	maxTitleLen = 2048
	// maxNotesLen is the longest notes, in bytes.
	maxNotesLen = 65536
	// maxTags is how many tags 1 bookmark holds.
	maxTags = 64
	// maxTagRunes is how long 1 tag is, in characters.
	maxTagRunes = 64
	// untitled is the name of a folder whose name is empty, "." or "..".
	untitled = "Untitled"
	// maxAddDate is the latest ADD_DATE that the parser reads, in Unix seconds:
	// the end of the year 9999.
	maxAddDate = 253402300799
)

// normalizedURL returns the canonical form of 1 URL, as the app's API gives it
// (rule 2 of docs/specs/api.md): an http or https URL with a lower-case scheme
// and host, no fragment, and at most 8192 bytes. The errors have the texts of
// the API's errors.
//
// It mirrors normalizedURL of the marks package, which is private to the daemon,
// and which the import must not import.
func normalizedURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("the URL %q is no URL: %s", value, err.Error())
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("the URL %q must start with http:// or https://", value)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("the URL %q has no host", value)
	}
	parsed.Scheme = scheme
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.RawFragment = ""
	normalized := parsed.String()
	if len(normalized) > maxURLLen {
		return "", fmt.Errorf("the URL is %d bytes, and the limit is %d", len(normalized), maxURLLen)
	}
	return normalized, nil
}

// isBlank reports whether a character is white space or a control character.
func isBlank(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}

// oneLine returns 1 text on 1 line: each run of white space and control
// characters is 1 space, and the ends are trimmed.
func oneLine(text string) string {
	return strings.Join(strings.FieldsFunc(text, isBlank), " ")
}

// cut returns a text of at most limit bytes, cut at a character boundary. The
// text is valid UTF-8.
func cut(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

// folderName returns the name of 1 folder from the text of its <H3>: on 1 line,
// each "/" a "-", "Untitled" for a name that is empty, "." or "..", and at most
// MaxNameLen bytes.
func folderName(text string) string {
	name := strings.ReplaceAll(oneLine(text), "/", "-")
	switch name {
	case "", ".", "..":
		return untitled
	}
	return cut(name, MaxNameLen)
}

// bookmarkTitle returns the title of 1 link from its text: on 1 line, and at
// most 2048 bytes.
func bookmarkTitle(text string) string {
	return cut(oneLine(text), maxTitleLen)
}

// bookmarkNotes returns the notes of 1 link from the text of its description:
// trimmed, and at most 65536 bytes.
func bookmarkNotes(text string) string {
	return cut(strings.TrimSpace(text), maxNotesLen)
}

// bookmarkTags returns the tags of 1 link from its TAGS attribute: the value is
// split at each comma, and each tag is trimmed, has each run of white space as
// "-" and no control character, is lower-cased and cut to 64 characters. Empty
// tags and repeats are dropped, and the first 64 stay.
func bookmarkTags(value string) []string {
	var tags []string
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(value, ",") {
		tag := cutRunes(strings.ToLower(tagText(raw)), maxTagRunes)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
		if len(tags) == maxTags {
			break
		}
	}
	return tags
}

// tagText returns 1 tag as 1 token: trimmed of white space and control characters, with each run of white space as
// "-", and with no control character.
func tagText(raw string) string {
	var out strings.Builder
	space := false
	for _, r := range strings.TrimFunc(raw, isBlank) {
		switch {
		case unicode.IsSpace(r):
			if !space {
				out.WriteByte('-')
			}
			space = true
			continue
		case unicode.IsControl(r):
			continue
		}
		space = false
		out.WriteRune(r)
	}
	return out.String()
}

// cutRunes returns a text of at most limit characters.
func cutRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	count := 0
	for i := range text {
		if count == limit {
			return text[:i]
		}
		count++
	}
	return text
}

// addDate returns the time of 1 ADD_DATE value, in Unix seconds, and the zero
// time for a value that is not a date.
func addDate(value string) time.Time {
	seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || seconds <= 0 || seconds > maxAddDate {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}
