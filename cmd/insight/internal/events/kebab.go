package events

import "strings"

// CamelToKebab converts CamelCase to kebab-case.
// e.g., "UserPromptSubmit" -> "user-prompt-submit".
func CamelToKebab(s string) string {
	var result strings.Builder

	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('-')
			}

			result.WriteRune(r + ('a' - 'A'))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// HookEndpoint returns the POST endpoint path for a hook event.
// e.g., "SessionStart" -> "/hooks/v1/session-start".
func HookEndpoint(eventType string) string {
	return "/hooks/v1/" + CamelToKebab(eventType)
}
