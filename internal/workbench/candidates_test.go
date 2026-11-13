package workbench

import (
	"context"
	"novel/internal/llm"
	"testing"
)

func TestCandidateGenerationAndEvidenceRankingAreDurable(t *testing.T) {
	s, c := runtimeFixture(t)
	ctx := context.Background()
	s.Provider = &llm.Fake{Handler: func(_ context.Context, r llm.Request) (llm.Response, error) {
		text := "候选中林川左臂仍然受伤。"
		if len(r.ResponseSchema) > 0 {
			text = `{"issues":[]}`
		}
		return llm.Response{Text: text, Raw: []byte(text)}, nil
	}}
	g, e := s.StartCandidates(ctx, c.ID, CandidateRequest{Count: 2, InputRevisionID: c.CurrentRevisionID})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 12; i++ {
		if e = s.AdvanceCandidates(ctx); e != nil {
			t.Fatal(e)
		}
		r, ok, e := s.ClaimRun(ctx, "test-worker")
		if e != nil {
			t.Fatal(e)
		}
		if ok {
			if e = s.ExecuteRun(ctx, r); e != nil {
				t.Fatal(e)
			}
		}
	}
	done, e := s.GetCandidateGroup(ctx, g.ID)
	if e != nil || done.Status != "Succeeded" || len(done.RunIDs) != 2 || len(done.Rankings) != 2 {
		t.Fatalf("candidate %+v %v", done, e)
	}
	var calls int
	if e = s.DB.QueryRow(ctx, "SELECT count(*) FROM wb_run_attempts").Scan(&calls); e != nil || calls != 4 {
		t.Fatalf("duplicate candidate attempts %d %v", calls, e)
	}
	ch, _ := s.GetChapter(ctx, c.ID)
	if ch.CurrentRevisionID != c.CurrentRevisionID {
		t.Fatal("candidate auto-applied")
	}
}
