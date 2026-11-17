package netscape

import (
	"html"
	"strings"
)

// parser holds the state of 1 read of a file's body, after its declaration.
type parser struct {
	// src is the text after the declaration.
	src string
	// pos is the offset of the next byte of src to read.
	pos int
	// doc is the document that the parser builds.
	doc Document
	// levels holds the folder index of each open <DL>, outermost first: a <DL>
	// that opens no folder holds the index of the level around it, and -1 stands
	// for the target folder.
	levels []int
	// pending is the name of the folder that the last <H3> gave, which the next
	// <DL> opens.
	pending string
	// hasPending reports whether an <H3> is waiting for its <DL>.
	hasPending bool
	// folderIndex holds the index in doc.Folders of each folder, by its key.
	folderIndex map[folderKey]int
	// lastBookmark is the index in doc.Bookmarks of the last <A>, or -1 when the
	// last <A> is a refusal or there is no <A>.
	lastBookmark int
	// descriptionOK reports whether a <DD> now belongs to the last <A>: no <H3>,
	// <DL> or </DL> came after it.
	descriptionOK bool
}

// folderKey is the identity of 1 folder: its parent and its name.
type folderKey struct {
	// parent is the index of the parent folder, or -1.
	parent int
	// name is the folder's name.
	name string
}

// run reads the whole text. It returns ErrTooDeep when a <DL> opens more than
// MaxLevels levels, and ErrTooManyFolders when the file holds more than
// MaxFolders folders.
func (p *parser) run() error {
	p.lastBookmark = -1
	for {
		if len(p.levels) > MaxLevels {
			return ErrTooDeep
		}
		start := nextTag(p.src, p.pos)
		if start >= len(p.src) {
			return nil
		}
		p.pos = start
		switch {
		case strings.HasPrefix(p.src[start:], "<!--"):
			p.skipComment()
		case p.src[start+1] == '!' || p.src[start+1] == '?':
			p.skipTo('>')
		case p.src[start+1] == '/':
			p.endTag()
		default:
			if err := p.startTag(); err != nil {
				return err
			}
		}
	}
}

// skipComment moves past the comment at the position: its end, or the end of
// the text for a comment that never ends.
func (p *parser) skipComment() {
	end := strings.Index(p.src[p.pos+len("<!--"):], "-->")
	if end < 0 {
		p.pos = len(p.src)
		return
	}
	p.pos += len("<!--") + end + len("-->")
}

// skipTo moves past the next byte c, or to the end of the text.
func (p *parser) skipTo(c byte) {
	end := strings.IndexByte(p.src[p.pos:], c)
	if end < 0 {
		p.pos = len(p.src)
		return
	}
	p.pos += end + 1
}

// endTag reads the end tag at the position. Only </DL> changes the tree: </A>
// and </H3> end a text that already ended at the next tag.
func (p *parser) endTag() {
	name, _ := tagName(p.src, p.pos+2)
	p.skipTo('>')
	if name != "dl" {
		return
	}
	p.descriptionOK = false
	if len(p.levels) > 0 {
		p.levels = p.levels[:len(p.levels)-1]
	}
}

// startTag reads the start tag at the position, and the text that follows it
// for <A>, <H3> and <DD>.
func (p *parser) startTag() error {
	name, next := tagName(p.src, p.pos+1)
	attrs, next := scanAttrs(p.src, next)
	p.pos = next
	switch name {
	case "dl":
		return p.openList()
	case "h3":
		p.descriptionOK = false
		p.pending = folderName(html.UnescapeString(p.text()))
		p.hasPending = true
	case "a":
		p.addLink(attrs)
	case "dd":
		text := p.text()
		if p.descriptionOK && p.lastBookmark >= 0 {
			p.doc.Bookmarks[p.lastBookmark].Notes = bookmarkNotes(html.UnescapeString(text))
		}
		p.descriptionOK = false
	}
	return nil
}

// text returns the text that starts at the position, up to the next tag, and
// moves the position to that tag.
func (p *parser) text() string {
	end := nextTag(p.src, p.pos)
	text := p.src[p.pos:end]
	p.pos = end
	return text
}

// current returns the folder index of the innermost open level: -1 outside
// every <DL>.
func (p *parser) current() int {
	if len(p.levels) == 0 {
		return -1
	}
	return p.levels[len(p.levels)-1]
}

// openList opens 1 level for a <DL>: a folder when an <H3> waits, and the level
// around it otherwise. It returns ErrTooManyFolders for a folder beyond
// MaxFolders.
func (p *parser) openList() error {
	p.descriptionOK = false
	folder := p.current()
	if p.hasPending {
		p.hasPending = false
		key := folderKey{parent: folder, name: p.pending}
		index, ok := p.folderIndex[key]
		if !ok {
			if len(p.doc.Folders) >= MaxFolders {
				return ErrTooManyFolders
			}
			index = len(p.doc.Folders)
			p.folderIndex[key] = index
			p.doc.Folders = append(p.doc.Folders, Folder{Name: key.name, Parent: key.parent})
		}
		folder = index
	}
	p.levels = append(p.levels, folder)
	return nil
}

// addLink reads the text of the <A> at the position, and it adds the link to the
// bookmarks, or to the refusals when its URL breaks a rule.
func (p *parser) addLink(attrs map[string]string) {
	title := bookmarkTitle(html.UnescapeString(p.text()))
	folder := p.current()
	p.descriptionOK = true
	p.lastBookmark = -1
	href, ok := attrs["href"]
	if !ok {
		p.doc.Refusals = append(p.doc.Refusals, Refusal{Folder: folder, Title: title, Reason: "the link has no HREF"})
		return
	}
	href = strings.TrimSpace(href)
	normalized, err := normalizedURL(href)
	if err != nil {
		p.doc.Refusals = append(p.doc.Refusals, Refusal{Folder: folder, URL: href, Title: title, Reason: err.Error()})
		return
	}
	p.lastBookmark = len(p.doc.Bookmarks)
	p.doc.Bookmarks = append(p.doc.Bookmarks, Bookmark{
		Folder: folder,
		URL:    normalized,
		Title:  title,
		Tags:   bookmarkTags(attrs["tags"]),
		Added:  addDate(attrs["add_date"]),
	})
}

// nextTag returns the offset of the first tag that starts at or after from, or
// the length of the text: a "<" that a letter, "/", "!" or "?" follows. Any other
// "<" is text.
func nextTag(src string, from int) int {
	for from < len(src) {
		offset := strings.IndexByte(src[from:], '<')
		if offset < 0 {
			return len(src)
		}
		at := from + offset
		if at+1 < len(src) && isTagStart(src[at+1]) {
			return at
		}
		from = at + 1
	}
	return len(src)
}

// isTagStart reports whether a byte that follows "<" makes a tag.
func isTagStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '/' || c == '!' || c == '?'
}

// isSpace reports whether a byte is HTML white space.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// tagName returns the lower-case name of the tag whose name starts at offset
// from, and the offset after the name.
func tagName(src string, from int) (string, int) {
	end := from
	for end < len(src) && !isSpace(src[end]) && src[end] != '/' && src[end] != '>' {
		end++
	}
	return strings.ToLower(src[from:end]), end
}

// scanAttrs reads the attributes of 1 tag from offset from, up to its ">", and
// returns them with the offset after the ">". Names are in lower case, a value is
// in double quotes, in single quotes or without quotes, or it is absent (an
// empty value), and a character reference of a value is decoded. The first
// attribute of a name stays.
func scanAttrs(src string, from int) (map[string]string, int) {
	var attrs map[string]string
	i := from
	for i < len(src) {
		c := src[i]
		if c == '>' {
			return attrs, i + 1
		}
		if isSpace(c) || c == '/' {
			i++
			continue
		}
		start := i
		for i < len(src) && !isSpace(src[i]) && src[i] != '=' && src[i] != '>' && src[i] != '/' {
			i++
		}
		if i == start {
			// A "=" with no name: it is skipped, so that the scan moves on.
			i++
			continue
		}
		name := strings.ToLower(src[start:i])
		for i < len(src) && isSpace(src[i]) {
			i++
		}
		value := ""
		if i < len(src) && src[i] == '=' {
			i++
			for i < len(src) && isSpace(src[i]) {
				i++
			}
			value, i = scanValue(src, i)
		}
		if attrs == nil {
			attrs = map[string]string{}
		}
		if _, ok := attrs[name]; !ok {
			attrs[name] = value
		}
	}
	return attrs, len(src)
}

// scanValue reads the attribute value that starts at offset from, and returns
// its decoded text with the offset after it.
func scanValue(src string, from int) (string, int) {
	if from >= len(src) {
		return "", from
	}
	if quote := src[from]; quote == '"' || quote == '\'' {
		end := strings.IndexByte(src[from+1:], quote)
		if end < 0 {
			return html.UnescapeString(src[from+1:]), len(src)
		}
		return html.UnescapeString(src[from+1 : from+1+end]), from + end + 2
	}
	end := from
	for end < len(src) && !isSpace(src[end]) && src[end] != '>' {
		end++
	}
	return html.UnescapeString(src[from:end]), end
}
