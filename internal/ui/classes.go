package ui

import "strings"

func cx(parts ...string) string {
	return strings.TrimSpace(strings.Join(parts, " "))
}

func pick(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return m[fallback]
}
