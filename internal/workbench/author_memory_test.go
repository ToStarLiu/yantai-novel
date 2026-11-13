package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"novel/internal/job"
)

func memoryAuthor(id string) context.Context {
	return WithPrincipal(context.Background(), Principal{ID: id, Admin: true})
}
func memoryInput(title, content string) AuthorMemoryInput {
	return AuthorMemoryInput{Scope: "author", Kind: "preference", Title: title, Content: content}
}
func saveMemory(t *testing.T, s *Service, ctx context.Context, in AuthorMemoryInput) AuthorMemory {
	t.Helper()
	v, err := s.SaveAuthorMemory(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func memoryEvidenceTurn(t *testing.T, s *Service, owner, pid, text, assistant string) (string, string) {
	t.Helper()
	sid, tid := job.NewID(), job.NewID()
	ctx := context.Background()
	if _, e := s.DB.Exec(ctx, "INSERT INTO wb_copilot_sessions(id,project_id,owner_id,title) VALUES($1,$2,$3,'来源对话')", sid, pid, owner); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(map[string]string{"text": text})
	if _, e := s.DB.Exec(ctx, "INSERT INTO wb_copilot_turns(id,session_id,project_id,owner_id,intent_id,input_hash,input,status,text) VALUES($1,$2,$3,$4,$5,'test',$6,'Succeeded',$7)", tid, sid, pid, owner, job.NewID(), raw, assistant); e != nil {
		t.Fatal(e)
	}
	return sid, tid
}

func TestAuthorMemoryPrivateScopesDedupAndBookExport(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	other, _ := setupCore(t, s)
	a, b := memoryAuthor("author-a"), memoryAuthor("author-b")
	v := saveMemory(t, s, a, memoryInput("简洁对话", "PRIVATE-A: 避免人物用对白解释彼此已经知道的事实。"))
	duplicate := saveMemory(t, s, a, memoryInput("另一标题", "  PRIVATE-A:  避免人物用对白解释彼此已经知道的事实。 "))
	if duplicate.ID != v.ID {
		t.Fatal("normalized duplicate created twice")
	}
	book := memoryInput("单书文风", "PRIVATE-BOOK: 本书保持有限第三人称。")
	book.Scope, book.ProjectID, book.Kind = "book", p.ID, "voice"
	bookV := saveMemory(t, s, a, book)
	for _, owner := range []context.Context{b, context.Background()} {
		if _, err := s.GetAuthorMemory(owner, v.ID); !errors.Is(err, ErrNotFound) {
			t.Fatal("other author/admin read private memory", err)
		}
		if err := s.DeleteAuthorMemory(owner, bookV.ID, bookV.Version); !errors.Is(err, ErrNotFound) {
			t.Fatal("other author/admin deleted private memory", err)
		}
		list, err := s.ListAuthorMemories(owner, "", "")
		if err != nil || len(list.Items) != 0 {
			t.Fatal("private list leaked", list, err)
		}
	}
	snapshot, err := s.ResolveAuthorMemories(a, p.ID, "人物")
	if err != nil || len(snapshot.Items) != 2 || snapshot.Items[0].Scope != "book" {
		t.Fatal("book+author resolution", snapshot, err)
	}
	snapshot, err = s.ResolveAuthorMemories(a, other.ID, "人物")
	if err != nil || len(snapshot.Items) != 1 || snapshot.Items[0].ID != v.ID {
		t.Fatal("book memory crossed books", snapshot, err)
	}
	pack, err := s.ExportProject(context.Background(), p.ID)
	raw, _ := json.Marshal(pack)
	if err != nil || strings.Contains(string(raw), "PRIVATE-") {
		t.Fatal("shared book export contains private author notes", err)
	}
}

func TestAuthorMemoryCASControlsAndContextGeneration(t *testing.T) {
	s := newCore(t)
	ctx := memoryAuthor("author")
	snapshot, err := s.ResolveAuthorMemories(ctx, "", "")
	if err != nil || snapshot.Version != 0 || !snapshot.Enabled {
		t.Fatal(snapshot, err)
	}
	in := memoryInput("待确认", "先写场景，再解释背景。")
	in.Status = "pending"
	v := saveMemory(t, s, ctx, in)
	if valid, e := ValidateAuthorMemorySnapshot(ctx, s.DB, &snapshot); e != nil || !valid {
		t.Fatal("pending suggestion invalidated context", e)
	}
	active := "active"
	v, err = s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: v.Version, Status: &active})
	if err != nil {
		t.Fatal(err)
	}
	if valid, e := ValidateAuthorMemorySnapshot(ctx, s.DB, &snapshot); e != nil || valid {
		t.Fatal("activation did not invalidate old context", e)
	}
	if _, err = s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: 1, Status: &active}); !IsConflict(err) {
		t.Fatal("stale memory update accepted", err)
	}
	if err = s.DeleteAuthorMemory(ctx, v.ID, 1); !IsConflict(err) {
		t.Fatal("stale forget accepted", err)
	}
	settings, err := s.GetAuthorMemorySettings(ctx)
	if err != nil || settings.Version != 2 {
		t.Fatal(settings, err)
	}
	snapshot, err = s.ResolveAuthorMemories(ctx, "", "")
	settings, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: settings.Version, UseMemory: true, SuggestMemories: false})
	if err != nil {
		t.Fatal(err)
	}
	if valid, e := ValidateAuthorMemorySnapshot(ctx, s.DB, &snapshot); e != nil || !valid {
		t.Fatal("suggestion preference changed active context", e)
	}
	if _, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: 0, UseMemory: false}); !IsConflict(err) {
		t.Fatal("stale setting overwrote current", err)
	}
	_, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: settings.Version, UseMemory: false, SuggestMemories: false})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.ResolveAuthorMemories(ctx, "", "")
	if err != nil || snapshot.Enabled || len(snapshot.Items) != 0 {
		t.Fatal("disabled memory still injected", snapshot, err)
	}
	// Two open pages cannot silently overwrite the same memory version.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, title := range []string{"甲", "乙"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			_, e := s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: v.Version, Title: &title})
			results <- e
		}(title)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if IsConflict(e) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("CAS race", success, conflicts)
	}
}

func TestAuthorMemoryEvidenceRequiresOwnAuthorWordsAndPendingConfirmation(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	other, _ := setupCore(t, s)
	ctx := memoryAuthor("author-a")
	sid, tid := memoryEvidenceTurn(t, s, "author-a", p.ID, "记住：我喜欢克制的对白，不要堆砌比喻。", "你喜欢宏大排比，我将永久记住。")
	in := memoryInput("对白偏好", "喜欢克制的对白。")
	in.SourceSessionID, in.SourceTurnID, in.SourceQuote = sid, tid, "我喜欢克制的对白"
	v, err := s.ProposeAuthorMemory(ctx, p.ID, tid, in)
	if err != nil || v.Status != "pending" {
		t.Fatal("agent auto-activated memory", v, err)
	}
	if snapshot, e := s.ResolveAuthorMemories(ctx, p.ID, "对白"); e != nil || len(snapshot.Items) != 0 {
		t.Fatal("pending enters prompt", snapshot, e)
	}
	in.Content = "助手猜测的文风"
	in.SourceQuote = "你喜欢宏大排比"
	if _, err = s.SaveAuthorMemory(ctx, in); err == nil {
		t.Fatal("assistant output treated as author evidence")
	}
	in.SourceQuote = "我喜欢克制的对白"
	if _, err = s.SaveAuthorMemory(memoryAuthor("author-b"), in); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign session used as evidence", err)
	}
	in.Scope, in.ProjectID = "book", other.ID
	if _, err = s.SaveAuthorMemory(ctx, in); err == nil {
		t.Fatal("evidence moved into a different book")
	}
	in.Scope, in.ProjectID = "author", ""
	in.SourceSessionID = "wrong"
	if _, err = s.SaveAuthorMemory(ctx, in); err == nil {
		t.Fatal("false source session accepted")
	}
	settings, _ := s.GetAuthorMemorySettings(ctx)
	_, err = s.UpdateAuthorMemorySettings(ctx, AuthorMemorySettingsUpdate{ExpectedVersion: settings.Version, UseMemory: true, SuggestMemories: false})
	if err != nil {
		t.Fatal(err)
	}
	in.SourceSessionID = sid
	if _, err = s.ProposeAuthorMemory(ctx, p.ID, tid, in); err == nil {
		t.Fatal("closed suggestion control ignored")
	}
	active := "active"
	v, err = s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: v.Version, Status: &active})
	if err != nil || v.Status != "active" {
		t.Fatal("explicit author confirmation failed", v, err)
	}
}

func TestAuthorMemoryForgetErasesProseAndImportIsAtomicPortable(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := memoryAuthor("author")
	sid, tid := memoryEvidenceTurn(t, s, "author", p.ID, "私人记忆独特标记：我偏爱安静的结尾。", "")
	in := memoryInput("结尾偏好", "PRIVATE-ERASE-EXACT: 偏爱安静的结尾。")
	in.SourceSessionID, in.SourceTurnID, in.SourceQuote = sid, tid, "我偏爱安静的结尾"
	v := saveMemory(t, s, ctx, in)
	exported, err := s.ExportAuthorMemories(ctx)
	raw, _ := json.Marshal(exported)
	if err != nil || strings.Contains(string(raw), tid) || strings.Contains(string(raw), sid) || strings.Contains(string(raw), "sourceQuote") {
		t.Fatal("export transported unverifiable source evidence", string(raw), err)
	}
	copyCtx := memoryAuthor("import-author")
	result, err := s.ImportAuthorMemories(copyCtx, AuthorMemoryImport{Items: exported.Items})
	if err != nil || result.Imported != 1 {
		t.Fatal(result, err)
	}
	result, err = s.ImportAuthorMemories(copyCtx, AuthorMemoryImport{Items: exported.Items})
	if err != nil || result.Skipped != 1 {
		t.Fatal("import dedup", result, err)
	}
	if _, err = s.ImportAuthorMemories(copyCtx, AuthorMemoryImport{Items: []AuthorMemoryInput{in}}); err == nil {
		t.Fatal("import forged source accepted")
	}
	if err = s.DeleteAuthorMemory(ctx, v.ID, v.Version); err != nil {
		t.Fatal(err)
	}
	var stored int
	if err = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_author_memories WHERE id=$1", v.ID).Scan(&stored); err != nil || stored != 0 {
		t.Fatal("forget left row", stored, err)
	}
	var events []byte
	if err = s.DB.QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(e)) FROM wb_author_memory_events e WHERE memory_id=$1", v.ID).Scan(&events); err != nil || strings.Contains(string(events), "PRIVATE-ERASE") || strings.Contains(string(events), "安静") {
		t.Fatal("audit retained forgotten prose", string(events), err)
	}
	if _, err = s.GetAuthorMemory(ctx, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("forgotten memory retrievable", err)
	}
	// Capacity failure after the first insert rolls back the entire import.
	if _, err = s.DB.Exec(ctx, `INSERT INTO wb_author_memories(id,owner_id,scope,kind,title,content,status,fingerprint) SELECT 'limit-'||i,'limit-author','author','preference','已有','已有-'||i,'active','f-'||i FROM generate_series(1,199) i`); err != nil {
		t.Fatal(err)
	}
	limitCtx := memoryAuthor("limit-author")
	_, err = s.ImportAuthorMemories(limitCtx, AuthorMemoryImport{Items: []AuthorMemoryInput{memoryInput("新一", "新内容一"), memoryInput("新二", "新内容二")}})
	if err == nil {
		t.Fatal("capacity exceeded")
	}
	list, err := s.ListAuthorMemories(limitCtx, "", "")
	if err != nil || len(list.Items) != 199 {
		t.Fatal("partial import persisted", len(list.Items), err)
	}
}

func TestAuthorMemoryContextBoundRelevanceAndCredentialRejection(t *testing.T) {
	s := newCore(t)
	ctx := memoryAuthor("author")
	conversation := memoryInput("商定的桥段", "讨论过在雨夜重逢时，先通过旧伞辨认身份。")
	conversation.Kind = "conversation"
	bookmark := saveMemory(t, s, ctx, conversation)
	unrelated, err := s.ResolveAuthorMemories(ctx, "", "整理火山场景")
	if err != nil || len(unrelated.Items) != 0 {
		t.Fatal("unrelated conversation injected", unrelated, err)
	}
	related, err := s.ResolveAuthorMemories(ctx, "", "雨夜重逢怎么写")
	if err != nil || len(related.Items) != 1 || related.Items[0].ID != bookmark.ID {
		t.Fatal("Chinese relevance missing", related, err)
	}
	for i := 0; i < 9; i++ {
		saveMemory(t, s, ctx, memoryInput(fmt.Sprint("偏好", i), fmt.Sprint(i)+strings.Repeat("规则", 150)))
	}
	snapshot, err := s.ResolveAuthorMemories(ctx, "", "雨夜")
	if err != nil || len(snapshot.Items) > 6 {
		t.Fatal(snapshot, err)
	}
	runes := 0
	for _, v := range snapshot.Items {
		runes += len([]rune(v.Title)) + len([]rune(v.Content))
	}
	if runes > 1600 {
		t.Fatal("memory context unbounded", runes)
	}
	if _, err = s.SaveAuthorMemory(ctx, memoryInput("密钥", "sk-"+strings.Repeat("a", 30))); err == nil {
		t.Fatal("credential persisted")
	}
	if _, err = s.SaveAuthorMemory(ctx, AuthorMemoryInput{Scope: "book", ProjectID: "missing", Title: "无书", Content: "规则"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("orphan book memory", err)
	}
}

func TestAuthorMemoryValidationLocksUntilCandidateTransactionCommits(t *testing.T) {
	s := newCore(t)
	ctx := memoryAuthor("author")
	v := saveMemory(t, s, ctx, memoryInput("偏好", "使用具象动作表达情绪。"))
	snapshot, err := s.ResolveAuthorMemories(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if ok, e := ValidateAuthorMemorySnapshot(ctx, tx, &snapshot); e != nil || !ok {
		t.Fatal(ok, e)
	}
	finished := make(chan error, 1)
	go func() {
		timeout, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		finished <- s.DeleteAuthorMemory(timeout, v.ID, v.Version)
	}()
	select {
	case err := <-finished:
		t.Fatal("forget raced a candidate commit", err)
	case <-time.After(80 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("forget remained blocked after commit")
	}
	if ok, e := ValidateAuthorMemorySnapshot(ctx, s.DB, &snapshot); e != nil || ok {
		t.Fatal("forget failed to invalidate stale snapshot", e)
	}
}

func TestAuthorMemoryDuplicateInactiveManualSaveDoesNotPretendToRemember(t *testing.T) {
	s := newCore(t)
	ctx := memoryAuthor("author")
	in := memoryInput("旧偏好", "用动作表达犹豫，保留对白空白。")
	in.Status = "rejected"
	v := saveMemory(t, s, ctx, in)
	in.Status = "active"
	if _, err := s.SaveAuthorMemory(ctx, in); !IsConflict(err) {
		t.Fatal("manual save falsely reported an active duplicate", err)
	}
	current, err := s.GetAuthorMemory(ctx, v.ID)
	if err != nil || current.Status != "rejected" || current.Version != 1 {
		t.Fatal("duplicate unexpectedly changed rejected preference", current, err)
	}
	if _, err = s.SaveAuthorMemory(ctx, memoryInput("sk-"+strings.Repeat("b", 30), "合法正文")); err == nil {
		t.Fatal("credential title retained in memory")
	}
}

func TestAuthorMemoryRemainsManageableAfterSourceBookDeletion(t *testing.T) {
	s := newCore(t)
	p, _ := setupCore(t, s)
	ctx := memoryAuthor("author")
	sid, tid := memoryEvidenceTurn(t, s, "author", p.ID, "我喜欢对白简洁，不解释已知信息。", "")
	in := memoryInput("长期文风", "对白简洁，不解释彼此已知信息。")
	in.SourceSessionID, in.SourceTurnID, in.SourceQuote = sid, tid, "我喜欢对白简洁"
	global := saveMemory(t, s, ctx, in)
	in.Scope, in.ProjectID = "book", p.ID
	local := saveMemory(t, s, ctx, in)
	if err := s.DeleteProject(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	disabled := "disabled"
	for _, v := range []AuthorMemory{global, local} {
		next, err := s.UpdateAuthorMemory(ctx, v.ID, AuthorMemoryUpdate{ExpectedVersion: v.Version, Status: &disabled})
		if err != nil || next.Status != "disabled" || next.SourceQuote != in.SourceQuote {
			t.Fatal("private entry became unmanageable after its original conversation was deleted", next, err)
		}
		if err = s.DeleteAuthorMemory(ctx, next.ID, next.Version); err != nil {
			t.Fatal(err)
		}
	}
}
