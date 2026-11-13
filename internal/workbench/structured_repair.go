package workbench

import (
	"encoding/json"
	"strings"
)

// repairStructuredJSON changes transport formatting only. It never invents
// missing keys, evidence, enums, or facts; callers must run the closed schema
// and source validators again. The original provider response stays in attempt.
func repairStructuredJSON(source string) (string, bool) {
	if len(source) > 1<<20 {
		return source, false
	}
	value := strings.TrimSpace(strings.TrimPrefix(source, "\ufeff"))
	if strings.HasPrefix(value, "```json\n") && strings.HasSuffix(value, "```") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "```json\n"), "```"))
	} else if strings.HasPrefix(value, "```\n") && strings.HasSuffix(value, "```") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "```\n"), "```"))
	}
	var b strings.Builder
	inString, escape := false, false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if inString {
			b.WriteByte(c)
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		}
		if c == ',' {
			j := i + 1
			for j < len(value) && strings.ContainsRune(" \t\r\n", rune(value[j])) {
				j++
			}
			if j < len(value) && (value[j] == '}' || value[j] == ']') {
				continue
			}
		}
		b.WriteByte(c)
	}
	result := b.String()
	if result == source || !strings.HasPrefix(result, "{") || !json.Valid([]byte(result)) {
		return source, false
	}
	return result, true
}
