package csrf

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func csrfCookieValue(cookies []*http.Cookie) string {
	for _, c := range cookies {
		if c.Name == "_csrf" {
			return c.Value
		}
	}
	return ""
}

func TestMiddleware_GETSetsTokenCookieAndContext(t *testing.T) {
	var token string
	h := Middleware([]byte("test-key"), false, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = TokenFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(w, r)

	if token == "" {
		t.Fatal("expected csrf token in request context")
	}
	if got := csrfCookieValue(w.Result().Cookies()); got == "" {
		t.Fatal("expected _csrf cookie on response")
	}
}

func TestMiddleware_POSTAcceptsHeaderToken(t *testing.T) {
	mw := Middleware([]byte("test-key"), false, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	getW := httptest.NewRecorder()
	h.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/", nil))
	token := csrfCookieValue(getW.Result().Cookies())
	if token == "" {
		t.Fatal("missing csrf cookie token")
	}

	postR := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	postR.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	postR.Header.Set("X-CSRF-Token", token)
	postW := httptest.NewRecorder()
	h.ServeHTTP(postW, postR)

	if postW.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, postW.Code)
	}
}

func TestMiddleware_POSTAcceptsFormToken(t *testing.T) {
	mw := Middleware([]byte("test-key"), false, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	getW := httptest.NewRecorder()
	h.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/", nil))
	token := csrfCookieValue(getW.Result().Cookies())
	if token == "" {
		t.Fatal("missing csrf cookie token")
	}

	form := url.Values{"_csrf": {token}}
	postR := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	postR.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postR.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	postW := httptest.NewRecorder()
	h.ServeHTTP(postW, postR)

	if postW.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, postW.Code)
	}
}

func TestMiddleware_POSTAcceptsMultipartToken(t *testing.T) {
	mw := Middleware([]byte("test-key"), false, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	getW := httptest.NewRecorder()
	h.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/", nil))
	token := csrfCookieValue(getW.Result().Cookies())
	if token == "" {
		t.Fatal("missing csrf cookie token")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("_csrf", token); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	postR := httptest.NewRequest(http.MethodPost, "/", &body)
	postR.Header.Set("Content-Type", writer.FormDataContentType())
	postR.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	postW := httptest.NewRecorder()
	h.ServeHTTP(postW, postR)

	if postW.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, postW.Code)
	}
}

func TestMiddleware_POSTMissingTokenForbidden(t *testing.T) {
	h := Middleware([]byte("test-key"), false, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	postW := httptest.NewRecorder()
	postR := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	h.ServeHTTP(postW, postR)

	if postW.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, postW.Code)
	}
}

func TestMiddleware_InvalidCookieIsReplaced(t *testing.T) {
	h := Middleware([]byte("test-key"), false, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "_csrf", Value: "invalid"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	newToken := csrfCookieValue(w.Result().Cookies())
	if newToken == "" || newToken == "invalid" {
		t.Fatalf("expected invalid token to be replaced, got %q", newToken)
	}
}

func TestMiddleware_SessionBoundTokenRejectedAcrossSessions(t *testing.T) {
	getSessionToken := func(r *http.Request) string {
		return r.Header.Get("X-Session-Token")
	}
	mw := Middleware([]byte("test-key"), false, getSessionToken)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	getW := httptest.NewRecorder()
	getR := httptest.NewRequest(http.MethodGet, "/", nil)
	getR.Header.Set("X-Session-Token", "session-a")
	h.ServeHTTP(getW, getR)
	token := csrfCookieValue(getW.Result().Cookies())
	if token == "" {
		t.Fatal("missing csrf cookie token")
	}

	postW := httptest.NewRecorder()
	postR := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	postR.Header.Set("X-Session-Token", "session-b")
	postR.Header.Set("X-CSRF-Token", token)
	postR.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	h.ServeHTTP(postW, postR)

	if postW.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, postW.Code)
	}
}

func TestMiddleware_SessionBoundTokenAcceptedSameSession(t *testing.T) {
	getSessionToken := func(r *http.Request) string {
		return r.Header.Get("X-Session-Token")
	}
	mw := Middleware([]byte("test-key"), false, getSessionToken)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	getW := httptest.NewRecorder()
	getR := httptest.NewRequest(http.MethodGet, "/", nil)
	getR.Header.Set("X-Session-Token", "session-a")
	h.ServeHTTP(getW, getR)
	token := csrfCookieValue(getW.Result().Cookies())
	if token == "" {
		t.Fatal("missing csrf cookie token")
	}

	postW := httptest.NewRecorder()
	postR := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	postR.Header.Set("X-Session-Token", "session-a")
	postR.Header.Set("X-CSRF-Token", token)
	postR.AddCookie(&http.Cookie{Name: "_csrf", Value: token})
	h.ServeHTTP(postW, postR)

	if postW.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, postW.Code)
	}
}
