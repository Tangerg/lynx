package input

import (
	"strings"
	"unicode/utf8"
)

// params is a control sequence's parameter section, parsed once and read by
// whichever decoder the sequence's final byte selects.
//
// There is one parser rather than one per sequence family. Two parsers over the
// same syntax drift: they disagree about what an empty field means, or about what
// to do with a field that is not a number, and the sequence that exercises the
// difference is the one nobody tested.
type params struct {
	// private is the marker byte a private sequence begins with — '<' for a mouse
	// report — or zero for an ordinary sequence.
	private byte
	// groups are the semicolon-separated parameters, each of which may carry
	// colon-separated subparameters.
	//
	// A field left empty is the protocol's default, which is zero. A field that is
	// not a number, or is far larger than any parameter legitimately gets, is -1:
	// a decoder can then refuse the report instead of acting on a value invented
	// for it.
	groups [][]int
}

// paramLimit is well past any real parameter and short of anything that could
// overflow. A number beyond it is treated as malformed.
const paramLimit = 1 << 20

func parseParams(body string) params {
	var ps params
	if body != "" && body[0] >= 0x3c && body[0] <= 0x3f {
		ps.private = body[0]
		body = body[1:]
	}
	if body == "" {
		return ps
	}
	fields := strings.Split(body, ";")
	ps.groups = make([][]int, 0, len(fields))
	for _, field := range fields {
		subs := strings.Split(field, ":")
		group := make([]int, 0, len(subs))
		for _, sub := range subs {
			group = append(group, parseParam(sub))
		}
		ps.groups = append(ps.groups, group)
	}
	return ps
}

func parseParam(s string) int {
	if s == "" {
		return 0
	}
	value := 0
	for i := range len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			return -1
		}
		value = value*10 + int(c-'0')
		if value > paramLimit {
			return -1
		}
	}
	return value
}

// empty reports whether the sequence carried no parameters.
func (ps params) empty() bool { return len(ps.groups) == 0 }

// mouse reports whether the sequence is a mouse report.
func (ps params) mouse() bool { return ps.private == '<' }

// first is the leading parameter, or zero when there was none. Zero is the
// protocol's own default for a missing parameter, so a caller need not distinguish.
func (ps params) first() int { return ps.at(0) }

// at is the leading value of group i, or zero when the group is absent.
func (ps params) at(i int) int {
	if i >= len(ps.groups) || len(ps.groups[i]) == 0 {
		return 0
	}
	return ps.groups[i][0]
}

// count is how many parameter groups the sequence carried.
func (ps params) count() int { return len(ps.groups) }

// keyMeta reads the modifier and transition group that key reports carry, and
// that the Kitty keyboard protocol also adds to arrow and numbered-key reports.
//
// It reports false for a group that is malformed rather than guessing: a key
// event with the wrong modifiers is worse than no key event, because it fires
// something the user did not ask for.
func (ps params) keyMeta() (Mods, Transition, bool) {
	if ps.count() < 2 || len(ps.groups[1]) == 0 {
		return 0, Press, true
	}
	group := ps.groups[1]
	if len(group) > 2 || group[0] < 0 {
		return 0, Press, false
	}

	var mods Mods
	if group[0] > 1 {
		// The encoding is the modifier bits plus one, so that a parameter of one
		// means no modifiers and the field is never empty.
		mods = Mods(group[0]-1) & (Shift | Alt | Ctrl | Super)
	}
	if len(group) < 2 {
		return mods, Press, true
	}
	switch group[1] {
	case 0, 1:
		return mods, Press, true
	case 2:
		return mods, Repeat, true
	case 3:
		return mods, Release, true
	default:
		return 0, Press, false
	}
}

// text reads the associated-text group of a Kitty key report: the code points the
// key produced, which the terminal is better placed to know than this program is.
func (ps params) text() (string, bool) {
	if ps.count() < 3 {
		return "", true
	}
	var b strings.Builder
	for _, cp := range ps.groups[2] {
		if cp == 0 {
			continue
		}
		r := rune(cp)
		if cp < 0 || !utf8.ValidRune(r) {
			return "", false
		}
		b.WriteRune(r)
	}
	return b.String(), true
}
