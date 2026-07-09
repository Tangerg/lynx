package fs

import "strings"

// fuzzyEditRegion locates the byte range in content that matches old when an
// exact substring match has already failed, tolerating only WHITESPACE drift —
// the common reason a model's copied snippet misses (re-indentation, tabs vs
// spaces, trailing whitespace, collapsed runs). It never relaxes the actual
// characters, and it refuses to guess: it returns
//
//	matches == 1  → a single region matched; [start,end) is safe to replace
//	matches  > 1  → several regions matched at that tier; AMBIGUOUS, caller must refuse
//	matches == 0  → no whitespace-variant match; caller reports "not found"
//
// Tiers run strictest-first (trim each line, then collapse internal whitespace);
// the first tier that produces any hit decides, so a looser comparison can never
// override a stricter one. The matched region always spans whole lines, so the
// replacement splices in cleanly between the surrounding newlines.
func fuzzyEditRegion(content, old string) (start, end, matches int) {
	oldLines := strings.Split(old, "\n")
	lines, offsets := linesWithOffsets(content)
	if len(oldLines) > len(lines) {
		return 0, 0, 0
	}
	for _, eq := range []func(a, b string) bool{trimLineEq, normWSEq} {
		var hits [][2]int
		for i := 0; i+len(oldLines) <= len(lines); i++ {
			if windowMatches(lines[i:i+len(oldLines)], oldLines, eq) {
				last := i + len(oldLines) - 1
				hits = append(hits, [2]int{offsets[i], offsets[last] + len(lines[last])})
			}
		}
		if len(hits) == 1 {
			return hits[0][0], hits[0][1], 1
		}
		if len(hits) > 1 {
			return 0, 0, len(hits)
		}
	}
	return 0, 0, 0
}

// linesWithOffsets splits s on '\n' into lines (without the newline) paired with
// each line's start byte offset in s. A trailing '\n' yields a final empty line,
// mirroring strings.Split, so window math stays in lockstep with the split of
// old.
func linesWithOffsets(s string) (lines []string, offsets []int) {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			offsets = append(offsets, start)
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	offsets = append(offsets, start)
	return lines, offsets
}

func windowMatches(window, want []string, eq func(a, b string) bool) bool {
	for i := range want {
		if !eq(window[i], want[i]) {
			return false
		}
	}
	return true
}

// trimLineEq compares two lines ignoring leading/trailing whitespace — handles
// re-indentation and trailing-whitespace drift.
func trimLineEq(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }

// normWSEq compares two lines ignoring ALL whitespace differences — every run of
// spaces/tabs collapses to one and the ends are trimmed. Looser than trimLineEq;
// catches internal spacing changes (e.g. "a  +  b" vs "a + b").
func normWSEq(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}
