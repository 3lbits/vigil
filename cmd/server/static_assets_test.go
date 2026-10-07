package main

import (
	"strings"
	"testing"
)

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

func TestEmbeddedCSSIsPlainStylesheet(t *testing.T) {
	b, err := staticFS.ReadFile("public/css/output.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	if strings.Contains(strings.ToLower(css), "tailwind") || !strings.Contains(css, ".app-frame") || !strings.Contains(css, ".ui2") {
		t.Fatal("embedded CSS must contain the authored app styles, not Tailwind output")
	}
}
