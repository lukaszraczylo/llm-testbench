package eval

import (
	"fmt"
	"regexp"
	"strings"
)

// fencePattern matches a fenced code block: ```lang\n...body...```
var fencePattern = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\n?(.*?)```")

// ExtractCodeBlock returns the first fenced code block tagged with lang
// (case-insensitive). If none is tagged, it falls back to the first
// untagged fenced block, then to the first fenced block of any language,
// then to the whole trimmed response if there are no fences at all.
func ExtractCodeBlock(response, lang string) string {
	matches := fencePattern.FindAllStringSubmatch(response, -1)
	wantLang := strings.ToLower(strings.TrimSpace(lang))

	var untagged string
	for _, m := range matches {
		blockLang := strings.ToLower(strings.TrimSpace(m[1]))
		if wantLang != "" && blockLang == wantLang {
			return strings.TrimSpace(m[2])
		}
		if untagged == "" && blockLang == "" {
			untagged = m[2]
		}
	}
	if untagged != "" {
		return strings.TrimSpace(untagged)
	}
	if len(matches) > 0 {
		return strings.TrimSpace(matches[0][2])
	}
	return strings.TrimSpace(response)
}

// jsonFencePattern matches a fenced ```json ... ``` block specifically
// (case-insensitive on the "json" language tag).
var jsonFencePattern = regexp.MustCompile("(?is)```json\\s*\\n?(.*?)```")

// findAllBalancedJSON scans text for every top-level, bracket-balanced JSON
// object/array literal, in the order they appear. Brackets inside string
// literals are ignored. An unbalanced trailing fragment (an opening
// bracket with no matching close) is not included.
func findAllBalancedJSON(text string) []string {
	var out []string
	pos := 0
	for pos < len(text) {
		start := -1
		for i := pos; i < len(text); i++ {
			if text[i] == '{' || text[i] == '[' {
				start = i
				break
			}
		}
		if start == -1 {
			break
		}

		depth := 0
		inString := false
		escaped := false
		end := -1
		for i := start; i < len(text); i++ {
			c := text[i]
			if inString {
				switch {
				case escaped:
					escaped = false
				case c == '\\':
					escaped = true
				case c == '"':
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end != -1 {
				break
			}
		}
		if end == -1 {
			break
		}
		out = append(out, text[start:end+1])
		pos = end + 1
	}
	return out
}

// ExtractJSON returns response's intended JSON object/array literal,
// tolerant of surrounding prose and markdown code fences. It prefers the
// content of the first ```json fenced block, if any; otherwise it takes
// the LAST balanced JSON value anywhere in response. Taking the last
// (rather than the first) value matters for a model that thinks out loud
// with a candidate JSON draft before committing to a final answer: the
// final JSON, not the draft, is the one to score.
func ExtractJSON(response string) (string, error) {
	if m := jsonFencePattern.FindStringSubmatch(response); m != nil {
		if all := findAllBalancedJSON(m[1]); len(all) > 0 {
			return all[0], nil
		}
	}

	all := findAllBalancedJSON(response)
	if len(all) == 0 {
		return "", fmt.Errorf("no JSON object or array found in response")
	}
	return all[len(all)-1], nil
}
