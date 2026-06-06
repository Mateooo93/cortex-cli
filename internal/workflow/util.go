package workflow

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// osGetenv is a tiny shim so the engine can be tested without
// the real environment. Production code uses os.Getenv.
var osGetenv = os.Getenv

// jsonUnmarshal is a tiny shim so the engine can be tested
// without a real JSON parser. Production code uses
// json.Unmarshal.
var jsonUnmarshal = json.Unmarshal

// extractJSON pulls a balanced JSON object out of an LLM
// response. Tries fenced ```json first, then walks the string
// looking for the first balanced { ... } block. Returns "" if
// nothing parseable is found.
//
// The LLM is notoriously sloppy with JSON output, so we have
// to be tolerant: ignore braces inside strings, handle
// escapes, etc. A real parser is overkill here.
var fencedJSONRE = regexp.MustCompile("(?s)```(?:json)?\\s*\\n([\\s\\S]*?)\\n```")

func extractJSON(content string) string {
	if m := fencedJSONRE.FindStringSubmatch(content); m != nil {
		return strings.TrimSpace(m[1])
	}
	start := strings.Index(content, "{")
	if start < 0 {
		return ""
	}
	depth, inString, escape := 0, false, false
	for i := start; i < len(content); i++ {
		c := content[i]
		if escape {
			escape = false
			continue
		}
		if c == '\\' {
			escape = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return ""
}
