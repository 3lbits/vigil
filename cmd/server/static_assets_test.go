package main

import "testing"

func TestEmbeddedJSPresent(t *testing.T) {
	for _, f := range []string{
		"public/js/htmx.min.js",
		"public/js/alpine.min.js",
		"public/js/mermaid.min.js",
	} {
		b, err := staticFS.ReadFile(f)
		if err != nil {
			t.Fatalf("%s missing from embed: %v", f, err)
		}
		if len(b) == 0 {
			t.Fatalf("%s is empty", f)
		}
	}
}
