package workbench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"novel/internal/job"
	"novel/internal/llm"
)

func TestCopilotWritingProfilePagesFullBookStyleAndRejectsPOV(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := copilotAdmin()
	brief := StoryBrief{Style: strings.Repeat("具体动作，克制叙述。", 450), Pace: strings.Repeat("快慢随场景而变。", 300), Restrictions: "秘密限制-不得向人物泄露真凶身份。"}
	saved, err := s.SaveStudioBrief(ctx, c.ProjectID, StudioWrite[StoryBrief]{ExpectedVersion: 0, Data: brief})
	if err != nil {
		t.Fatal(err)
	}
	real, err := s.SaveSetting(ctx, c.ProjectID, Setting{Kind: "style", Name: "手动写法", Content: "对白少解释。"})
	if err != nil {
		t.Fatal(err)
	}
	scope := copilotScope(t, s, c, "")
	list, err := s.ExecuteCopilotTool(ctx, scope, "read_settings", []byte(`{"kind":"style"}`))
	if err != nil {
		t.Fatal(err)
	}
	items := list.Content.(map[string]any)["settings"].([]map[string]any)
	if len(items) != 2 || items[0]["id"] != bookWritingProfileID || items[1]["id"] != real.ID {
		t.Fatal("book profile missing from style settings", items)
	}
	var full strings.Builder
	offset := 0
	for {
		page, err := s.ExecuteCopilotTool(ctx, scope, "read_settings", copilotArgs(map[string]any{"id": bookWritingProfileID, "contentOffset": offset}))
		if err != nil {
			t.Fatal(err)
		}
		entries := page.Content.(map[string]any)["settings"].([]map[string]any)
		if len(entries) != 1 {
			t.Fatal("profile page missing")
		}
		item := entries[0]
		text := item["content"].(string)
		if item["version"] != saved.Version || item["nextContentOffset"].(int) != offset+len([]rune(text)) || len([]rune(text)) > 2000 {
			t.Fatal("profile version or cursor mismatch", item)
		}
		full.WriteString(text)
		offset = item["nextContentOffset"].(int)
		if offset == item["totalChars"].(int) {
			break
		}
	}
	expected := "文风与表达：\n" + brief.Style + "\n\n节奏：\n" + brief.Pace + "\n\n避免事项：\n" + brief.Restrictions
	if full.String() != expected {
		t.Fatal("profile pagination lost a field")
	}
	profile, err := loadCopilotWritingProfile(ctx, s.DB, c.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	directory := profile.directory().(map[string]any)
	if len([]rune(directory["preview"].(string))) > 24 || directory["truncated"] != true {
		t.Fatal("startup profile not bounded", directory)
	}
	pov := scope
	pov.PointOfViewCharacterID = "character-private"
	if _, err = s.ExecuteCopilotTool(ctx, pov, "read_settings", copilotArgs(map[string]string{"id": bookWritingProfileID})); err != ErrForbidden {
		t.Fatal("POV read omniscient writing profile", err)
	}
	for _, query := range []string{"", brief.Restrictions} {
		result, err := s.ExecuteCopilotTool(ctx, pov, "read_settings", copilotArgs(map[string]string{"kind": "style", "query": query}))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(result.Content)
		if strings.Contains(string(raw), "秘密限制") || strings.Contains(string(raw), bookWritingProfileID) {
			t.Fatal("POV profile content/directory leaked", string(raw))
		}
	}
	brief.Style = "文风已经更新。"
	if _, err = s.SaveStudioBrief(ctx, c.ProjectID, StudioWrite[StoryBrief]{ExpectedVersion: saved.Version, Data: brief}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ExecuteCopilotTool(ctx, scope, "read_settings", copilotArgs(map[string]string{"id": bookWritingProfileID})); !IsConflict(err) {
		t.Fatal("stale style profile source accepted", err)
	}
}

func TestCopilotWritingProfileActualPiReadsCompleteBeforeWriting(t *testing.T) {
	node, _ := filepath.Abs("../../.tools/node-v24.22.3-win-x64/node.exe")
	entry, _ := filepath.Abs("../../harness/dist/main.js")
	if _, err := os.Stat(node); err != nil {
		t.Skip("Node runtime unavailable")
	}
	if _, err := os.Stat(entry); err != nil {
		t.Skip("Build Pi harness")
	}
	t.Setenv("HARNESS_NODE_PATH", node)
	t.Setenv("HARNESS_ENTRY_PATH", entry)
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	_, err := s.SaveStudioBrief(ctx, c.ProjectID, StudioWrite[StoryBrief]{Data: StoryBrief{Style: strings.Repeat("动作代替解释。", 310), Pace: "对峙时短句，场景切换留白。", Restrictions: "避免段尾总结。"}})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "请按照本书写法改写得更含蓄"})
	if err != nil {
		t.Fatal(err)
	}
	turn = runningCopilot(t, s, turn)
	if err = s.ExecuteCopilotTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	done, err := s.GetCopilotTurn(ctx, turn.ID)
	if err != nil || done.Status != "Succeeded" {
		t.Fatal(done.Status, done.Error, err)
	}
	events, err := s.CopilotEvents(ctx, turn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	var readContent strings.Builder
	for _, event := range events {
		if event.Kind == "tool-start" {
			var v struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(event.Data, &v)
			names = append(names, v.Name)
		}
		if event.Kind == "tool-result" {
			var v struct {
				Name   string `json:"name"`
				Result struct {
					Content struct {
						Settings []struct {
							Content string `json:"content"`
						} `json:"settings"`
					} `json:"content"`
				} `json:"result"`
			}
			_ = json.Unmarshal(event.Data, &v)
			if v.Name == "read_settings" {
				for _, item := range v.Result.Content.Settings {
					readContent.WriteString(item.Content)
				}
			}
		}
	}
	if strings.Join(names, ",") != "read_settings,read_settings,read_chapter,create_revision_candidate,show_diff" {
		t.Fatal("Pi did not read writing profile before source/write", names)
	}
	if !strings.Contains(readContent.String(), "避免段尾总结。") || !strings.Contains(readContent.String(), "对峙时短句") {
		t.Fatal("Pi never received latter profile fields")
	}
}

func TestCandidateIterationNormalRewriteParentExpandsOriginalManuscript(t *testing.T) {
	s, c, session := copilotFixture(t)
	ctx := copilotAdmin()
	original := "原稿开头：铜印在右手。\n\n这是需要改写的一段。\n\n原稿结尾：左臂伤势尚未恢复。"
	revision, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText(original), Source: "author"})
	if err != nil {
		t.Fatal(err)
	}
	c.CurrentRevisionID = revision.ID
	s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
		return llm.Response{Text: "这段被改为克制的对白。"}, nil
	}}
	run, err := s.SubmitRun(ctx, c.ID, RunRequest{IntentID: job.NewID(), TaskType: "rewrite", Selection: "这是需要改写的一段。", Operation: "polish"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := s.ClaimRun(ctx, "profile-rewrite-test")
	if err != nil || !ok {
		t.Fatal("claim", err)
	}
	if err = s.ExecuteRun(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	run, err = s.GetRun(ctx, run.ID)
	if err != nil || run.Status != "Succeeded" {
		t.Fatal(run.Status, run.Error, err)
	}
	if run.Output.Text != "这段被改为克制的对白。" {
		t.Fatal("single-shot output contract changed", run.Output.Text)
	}
	expected := strings.Replace(original, "这是需要改写的一段。", run.Output.Text, 1)
	turn, err := s.SubmitCopilotTurn(ctx, session.ID, CopilotTurnRequest{IntentID: job.NewID(), Text: "只对候选这段继续打磨，保留未选部分", CandidateRunID: run.ID, CandidateSelection: run.Output.Text})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Input.Candidate.TextHash != job.Hash([]byte(expected)) {
		t.Fatal("rewrite parent hash is partial, not whole manuscript")
	}
	turn = runningCopilot(t, s, turn)
	scope := iterationScope(t, s, turn)
	page, err := s.ExecuteCopilotTool(ctx, scope, "read_revision_candidate", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if page.Content.(map[string]any)["text"] != expected {
		t.Fatal("rewrite parent not expanded against frozen original")
	}
	child, err := s.ExecuteCopilotTool(ctx, scope, "create_revision_candidate", []byte(`{"mode":"selection","text":"她没有继续解释。"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(child.Artifact.Text, "原稿开头：铜印在右手。") || !strings.HasSuffix(child.Artifact.Text, "原稿结尾：左臂伤势尚未恢复。") || strings.Contains(child.Artifact.Text, "这是需要") {
		t.Fatal("ordinary rewrite iteration lost untouched manuscript", child.Artifact.Text)
	}
	parent, err := s.GetRun(ctx, run.ID)
	if err != nil || parent.Output.Text != run.Output.Text {
		t.Fatal("rewrote persisted partial output", err)
	}
	if err = s.finishCopilot(ctx, turn, "Succeeded", "", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	applied, err := s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: revision.ID, Source: "ai", RunID: child.Artifact.RunID, Document: DocumentFromText(child.Artifact.Text)})
	if err != nil || !strings.HasSuffix(applied.PlainText, "原稿结尾：左臂伤势尚未恢复。") {
		t.Fatal("whole chapter adoption lost trailing original", applied, err)
	}
}
