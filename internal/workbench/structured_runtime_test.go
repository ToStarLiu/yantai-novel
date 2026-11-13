package workbench

import (
	"context"
	"novel/internal/llm"
	"testing"
)

func TestStructuredRuntimeRepairPreservesOriginalAndEvidenceValidation(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "valid-evidence", false: "invented-evidence"}[valid], func(t *testing.T) {
			s, c := runtimeFixture(t)
			quote := "林川左臂受伤。"
			if !valid {
				quote = "不存在的证据"
			}
			raw := "```json\n{\"issues\":[{\"kind\":\"Continuity\",\"severity\":\"Major\",\"quote\":\"" + quote + "\",\"explanation\":\"伤势需保持\",\"suggestedAction\":\"保留受伤\",},],}\n```"
			s.Provider = &llm.Fake{Handler: func(context.Context, llm.Request) (llm.Response, error) {
				return llm.Response{Text: raw, Raw: []byte(raw)}, nil
			}}
			r := submit(t, s, c, "review")
			claimed := claim(t, s)
			if err := s.ExecuteRun(context.Background(), claimed); err != nil {
				t.Fatal(err)
			}
			result, err := s.GetRun(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if valid && (result.Status != "Succeeded" || !result.Output.FormatRepaired) {
				t.Fatalf("repair did not pass: %#v", result)
			}
			if !valid && result.Status != "Failed" {
				t.Fatal("repair bypassed evidence")
			}
			var stored []byte
			if err = s.DB.QueryRow(context.Background(), "SELECT raw FROM wb_run_attempts WHERE run_id=$1", r.ID).Scan(&stored); err != nil || string(stored) != raw {
				t.Fatal("original response lost", err)
			}
		})
	}
}
