package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"novel/internal/llm"
	"novel/internal/testutil"
	"novel/internal/workbench"
)

const testOrigin = "http://novel.test"

type testLogin struct {
	Cookie *http.Cookie
	CSRF   string
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return &Server{Service: &workbench.Service{DB: testutil.Database(t), Provider: &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: `{"issues":[]}`, Raw: []byte(`{"issues":[]}`)}, nil
	}}, ProviderName: "fake", Model: "fake-v1", MaxOutputTokens: 1600, MaxConcurrent: 2}, Auth: Auth{Username: "author", Password: "test-only-password", PublicOrigin: testOrigin}}
}
func request(s *Server, method, path, body string, session testLogin) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, testOrigin+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if method != "GET" && method != "HEAD" {
		r.Header.Set("Origin", testOrigin)
	}
	if session.Cookie != nil {
		r.AddCookie(session.Cookie)
	}
	if session.CSRF != "" {
		r.Header.Set("X-CSRF-Token", session.CSRF)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func loginForTest(t *testing.T, s *Server) testLogin {
	t.Helper()
	w := request(s, "POST", "/api/session", `{"username":"author","password":"test-only-password"}`, testLogin{})
	if w.Code != 200 {
		t.Fatalf("login %d %s", w.Code, w.Body.String())
	}
	var v struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || v.CSRF == "" {
		t.Fatal("login did not establish session")
	}
	return testLogin{Cookie: cookies[0], CSRF: v.CSRF}
}
func decodeResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProtectedAPIsRejectUnauthenticatedRequests(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/api/projects", "/api/chapters/unknown", "/api/revisions/unknown", "/api/runs/unknown", "/api/runs/unknown/events", "/api/projects/unknown/export", "/api/projects/unknown/state", "/api/projects/unknown/search?q=铜印"} {
		w := request(s, "GET", path, "", testLogin{})
		if w.Code != 401 {
			t.Fatalf("%s status %d", path, w.Code)
		}
	}
	info := request(s, "GET", "/api/session", "", testLogin{})
	var v struct {
		Authenticated bool `json:"authenticated"`
	}
	_ = json.Unmarshal(info.Body.Bytes(), &v)
	if info.Code != 200 || v.Authenticated {
		t.Fatal("anonymous session info invalid")
	}
}

func TestLoginOriginCSRFAndLogout(t *testing.T) {
	s := testServer(t)
	for _, origin := range []string{"", "http://other.test"} {
		r := httptest.NewRequest("POST", testOrigin+"/api/session", strings.NewReader(`{"username":"author","password":"test-only-password"}`))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin %q status %d", origin, w.Code)
		}
	}
	wrong := request(s, "POST", "/api/session", `{"username":"author","password":"wrong"}`, testLogin{})
	if wrong.Code != 401 {
		t.Fatal("wrong credential accepted")
	}
	session := loginForTest(t, s)
	if !session.Cookie.HttpOnly || session.Cookie.SameSite != http.SameSiteStrictMode || session.Cookie.Secure {
		t.Fatalf("local cookie flags %#v", session.Cookie)
	}
	var rawSessionCount int
	if err := s.Service.DB.QueryRow(context.Background(), "SELECT count(*) FROM wb_sessions WHERE id_hash=$1", session.Cookie.Value).Scan(&rawSessionCount); err != nil || rawSessionCount != 0 {
		t.Fatal("session token stored in clear")
	}
	withoutCSRF := session
	withoutCSRF.CSRF = ""
	if w := request(s, "POST", "/api/projects", `{"title":"测试"}`, withoutCSRF); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	forged := session
	forged.CSRF = "forged"
	if w := request(s, "POST", "/api/projects", `{"title":"测试"}`, forged); w.Code != 403 {
		t.Fatal("forged CSRF accepted")
	}
	r := httptest.NewRequest("POST", testOrigin+"/api/projects", strings.NewReader(`{"title":"测试"}`))
	r.AddCookie(session.Cookie)
	r.Header.Set("X-CSRF-Token", session.CSRF)
	r.Header.Set("Origin", "https://attacker.test")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross origin write accepted")
	}
	_ = decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"测试"}`, session))
	if w := request(s, "DELETE", "/api/session", "", session); w.Code != 200 {
		t.Fatalf("logout %d", w.Code)
	}
	if w := request(s, "GET", "/api/projects", "", session); w.Code != 401 {
		t.Fatal("logged out cookie still authenticates")
	}
}

func TestLoginRateLimitAndExpiredSession(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	if _, err := s.Service.DB.Exec(context.Background(), "UPDATE wb_sessions SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if w := request(s, "GET", "/api/projects", "", session); w.Code != 401 {
		t.Fatal("expired session accepted")
	}
	for i := 0; i < 9; i++ {
		w := request(s, "POST", "/api/session", `{"username":"author","password":"wrong"}`, testLogin{})
		if w.Code != 401 {
			t.Fatalf("wrong attempt %d status %d", i, w.Code)
		}
	}
	if w := request(s, "POST", "/api/session", `{"username":"author","password":"wrong"}`, testLogin{}); w.Code != 429 {
		t.Fatalf("rate limit status %d", w.Code)
	}
}
