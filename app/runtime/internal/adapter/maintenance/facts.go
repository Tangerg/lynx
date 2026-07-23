package maintenance

import "strings"

// parseMemoryFacts translates the utility model's Markdown response into the
// structured fact vocabulary the memory domain accepts. Fences, list markers,
// and sentinel tokens are properties of the extraction prompt contract.
func parseMemoryFacts(markdown string) []string {
	var facts []string
	seen := make(map[string]struct{})
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "```" || strings.EqualFold(line, "NO_FACTS") || strings.EqualFold(line, "NO_MEMORY") {
			continue
		}
		line = trimFactMarker(line)
		if line == "" {
			continue
		}
		if _, duplicate := seen[line]; duplicate {
			continue
		}
		seen[line] = struct{}{}
		facts = append(facts, line)
	}
	return facts
}

func trimFactMarker(line string) string {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && line[1] == ' ' {
		return strings.TrimSpace(line[2:])
	}
	if index := strings.IndexByte(line, '.'); index > 0 && index+1 < len(line) && line[index+1] == ' ' {
		for _, digit := range line[:index] {
			if digit < '0' || digit > '9' {
				return line
			}
		}
		return strings.TrimSpace(line[index+2:])
	}
	return line
}
