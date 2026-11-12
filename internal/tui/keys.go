package tui

import "bytes"

type keyKind int

const (
	keyUnknown keyKind = iota
	keyRune            // printable ASCII or space, in r
	keyEnter
	keyBackspace
	keyDelete
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyCtrlA
	keyCtrlC
	keyCtrlE
	keyPaste // bracketed paste, in text
)

type key struct {
	kind keyKind
	r    byte
	text string
}

var escSeqs = map[string]keyKind{
	"[A": keyUp, "[B": keyDown, "[C": keyRight, "[D": keyLeft,
	"OA": keyUp, "OB": keyDown, "OC": keyRight, "OD": keyLeft,
	"[H": keyHome, "[F": keyEnd, "OH": keyHome, "OF": keyEnd,
	"[1~": keyHome, "[4~": keyEnd, "[7~": keyHome, "[8~": keyEnd,
	"[3~": keyDelete,
}

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// parseKeys decodes buf into keys. It returns the number of bytes consumed;
// an incomplete escape sequence or paste at the end is left unconsumed.
func parseKeys(buf []byte) ([]key, int) {
	var keys []key
	i := 0
	for i < len(buf) {
		c := buf[i]
		if c != 0x1b {
			keys = append(keys, controlKey(c))
			i++
			continue
		}
		rest := buf[i:]
		if bytes.HasPrefix(rest, []byte(pasteStart)) {
			end := bytes.Index(rest, []byte(pasteEnd))
			if end < 0 {
				return keys, i // wait for the rest of the paste
			}
			keys = append(keys, key{kind: keyPaste, text: string(rest[len(pasteStart):end])})
			i += end + len(pasteEnd)
			continue
		}
		n, k, complete := escapeSeq(rest[1:])
		if !complete {
			return keys, i
		}
		keys = append(keys, key{kind: k})
		i += 1 + n
	}
	return keys, i
}

// escapeSeq parses the bytes after ESC. A lone ESC (no following bytes) is
// reported incomplete; callers flush it as unknown after a short wait.
func escapeSeq(b []byte) (n int, k keyKind, complete bool) {
	if len(b) == 0 {
		return 0, keyUnknown, false
	}
	if b[0] != '[' && b[0] != 'O' {
		return 0, keyUnknown, true // lone ESC followed by a normal key
	}
	// CSI/SS3: parameters (digits, ';') then a final byte in 0x40-0x7e.
	for j := 1; j < len(b); j++ {
		c := b[j]
		if (c >= '0' && c <= '9') || c == ';' {
			continue
		}
		if c >= 0x40 && c <= 0x7e {
			return j + 1, escSeqs[string(b[:j+1])], true
		}
		return j, keyUnknown, true
	}
	return 0, keyUnknown, false
}

func controlKey(c byte) key {
	switch c {
	case '\r', '\n':
		return key{kind: keyEnter}
	case 0x7f, 0x08:
		return key{kind: keyBackspace}
	case 0x01:
		return key{kind: keyCtrlA}
	case 0x03:
		return key{kind: keyCtrlC}
	case 0x05:
		return key{kind: keyCtrlE}
	}
	if c >= 32 && c <= 126 {
		return key{kind: keyRune, r: c}
	}
	return key{kind: keyUnknown}
}
