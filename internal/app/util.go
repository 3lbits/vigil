package app

import (
	"log/slog"
	"net"
	"net/url"
	"strings"
)

func parseCIDRs(raw string) []*net.IPNet {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]*net.IPNet, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		_, cidr, err := net.ParseCIDR(p)
		if err != nil {
			slog.Warn("invalid TRUSTED_PROXY_CIDRS entry", "cidr", p, "error", err)
			continue
		}
		out = append(out, cidr)
	}
	return out
}

func redirectPathFromReferer(ref string) string {
	if ref == "" {
		return "/"
	}
	u, err := url.Parse(ref)
	if err != nil || u.Path == "" {
		return "/"
	}
	redirect := u.Path
	if u.RawQuery == "" {
		return safeLocalRedirect(redirect)
	}
	redirect += "?" + u.RawQuery
	return safeLocalRedirect(redirect)
}

func safeLocalRedirect(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/"
	}
	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, "/\\") {
		return "/"
	}
	return path
}
