package workbench

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestRetrievalVisibilityKnowledgeAndRebuildIsolation(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c1 := f.chapter(t, "第一章", "林川持有铜印。沈月知道铜印来自王都司钥署，林川只知道与王都有关。")
	p := f.proposal(t, c1, []MemoryChange{
		f.change(t, c1, "Ownership", f.seal.ID, "owner", f.lin.ID, "", "True", "林川持有铜印"),
		f.change(t, c1, "Knowledge", f.secret.ID, "origin", "铜印来自王都司钥署", f.shen.ID, "True", "沈月知道铜印来自王都司钥署"),
		f.change(t, c1, "Knowledge", f.secret.ID, "origin", "铜印与王都有关", f.lin.ID, "True", "沈月知道铜印来自王都司钥署，林川只知道与王都有关"),
	})
	f.apply(t, p)
	c2 := f.chapter(t, "第二章", "铜印在第二章才转交沈月。")
	f.apply(t, f.proposal(t, c2, []MemoryChange{f.change(t, c2, "Ownership", f.seal.ID, "owner", f.shen.ID, "", "True", "铜印在第二章才转交沈月")}))
	query := Query{ProjectID: f.p.ID, BeforeChapterID: c2.ID, Text: "铜印", Purpose: "AuthorReview", Limit: 10}
	results, err := f.s.Search(ctx, query)
	if err != nil || len(results) != 1 || results[0].ChapterID != c1.ID {
		t.Fatalf("future chapter leaked %#v %v", results, err)
	}
	query.Purpose = "CharacterKnowledge"
	query.CharacterID = f.lin.ID
	query.Text = "司钥署"
	results, err = f.s.Search(ctx, query)
	if err != nil || len(results) != 0 {
		t.Fatalf("narrator secret leaked %#v %v", results, err)
	}
	query.CharacterID = f.shen.ID
	results, err = f.s.Search(ctx, query)
	if err != nil || len(results) != 1 || results[0].Kind != "Knowledge" {
		t.Fatalf("known secret missing %#v %v", results, err)
	}
	query.CharacterID = f.lin.ID
	query.Text = "王都"
	results, err = f.s.Search(ctx, query)
	if err != nil || len(results) != 1 || strings.Contains(results[0].Content, "司钥署") {
		t.Fatalf("character projection leaked %#v %v", results, err)
	}
	projection, err := f.s.ConfirmedContext(ctx, f.p.ID, c2.ID, f.lin.ID)
	if err != nil || strings.Contains(projection, "司钥署") {
		t.Fatalf("character evidence leaked narrator truth %q %v", projection, err)
	}
	other, err := f.s.CreateProject(ctx, "隔离作品", "", "")
	if err != nil {
		t.Fatal(err)
	}
	results, err = f.s.Search(ctx, Query{ProjectID: other.ID, Text: "铜印"})
	if err != nil || len(results) != 0 {
		t.Fatalf("other work leaked %#v %v", results, err)
	}
	if _, err = f.s.Search(ctx, Query{ProjectID: other.ID, Text: "铜印", BeforeChapterID: c1.ID}); err == nil {
		t.Fatal("foreign boundary accepted")
	}
	draft, err := f.s.SaveRevision(ctx, c1.ID, SaveInput{ExpectedCurrentRevisionID: c1.CurrentRevisionID, Document: DocumentFromText("铜印已经熔毁。")})
	if err != nil {
		t.Fatal(err)
	}
	results, err = f.s.Search(ctx, Query{ProjectID: f.p.ID, Text: "熔毁"})
	if err != nil || len(results) != 0 {
		t.Fatalf("unadopted draft indexed %#v %v", results, err)
	}
	if _, err = f.s.Finalize(ctx, c1.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	results, err = f.s.Search(ctx, Query{ProjectID: f.p.ID, Text: "铜印"})
	if err != nil || len(results) != 0 {
		t.Fatalf("invalidated versions leaked %#v %v", results, err)
	}
	if err = f.s.Reindex(ctx, f.p.ID); err != nil {
		t.Fatal(err)
	}
	results, err = f.s.Search(ctx, Query{ProjectID: f.p.ID, Text: "铜印"})
	if err != nil || len(results) != 0 {
		t.Fatalf("reindex resurrected invalidated sources %#v %v", results, err)
	}
}

func TestRetrievalChunkingAndReindexIdempotence(t *testing.T) {
	text := strings.Repeat("铜印的旧事需要原文证据。", 150)
	parts := chunks(text + "\n\n林川仍留在磨坊。")
	if len(parts) < 3 {
		t.Fatal("long paragraphs were not split")
	}
	for _, part := range parts {
		if len([]rune(part)) > 500 {
			t.Fatal("chunk exceeded rune cap")
		}
	}
	if strings.Join(parts, "") != text+"林川仍留在磨坊。" {
		t.Fatal("chunking lost source text")
	}
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", text)
	f.apply(t, f.proposal(t, c, []MemoryChange{}))
	var before, after int
	if err := f.s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_chunks WHERE project_id=$1", f.p.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := f.s.Reindex(ctx, f.p.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_chunks WHERE project_id=$1", f.p.ID).Scan(&after); err != nil || before != after {
		t.Fatalf("reindex duplicate chunks %d %d %v", before, after, err)
	}
	results, err := f.s.Search(ctx, Query{ProjectID: f.p.ID, Text: "木箱里面的地图"})
	if err != nil || len(results) != 0 {
		t.Fatalf("no answer manufactured %#v %v", results, err)
	}
}

func TestNarrativeArchiveRoundtripPreservesMappedEvidence(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", "林川左臂受伤，铜印仍在内袋。")
	f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "林川左臂受伤")}))
	archive, err := f.s.ExportProject(ctx, f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.ImportProject(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.s.State(ctx, restored.Project.ID)
	if err != nil || len(state.Facts) != 1 {
		t.Fatalf("restored state %#v %v", state, err)
	}
	fact := state.Facts[0]
	if fact.ProjectID != restored.Project.ID || fact.ChapterID == c.ID || fact.SourceRevisionID == c.FinalRevisionID || fact.Subject == f.lin.ID {
		t.Fatalf("IDs not remapped %#v", fact)
	}
	source, err := f.s.GetRevision(ctx, fact.SourceRevisionID)
	if err != nil || source.Hash != fact.Evidence.PlainTextHash || !strings.Contains(source.PlainText, fact.Evidence.Quote) {
		t.Fatalf("evidence no longer resolves %#v %v", source, err)
	}
	citations, err := f.s.Search(ctx, Query{ProjectID: restored.Project.ID, Text: "铜印"})
	if err != nil || len(citations) != 1 || citations[0].ProjectID != restored.Project.ID {
		t.Fatalf("restored index %#v %v", citations, err)
	}
	var raw narrativeArchive
	if err = json.Unmarshal(archive.Narrative, &raw); err != nil {
		t.Fatal(err)
	}
	raw.Facts[0].Evidence.Quote = "虚构的证据"
	archive.Narrative, _ = json.Marshal(raw)
	if _, err = f.s.ImportProject(ctx, archive); err == nil {
		t.Fatal("corrupt evidence archive accepted")
	}
}

func TestNarrativeArchiveDoesNotPromotePendingOrInvalidMemory(t *testing.T) {
	f := newNarrativeFixture(t)
	ctx := context.Background()
	c := f.chapter(t, "第一章", "林川左臂受伤。")
	archive, err := f.s.ExportProject(ctx, f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.ImportProject(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	state, err := f.s.State(ctx, restored.Project.ID)
	if err != nil || state.Health != "MemoryPending" || len(state.PendingChapterIDs) != 1 {
		t.Fatalf("pending memory became ready %#v %v", state, err)
	}
	f.apply(t, f.proposal(t, c, []MemoryChange{f.change(t, c, "Injury", f.lin.ID, "leftArm", "受伤", "", "True", "左臂受伤")}))
	revision, err := f.s.SaveRevision(ctx, c.ID, SaveInput{ExpectedCurrentRevisionID: c.CurrentRevisionID, Document: DocumentFromText("林川右臂受伤。")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Finalize(ctx, c.ID, revision.ID); err != nil {
		t.Fatal(err)
	}
	archive, err = f.s.ExportProject(ctx, f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err = f.s.ImportProject(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	state, err = f.s.State(ctx, restored.Project.ID)
	if err != nil || state.Health == "Ready" || len(state.Facts) != 0 {
		t.Fatalf("invalid memory became ready %#v %v", state, err)
	}
}
