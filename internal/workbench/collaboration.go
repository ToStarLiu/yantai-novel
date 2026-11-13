package workbench

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

// Request principals are set only after verifying a server-side session. Internal
// worker contexts deliberately have no principal, and never reach an HTTP route.
type Principal struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Admin       bool   `json:"admin"`
}
type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

var ErrForbidden = errors.New("没有访问该作品或执行此操作的权限")

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Disabled    bool      `json:"disabled"`
	CreatedAt   time.Time `json:"createdAt"`
}
type Member struct {
	UserID      string    `json:"userId"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	JoinedAt    time.Time `json:"joinedAt"`
}
type Invitation struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	Role       string    `json:"role"`
	Token      string    `json:"token,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt"`
	AcceptedBy string    `json:"acceptedBy"`
	Revoked    bool      `json:"revoked"`
	CreatedAt  time.Time `json:"createdAt"`
}
type Presence struct {
	UserID      string    `json:"userId"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	ChapterID   string    `json:"chapterId"`
	RevisionID  string    `json:"revisionId"`
	Cursor      *int      `json:"cursor,omitempty"`
	SeenAt      time.Time `json:"seenAt"`
}
type PresenceInput struct {
	ChapterID  string `json:"chapterId"`
	RevisionID string `json:"revisionId"`
	Cursor     *int   `json:"cursor,omitempty"`
}
type CollaborationEvent struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"projectId"`
	Kind      string    `json:"kind"`
	ObjectID  string    `json:"objectId"`
	CreatedAt time.Time `json:"createdAt"`
}

func validRole(role string) bool {
	return role == "owner" || role == "editor" || role == "reviewer" || role == "viewer"
}
func roleLevel(role string) int {
	switch role {
	case "owner":
		return 4
	case "editor":
		return 3
	case "reviewer":
		return 2
	case "viewer":
		return 1
	}
	return 0
}
func normalizedUsername(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) < 2 || len(name) > 80 {
		return "", fmt.Errorf("用户名需要2至80字节")
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("/\\@", r) {
			return "", fmt.Errorf("用户名不可包含空白或特殊分隔符")
		}
	}
	return name, nil
}
func passwordHash(password string) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("密码需要12至1024字节")
	}
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 210000, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256$210000$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key), nil
}
func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || len(password) > 1024 {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 100000 || iterations > 1000000 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil || len(salt) != 32 {
		return false
	}
	expected, err := hex.DecodeString(parts[3])
	if err != nil || len(expected) != 32 {
		return false
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	return err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
}
func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Disabled, &u.CreatedAt)
	return u, noRows(err)
}
func (s *Service) CreateUser(ctx context.Context, username, password, displayName string) (User, error) {
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		return User{}, ErrForbidden
	}
	name, err := normalizedUsername(username)
	if err != nil {
		return User{}, err
	}
	if !validName(displayName) {
		displayName = name
	}
	hash, err := passwordHash(password)
	if err != nil {
		return User{}, err
	}
	return scanUser(s.DB.QueryRow(ctx, "INSERT INTO wb_users(id,username,display_name,password_hash) VALUES($1,$2,$3,$4) RETURNING id,username,display_name,disabled,created_at", job.NewID(), name, displayName, hash))
}
func (s *Service) AuthenticateUser(ctx context.Context, username, password string) (User, bool) {
	name, e := normalizedUsername(username)
	if e != nil {
		return User{}, false
	}
	var hash string
	var u User
	e = s.DB.QueryRow(ctx, "SELECT id,username,display_name,disabled,created_at,password_hash FROM wb_users WHERE username=$1", name).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Disabled, &u.CreatedAt, &hash)
	if e != nil { // Equal-cost unknown-account verification prevents a useful timing oracle.
		_ = verifyPassword("pbkdf2-sha256$210000$"+strings.Repeat("00", 32)+"$"+strings.Repeat("00", 32), password)
		return User{}, false
	}
	return u, !u.Disabled && verifyPassword(hash, password)
}
func (s *Service) Users(ctx context.Context) ([]User, error) {
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		return nil, ErrForbidden
	}
	rows, err := s.DB.Query(ctx, "SELECT id,username,display_name,disabled,created_at FROM wb_users ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *Service) ProjectRole(ctx context.Context, pid string) (string, error) {
	p, ok := PrincipalFromContext(ctx)
	if !ok || p.Admin {
		return "owner", nil
	}
	var role string
	err := s.DB.QueryRow(ctx, "SELECT m.role FROM wb_project_members m JOIN wb_users u ON u.id=m.user_id WHERE m.project_id=$1 AND m.user_id=$2 AND NOT u.disabled", pid, p.ID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return role, err
}
func (s *Service) RequireProjectRole(ctx context.Context, pid, minimum string) error {
	role, err := s.ProjectRole(ctx, pid)
	if err != nil {
		return err
	}
	if roleLevel(role) < roleLevel(minimum) {
		return ErrForbidden
	}
	return nil
}
func ownerForNewProjectTx(ctx context.Context, tx pgx.Tx, pid string) error {
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		_, err := tx.Exec(ctx, "INSERT INTO wb_project_members(project_id,user_id,role) VALUES($1,$2,'owner')", pid, p.ID)
		return err
	}
	return nil
}
func (s *Service) Members(ctx context.Context, pid string) ([]Member, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT m.user_id,u.username,u.display_name,m.role,m.joined_at FROM wb_project_members m JOIN wb_users u ON u.id=m.user_id WHERE m.project_id=$1 ORDER BY m.joined_at,m.user_id", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var v Member
		if err = rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.Role, &v.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) AddMember(ctx context.Context, pid, username, role string) ([]Member, error) {
	if err := s.RequireProjectRole(ctx, pid, "owner"); err != nil {
		return nil, err
	}
	if !validRole(role) {
		return nil, fmt.Errorf("成员角色无效")
	}
	name, err := normalizedUsername(username)
	if err != nil {
		return nil, err
	}
	var uid string
	if err = s.DB.QueryRow(ctx, "SELECT id FROM wb_users WHERE username=$1 AND NOT disabled", name).Scan(&uid); err != nil {
		return nil, noRows(err)
	}
	_, err = s.DB.Exec(ctx, "INSERT INTO wb_project_members(project_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", pid, uid, role)
	if err != nil {
		return nil, err
	}
	return s.Members(ctx, pid)
}
func (s *Service) ChangeMember(ctx context.Context, pid, uid, role string, remove bool) error {
	if err := s.RequireProjectRole(ctx, pid, "owner"); err != nil {
		return err
	}
	if !remove && !validRole(role) {
		return fmt.Errorf("成员角色无效")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = scanProject(tx.QueryRow(ctx, "SELECT "+projectColumns+" FROM wb_projects WHERE id=$1 FOR UPDATE", pid)); err != nil {
		return err
	}
	var current string
	if err = tx.QueryRow(ctx, "SELECT role FROM wb_project_members WHERE project_id=$1 AND user_id=$2", pid, uid).Scan(&current); err != nil {
		return noRows(err)
	}
	if current == "owner" && (remove || role != "owner") {
		var n int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM wb_project_members WHERE project_id=$1 AND role='owner'", pid).Scan(&n); err != nil {
			return err
		}
		if n <= 1 {
			return &Conflict{Message: "请先指定另一位所有者，再移除最后一位所有者"}
		}
	}
	if remove {
		_, err = tx.Exec(ctx, "DELETE FROM wb_project_members WHERE project_id=$1 AND user_id=$2", pid, uid)
		if err == nil {
			_, err = tx.Exec(ctx, "DELETE FROM wb_presence WHERE project_id=$1 AND user_id=$2", pid, uid)
		}
	} else {
		_, err = tx.Exec(ctx, "UPDATE wb_project_members SET role=$3 WHERE project_id=$1 AND user_id=$2", pid, uid, role)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) CreateInvitation(ctx context.Context, pid, role string) (Invitation, error) {
	if err := s.RequireProjectRole(ctx, pid, "owner"); err != nil {
		return Invitation{}, err
	}
	if !validRole(role) {
		return Invitation{}, fmt.Errorf("邀请角色无效")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Invitation{}, err
	}
	v := Invitation{ID: job.NewID(), ProjectID: pid, Role: role, Token: hex.EncodeToString(raw)}
	p, _ := PrincipalFromContext(ctx)
	err := s.DB.QueryRow(ctx, "INSERT INTO wb_invitations(id,project_id,role,token_hash,created_by,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '7 days') RETURNING expires_at,created_at", v.ID, pid, role, job.Hash([]byte(v.Token)), p.ID).Scan(&v.ExpiresAt, &v.CreatedAt)
	return v, err
}
func (s *Service) Invitations(ctx context.Context, pid string) ([]Invitation, error) {
	if err := s.RequireProjectRole(ctx, pid, "owner"); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,project_id,role,expires_at,accepted_by,revoked,created_at FROM wb_invitations WHERE project_id=$1 ORDER BY created_at DESC", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var v Invitation
		if err = rows.Scan(&v.ID, &v.ProjectID, &v.Role, &v.ExpiresAt, &v.AcceptedBy, &v.Revoked, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) RevokeInvitation(ctx context.Context, pid, id string) error {
	if err := s.RequireProjectRole(ctx, pid, "owner"); err != nil {
		return err
	}
	n, e := s.DB.Exec(ctx, "UPDATE wb_invitations SET revoked=true WHERE id=$1 AND project_id=$2", id, pid)
	if e == nil && n.RowsAffected() != 1 {
		return ErrNotFound
	}
	return e
}
func (s *Service) AcceptInvitation(ctx context.Context, token, username, password, displayName string) (User, error) {
	if len(token) != 64 {
		return User{}, fmt.Errorf("邀请已失效或不存在")
	}
	name, err := normalizedUsername(username)
	if err != nil {
		return User{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var iid, pid, role string
	err = tx.QueryRow(ctx, "SELECT id,project_id,role FROM wb_invitations WHERE token_hash=$1 AND expires_at>now() AND accepted_by='' AND NOT revoked FOR UPDATE", job.Hash([]byte(token))).Scan(&iid, &pid, &role)
	if err != nil {
		return User{}, fmt.Errorf("邀请已失效或不存在")
	}
	var hash string
	var u User
	err = tx.QueryRow(ctx, "SELECT id,username,display_name,disabled,created_at,password_hash FROM wb_users WHERE username=$1", name).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Disabled, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, err = passwordHash(password)
		if err != nil {
			return User{}, err
		}
		if !validName(displayName) {
			displayName = name
		}
		u, err = scanUser(tx.QueryRow(ctx, "INSERT INTO wb_users(id,username,display_name,password_hash) VALUES($1,$2,$3,$4) RETURNING id,username,display_name,disabled,created_at", job.NewID(), name, displayName, hash))
	} else if err == nil && (u.Disabled || !verifyPassword(hash, password)) {
		return User{}, fmt.Errorf("已有账号的密码错误")
	}
	if err != nil {
		return User{}, err
	} // An invitation cannot silently demote an existing project owner.
	_, err = tx.Exec(ctx, "INSERT INTO wb_project_members(project_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", pid, u.ID, role)
	if err != nil {
		return User{}, err
	}
	_, err = tx.Exec(ctx, "UPDATE wb_invitations SET accepted_by=$2 WHERE id=$1", iid, u.ID)
	if err != nil {
		return User{}, err
	}
	return u, tx.Commit(ctx)
}
func (s *Service) UpdatePresence(ctx context.Context, pid string, in PresenceInput) ([]Presence, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return nil, err
	}
	p, ok := PrincipalFromContext(ctx)
	if !ok {
		return nil, ErrForbidden
	}
	if in.Cursor != nil && (*in.Cursor < 0 || *in.Cursor > 10000000) {
		return nil, fmt.Errorf("光标位置无效")
	}
	if in.ChapterID != "" {
		var found bool
		if e := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_chapters WHERE id=$1 AND project_id=$2 AND NOT deleted)", in.ChapterID, pid).Scan(&found); e != nil {
			return nil, e
		}
		if !found {
			return nil, ErrNotFound
		}
	}
	if in.RevisionID != "" {
		var found bool
		if e := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_revisions r JOIN wb_chapters c ON c.id=r.chapter_id WHERE r.id=$1 AND c.id=$2 AND c.project_id=$3)", in.RevisionID, in.ChapterID, pid).Scan(&found); e != nil {
			return nil, e
		}
		if !found {
			return nil, ErrNotFound
		}
	}
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_presence(project_id,user_id,username,display_name,chapter_id,revision_id,cursor_position) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(project_id,user_id) DO UPDATE SET chapter_id=excluded.chapter_id,revision_id=excluded.revision_id,cursor_position=excluded.cursor_position,seen_at=now(),display_name=excluded.display_name", pid, p.ID, p.Username, p.DisplayName, in.ChapterID, in.RevisionID, in.Cursor)
	if err != nil {
		return nil, err
	}
	return s.Presence(ctx, pid)
}
func (s *Service) Presence(ctx context.Context, pid string) ([]Presence, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT user_id,username,display_name,chapter_id,revision_id,cursor_position,seen_at FROM wb_presence WHERE project_id=$1 AND seen_at>now()-interval '60 seconds' ORDER BY seen_at DESC", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Presence{}
	for rows.Next() {
		var v Presence
		if err = rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.ChapterID, &v.RevisionID, &v.Cursor, &v.SeenAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) CollaborationEvents(ctx context.Context, pid string, after int64) ([]CollaborationEvent, error) {
	if err := s.RequireProjectRole(ctx, pid, "viewer"); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,project_id,kind,object_id,created_at FROM wb_collab_events WHERE project_id=$1 AND id>$2 ORDER BY id LIMIT 100", pid, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CollaborationEvent{}
	for rows.Next() {
		var v CollaborationEvent
		if err = rows.Scan(&v.ID, &v.ProjectID, &v.Kind, &v.ObjectID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
