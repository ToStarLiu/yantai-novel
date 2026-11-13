package workbench

import (
	"novel/internal/review"
	"testing"
)

func TestStructuredRepairChangesFormattingOnlyAndRetainsEvidenceGate(t *testing.T) {
	source := "```json\n{\"issues\":[{\"kind\":\"Continuity\",\"severity\":\"Major\",\"quote\":\"伤口,}\",\"explanation\":\"有伤\",\"suggestedAction\":\"保留伤势\",},],}\n```"
	value, changed := repairStructuredJSON(source)
	if !changed {
		t.Fatal("format not repaired")
	}
	issues, err := review.DecodeIssues([]byte(value))
	if err != nil || issues[0].Quote != "伤口,}" {
		t.Fatalf("string content changed: %s %v", value, err)
	}
	if review.ValidateIssues("不存在该引文", issues) == nil {
		t.Fatal("repair bypassed evidence")
	}
	for _, bad := range []string{"说明{\"issues\":[]}", "{issues:[]}", "{\"issues\":", "[1,]"} {
		if _, changed := repairStructuredJSON(bad); changed {
			t.Fatalf("unsafe repair of %q", bad)
		}
	}
	if _, err := review.DecodeIssues([]byte("{\"issues\":[],\"invented\":true}")); err == nil {
		t.Fatal("closed schema bypassed")
	}
}
