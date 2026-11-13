package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"novel/internal/job"
	"novel/internal/workbench"
	"strings"
	"sync"
	"time"
)

type authContext struct {
	CSRF      string
	Token     string
	Principal workbench.Principal
}
type loginWindow struct {
	Count int
	Since time.Time
}
type Auth struct {
	Username, Password, PublicOrigin string
	Secure                           bool
	mu                               sync.Mutex
	attempts                         map[string]loginWindow
}

func (a *Auth) validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	scheme := "http"
	if a.Secure {
		scheme = "https"
	}
	expected := a.PublicOrigin
	if expected == "" {
		expected = scheme + "://" + r.Host
	}
	return origin != "" && strings.TrimRight(origin, "/") == strings.TrimRight(expected, "/")
}
func (s *Server) session(r *http.Request) (authContext, bool) {
	cookie, err := r.Cookie("novel_session")
	if err != nil {
		return authContext{}, false
	}
	var session authContext
	var uid *string
	err = s.Service.DB.QueryRow(r.Context(), `SELECT s.csrf_token,s.user_id,s.is_admin,COALESCE(NULLIF(s.username,''),u.username,''),COALESCE(u.display_name,'') FROM wb_sessions s LEFT JOIN wb_users u ON u.id=s.user_id WHERE s.id_hash=$1 AND s.expires_at>now() AND (s.is_admin OR (u.id IS NOT NULL AND NOT u.disabled))`, job.Hash([]byte(cookie.Value))).Scan(&session.CSRF, &uid, &session.Principal.Admin, &session.Principal.Username, &session.Principal.DisplayName)
	if err != nil {
		return authContext{}, false
	}
	session.Token = cookie.Value
	if session.Principal.Admin {
		session.Principal.ID = "admin"
		session.Principal.Username = s.Auth.Username
		session.Principal.DisplayName = s.Auth.Username
	} else if uid != nil {
		session.Principal.ID = *uid
	}
	return session, true
}
func (s *Server) sessionInfo(w http.ResponseWriter, r *http.Request) {
	session, ok := s.session(r)
	respond(w, 200, map[string]any{"authenticated": ok, "csrfToken": session.CSRF, "username": session.Principal.Username, "user": session.Principal})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.Auth.validOrigin(r) {
		respond(w, 403, map[string]string{"error": "来源校验失败"})
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.Auth.mu.Lock()
	if s.Auth.attempts == nil {
		s.Auth.attempts = map[string]loginWindow{}
	}
	window := s.Auth.attempts[ip]
	if time.Since(window.Since) > time.Minute {
		window = loginWindow{Since: time.Now()}
	}
	window.Count++
	s.Auth.attempts[ip] = window
	s.Auth.mu.Unlock()
	if window.Count > 10 {
		respond(w, 429, map[string]string{"error": "尝试过于频繁，请稍后重试"})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	actual := sha256.Sum256([]byte(input.Username + "\x00" + input.Password))
	expected := sha256.Sum256([]byte(s.Auth.Username + "\x00" + s.Auth.Password))
	principal := workbench.Principal{ID: "admin", Username: s.Auth.Username, DisplayName: s.Auth.Username, Admin: true}
	if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
		user, ok := s.Service.AuthenticateUser(r.Context(), input.Username, input.Password)
		if ok {
			principal = workbench.Principal{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName}
		} else {
			respond(w, 401, map[string]string{"error": "用户名或密码错误"})
			return
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fail(w, err)
		return
	}
	token := hex.EncodeToString(b)
	csrf := job.NewID()
	if _, err := s.Service.DB.Exec(r.Context(), "DELETE FROM wb_sessions WHERE expires_at<=now()"); err != nil {
		fail(w, err)
		return
	}
	var uid any
	if !principal.Admin {
		uid = principal.ID
	}
	if _, err := s.Service.DB.Exec(r.Context(), "INSERT INTO wb_sessions(id_hash,csrf_token,expires_at,user_id,is_admin,username) VALUES($1,$2,now()+interval '7 days',$3,$4,$5)", job.Hash([]byte(token)), csrf, uid, principal.Admin, principal.Username); err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "novel_session", Value: token, Path: "/", HttpOnly: true, Secure: s.Auth.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 7 * 24 * 3600})
	respond(w, 200, map[string]any{"authenticated": true, "csrfToken": csrf, "username": principal.Username, "user": principal})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	session, _ := s.session(r)
	_, err := s.Service.DB.Exec(r.Context(), "DELETE FROM wb_sessions WHERE id_hash=$1", job.Hash([]byte(session.Token)))
	if err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "novel_session", Value: "", Path: "/", HttpOnly: true, Secure: s.Auth.Secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	respond(w, 200, map[string]bool{"authenticated": false})
}
func (s *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := s.session(r)
		if !ok {
			respond(w, 401, map[string]string{"error": "请先登录"})
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if !s.Auth.validOrigin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
				respond(w, 403, map[string]string{"error": "会话校验失败，请刷新页面"})
				return
			}
		}
		r = r.WithContext(workbench.WithPrincipal(r.Context(), session.Principal))
		if err := s.authorizeRequest(r); err != nil {
			if err == workbench.ErrForbidden {
				respond(w, 403, map[string]string{"error": err.Error()})
			} else {
				fail(w, err)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
