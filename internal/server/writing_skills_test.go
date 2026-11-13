package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"novel/internal/workbench"
	"strings"
	"testing"
)

const httpWritingSkill = "---\nname: sensory-prose\ndescription: 具体物象和人物动作\nlicense: MIT\n---\n先核对人物想达到什么，再用可观察细节替代空泛结论。"

func TestWritingSkillHTTPPreviewCommitExportVersionAndCSRF(t *testing.T) {
	s := testServer(t)
	session := loginForTest(t, s)
	project := decodeResponse[workbench.Project](t, request(s, "POST", "/api/projects", `{"title":"技能HTTP","description":"","genre":"悬疑"}`, session))
	base := "/api/projects/" + project.ID + "/writing-skills"
	catalog := request(s, "GET", "/api/writing-skill-catalog", "", session)
	if catalog.Code != 200 || !strings.Contains(catalog.Body.String(), "builtin") {
		t.Fatal("catalog not available", catalog.Code)
	}
	input := workbench.WritingSkillPreviewInput{Format: "markdown", Content: httpWritingSkill}
	preview := decodeResponse[workbench.SkillPreview](t, request(s, "POST", base+"/preview", jsonBody(input), session))
	if preview.Name != "sensory-prose" {
		t.Fatal("preview missing metadata")
	}
	list := decodeResponse[workbench.WritingSkillList](t, request(s, "GET", base, "", session))
	if len(list.Items) != 0 {
		t.Fatal("preview changed library")
	}
	saved := decodeResponse[workbench.WritingSkill](t, request(s, "POST", base, jsonBody(workbench.WritingSkillSave{Preview: preview, Enabled: true, Priority: 5}), session))
	if saved.Version != 1 || !saved.Enabled {
		t.Fatal("confirmation did not save", saved)
	}
	badSession := session
	badSession.CSRF = "wrong"
	if response := request(s, "PUT", base+"/"+saved.ID, `{"expectedVersion":1,"enabled":false}`, badSession); response.Code != 403 {
		t.Fatal("missing csrf accepted", response.Code)
	}
	response := request(s, "GET", base+"/"+saved.ID+"/export", "", session)
	exported := decodeResponse[workbench.SkillPreview](t, response)
	if exported.Content != httpWritingSkill || !strings.Contains(response.Header().Get("Content-Disposition"), "skill.json") {
		t.Fatal("bundle export wrong")
	}
	markdown := request(s, "GET", base+"/"+saved.ID+"/export?format=markdown", "", session)
	if markdown.Code != 200 || markdown.Body.String() != httpWritingSkill || !strings.Contains(markdown.Header().Get("Content-Type"), "text/markdown") {
		t.Fatal("markdown export wrong")
	}
	changed := decodeResponse[workbench.WritingSkill](t, request(s, "PUT", base+"/"+saved.ID, `{"expectedVersion":1,"enabled":false}`, session))
	if changed.Version != 2 || changed.Enabled {
		t.Fatal("disable version wrong")
	}
	if stale := request(s, "PUT", base+"/"+saved.ID, `{"expectedVersion":1,"enabled":true}`, session); stale.Code != 409 {
		t.Fatal("stale mutation accepted", stale.Code)
	}
	if bad := request(s, "DELETE", base+"/"+saved.ID, "", session); bad.Code != 400 {
		t.Fatal("delete without version accepted")
	}
	if stale := request(s, "DELETE", base+"/"+saved.ID+"?expectedVersion=1", "", session); stale.Code != 409 {
		t.Fatal("stale delete accepted", stale.Code)
	}
	if removed := request(s, "DELETE", base+"/"+saved.ID+"?expectedVersion=2", "", session); removed.Code != 200 {
		t.Fatal("delete failed", removed.Code, removed.Body.String())
	}
	if read := request(s, "GET", base+"/"+saved.ID+"/export", "", session); read.Code != 404 {
		t.Fatal("deleted skill exported")
	}
	if read := request(s, "GET", base, "", testLogin{}); read.Code != 401 {
		t.Fatal("anonymous skill library access")
	}
}
func TestWritingSkillHTTPViewerCanReadButCannotImportOrMutateOtherProject(t *testing.T) {
	s := testServer(t)
	admin := loginForTest(t, s)
	ctx := context.Background()
	p, err := s.Service.CreateProject(ctx, "共享技能", "", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.Service.CreateProject(ctx, "隔离作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.Service.CreateUser(ctx, "writing-viewer", "viewer-password-2026", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Service.DB.Exec(ctx, "INSERT INTO wb_project_members(project_id,user_id,role) VALUES($1,$2,'viewer')", p.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	login := request(s, "POST", "/api/session", `{"username":"writing-viewer","password":"viewer-password-2026"}`, testLogin{})
	if login.Code != 200 {
		t.Fatal(login.Code)
	}
	var info struct {
		CSRF string `json:"csrfToken"`
	}
	json.Unmarshal(login.Body.Bytes(), &info)
	viewer := testLogin{Cookie: login.Result().Cookies()[0], CSRF: info.CSRF}
	base := "/api/projects/" + p.ID + "/writing-skills"
	if r := request(s, "GET", base, "", viewer); r.Code != 200 {
		t.Fatal("viewer could not read", r.Code)
	}
	for _, path := range []string{base + "/preview", base} {
		r := request(s, "POST", path, `{"format":"markdown","content":"x"}`, viewer)
		if r.Code != 403 {
			t.Fatal("viewer mutating Skill", r.Code)
		}
	}
	if r := request(s, "GET", "/api/projects/"+foreign.ID+"/writing-skills", "", viewer); r.Code != 403 {
		t.Fatal("foreign skill access", r.Code)
	}
	if r := request(s, "GET", "/api/writing-skill-catalog", "", viewer); r.Code != 200 {
		t.Fatal("authenticated catalog denied", r.Code)
	}
	preview := decodeResponse[workbench.SkillPreview](t, request(s, "POST", base+"/preview", jsonBody(workbench.WritingSkillPreviewInput{Content: httpWritingSkill}), admin))
	saved := decodeResponse[workbench.WritingSkill](t, request(s, "POST", base, jsonBody(workbench.WritingSkillSave{Preview: preview}), admin))
	if r := request(s, "GET", "/api/projects/"+foreign.ID+"/writing-skills/"+saved.ID+"/export", "", admin); r.Code != 404 {
		t.Fatal("foreign export leaked", r.Code)
	}
	_ = httptest.NewRecorder()
}
