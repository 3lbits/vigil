package app

import (
	"net"
	"testing"
)

func TestSafeLocalRedirect(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "/"},
		{name: "absolute url", in: "https://evil.example/x", want: "/"},
		{name: "protocol relative", in: "//evil.example/x", want: "/"},
		{name: "backslash prefixed", in: "/\\evil.example/x", want: "/"},
		{name: "local path", in: "/dashboard", want: "/dashboard"},
		{name: "local path with query", in: "/dashboard?tab=open", want: "/dashboard?tab=open"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeLocalRedirect(tc.in)
			if got != tc.want {
				t.Fatalf("safeLocalRedirect(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedirectPathFromReferer(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "empty", ref: "", want: "/"},
		{name: "invalid", ref: "://", want: "/"},
		{name: "normal local path", ref: "https://app.example/risk?id=1", want: "/risk?id=1"},
		{name: "protocol relative path", ref: "https://app.example//evil.example", want: "/"},
		{name: "backslash prefixed path", ref: "https://app.example/\\evil.example", want: "/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redirectPathFromReferer(tc.ref)
			if got != tc.want {
				t.Fatalf("redirectPathFromReferer(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // expected CIDR strings; nil means nil return
	}{
		{name: "empty string", input: "", want: nil},
		{name: "single valid CIDR", input: "10.0.0.0/8", want: []string{"10.0.0.0/8"}},
		{name: "multiple valid CIDRs", input: "10.0.0.0/8,192.168.0.0/16", want: []string{"10.0.0.0/8", "192.168.0.0/16"}},
		{name: "whitespace around entries", input: " 10.0.0.0/8 , 172.16.0.0/12 ", want: []string{"10.0.0.0/8", "172.16.0.0/12"}},
		{name: "invalid entry skipped", input: "not-a-cidr,10.0.0.0/8", want: []string{"10.0.0.0/8"}},
		{name: "all invalid entries", input: "bad,also-bad", want: []string{}},
		{name: "blank entries skipped", input: "10.0.0.0/8,,192.168.0.0/16", want: []string{"10.0.0.0/8", "192.168.0.0/16"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCIDRs(tc.input)
			assertParsedCIDRs(t, tc.input, got, tc.want)
		})
	}
}

func assertParsedCIDRs(t *testing.T, input string, got []*net.IPNet, want []string) {
	t.Helper()

	if want == nil {
		if got != nil {
			t.Fatalf("parseCIDRs(%q) = %v, want nil", input, got)
		}
		return
	}
	if len(got) != len(want) {
		t.Fatalf("parseCIDRs(%q) len=%d, want %d: %v", input, len(got), len(want), got)
	}
	for i, cidr := range want {
		_, wantNet, _ := net.ParseCIDR(cidr)
		if got[i].String() != wantNet.String() {
			t.Errorf("entry %d: got %v, want %v", i, got[i], wantNet)
		}
	}
}
