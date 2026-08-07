package textutil

import (
	"strings"
	"unicode"
)

// SplitBounded splits text at stable natural boundaries when possible.
func SplitBounded(content string, maxRunes int) []string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) == 0 || maxRunes < 1 {
		return nil
	}
	parts := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = preferredBoundary(runes, start, end)
		}
		if end <= start {
			end = min(start+maxRunes, len(runes))
		}
		part := strings.TrimSpace(string(runes[start:end]))
		if part != "" {
			parts = append(parts, part)
		}
		start = end
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}
	return parts
}

func preferredBoundary(runes []rune, start, end int) int {
	for index := end - 1; index > start; index-- {
		if runes[index] == '\n' && runes[index-1] == '\n' {
			return index + 1
		}
	}
	for index := end - 1; index > start; index-- {
		if runes[index] == '\n' {
			return index + 1
		}
	}
	for index := end - 1; index > start; index-- {
		if unicode.IsSpace(runes[index]) {
			return index + 1
		}
	}
	return end
}
