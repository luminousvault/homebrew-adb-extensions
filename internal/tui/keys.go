package tui

import "bytes"

// KeyKind is the kind of a decoded key press.
type KeyKind int

// Key kinds. New kinds go at the end so existing values stay stable.
const (
	KeyUnknown KeyKind = iota
	KeyRune            // printable ASCII or space in Rune; also each byte of UTF-8 text (>= 0x80)
	KeyEnter
	KeyBackspace
	KeyDelete
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyCtrlA
	KeyCtrlC
	KeyCtrlE
	KeyPaste // bracketed paste, in Text
	KeyTab
	KeyEsc // Esc pressed alone (see ParseKeys)
	KeyPgUp
	KeyPgDn
)

// Key is one decoded key press.
type Key struct {
	Kind KeyKind
	Rune byte
	Text string
	// Shift is set for Shift+Delete (ESC [3;2~) and Shift+Tab (ESC [Z),
	// which are otherwise a KeyDelete and a KeyTab: only screens that give
	// them a meaning of their own look.
	Shift bool
}

var escSeqs = map[string]KeyKind{
	"[A": KeyUp, "[B": KeyDown, "[C": KeyRight, "[D": KeyLeft,
	"OA": KeyUp, "OB": KeyDown, "OC": KeyRight, "OD": KeyLeft,
	"[H": KeyHome, "[F": KeyEnd, "OH": KeyHome, "OF": KeyEnd,
	"[1~": KeyHome, "[4~": KeyEnd, "[7~": KeyHome, "[8~": KeyEnd,
	"[3~": KeyDelete, "[5~": KeyPgUp, "[6~": KeyPgDn,
	shiftDelete: KeyDelete, shiftTab: KeyTab,
}

// shiftDelete is Shift+Delete after ESC (xterm's modifier 2 = Shift).
const shiftDelete = "[3;2~"

// shiftTab is Shift+Tab after ESC (back tab).
const shiftTab = "[Z"

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// ParseKeys decodes buf into keys. It returns the number of bytes consumed;
// an incomplete escape sequence or paste at the end is left unconsumed. A lone
// ESC at the very end is also left unconsumed: terminals send an escape
// sequence in one write, so a caller whose read ends in a lone ESC treats it as
// KeyEsc (see LoneEsc).
func ParseKeys(buf []byte) ([]Key, int) {
	var keys []Key
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
			keys = append(keys, Key{Kind: KeyPaste, Text: string(rest[len(pasteStart):end])})
			i += end + len(pasteEnd)
			continue
		}
		n, k, complete := escapeSeq(rest[1:])
		if !complete {
			return keys, i
		}
		seq := string(rest[1 : 1+n])
		keys = append(keys, Key{Kind: k, Shift: seq == shiftDelete || seq == shiftTab})
		i += 1 + n
	}
	return keys, i
}

// escapeSeq parses the bytes after ESC. A lone ESC (no following bytes) is
// reported incomplete; callers flush it as unknown after a short wait.
func escapeSeq(b []byte) (n int, k KeyKind, complete bool) {
	if len(b) == 0 {
		return 0, KeyUnknown, false
	}
	if b[0] != '[' && b[0] != 'O' {
		return 0, KeyEsc, true // lone ESC followed by a normal key
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
		return j, KeyUnknown, true
	}
	return 0, KeyUnknown, false
}

func controlKey(c byte) Key {
	switch c {
	case '\r', '\n':
		return Key{Kind: KeyEnter}
	case 0x7f, 0x08:
		return Key{Kind: KeyBackspace}
	case 0x01:
		return Key{Kind: KeyCtrlA}
	case 0x03:
		return Key{Kind: KeyCtrlC}
	case 0x05:
		return Key{Kind: KeyCtrlE}
	case '\t':
		return Key{Kind: KeyTab}
	}
	if (c >= 32 && c <= 126) || c >= 0x80 {
		return Key{Kind: KeyRune, Rune: c}
	}
	return Key{Kind: KeyUnknown}
}

// LoneEsc reports whether rest, the bytes ParseKeys left unconsumed after a
// read, is a lone ESC: the Esc key rather than the start of a sequence.
func LoneEsc(rest []byte) bool { return len(rest) == 1 && rest[0] == 0x1b }
