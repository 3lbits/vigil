package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/3lbits/vigil/internal/httpresp"
)

type ipEntry struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

// IPRateLimiter is a fixed-window per-IP rate limiter with no external dependencies.
type IPRateLimiter struct {
	entries    sync.Map
	limit      int
	window     time.Duration
	requestKey func(*http.Request) string
}

func NewIPRateLimiter(limit int, window time.Duration) *IPRateLimiter {
	return NewIPRateLimiterWithKey(limit, window, nil)
}

// NewIPRateLimiterWithKey returns a fixed-window rate limiter that uses requestKey
// to derive the bucket key. Empty keys fall back to RemoteAddr host parsing.
func NewIPRateLimiterWithKey(limit int, window time.Duration, requestKey func(*http.Request) string) *IPRateLimiter {
	if requestKey == nil {
		requestKey = remoteIPFromRequest
	}
	l := &IPRateLimiter{
		limit:      limit,
		window:     window,
		requestKey: requestKey,
	}
	go l.cleanupLoop()
	return l
}

func (l *IPRateLimiter) cleanupLoop() {
	t := time.NewTicker(l.window)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		l.entries.Range(func(k, v any) bool {
			e, ok := v.(*ipEntry)
			if ok && now.After(e.resetAt.Add(l.window)) {
				l.entries.Delete(k)
			}
			return true
		})
	}
}

func (l *IPRateLimiter) allow(ip string) bool {
	now := time.Now()
	v, _ := l.entries.LoadOrStore(ip, &ipEntry{resetAt: now.Add(l.window)})
	e, ok := v.(*ipEntry)
	if !ok {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if now.After(e.resetAt) {
		e.count = 0
		e.resetAt = now.Add(l.window)
	}
	e.count++
	return e.count <= l.limit
}

func remoteIPFromRequest(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (l *IPRateLimiter) bucketKey(r *http.Request) string {
	if l.requestKey == nil {
		return remoteIPFromRequest(r)
	}
	k := strings.TrimSpace(l.requestKey(r))
	if k == "" {
		return remoteIPFromRequest(r)
	}
	return k
}

// Wrap returns a HandlerFunc that applies the rate limit keyed on request key.
// Empty keys fall back to RemoteAddr host parsing.
func (l *IPRateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := l.bucketKey(r)
		if !l.allow(ip) {
			httpresp.TooManyRequests(w, r)
			return
		}
		next(w, r)
	}
}
