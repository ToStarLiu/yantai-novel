package review

import "testing"

func validIssue() Issue {
	return Issue{Kind: "Continuity", Severity: "Major", Quote: "林川把铜印放进口袋。", Explanation: "需与先前归属核对", SuggestedAction: "核对铜印当前持有人"}
}

func TestValidateIssuesRejectsInventedEvidence(t *testing.T) {
	issue := validIssue()
	issue.Quote = "铜印已经熔毁"
	if err := ValidateIssues("林川把铜印放进口袋。", []Issue{issue}); err == nil {
		t.Fatal("invented evidence must be rejected")
	}
}

func TestValidateIssues(t *testing.T) {
	if err := ValidateIssues("林川把铜印放进口袋。", []Issue{validIssue()}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIssues("正文没有矛盾。", []Issue{}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Issue){
		func(i *Issue) { i.Quote = " " },
		func(i *Issue) { i.Kind = "Style" },
		func(i *Issue) { i.Severity = "Unspecified" },
		func(i *Issue) { i.Explanation = "" },
		func(i *Issue) { i.SuggestedAction = "" },
	} {
		i := validIssue()
		change(&i)
		if err := ValidateIssues("林川把铜印放进口袋。", []Issue{i}); err == nil {
			t.Fatalf("invalid issue accepted: %+v", i)
		}
	}
}

func TestDecodeIssuesStrictShape(t *testing.T) {
	valid := `{"issues":[{"kind":"Continuity","severity":"Major","quote":"林川把铜印放进口袋。","explanation":"与归属冲突","suggestedAction":"核对归属"}]}`
	issues, err := DecodeIssues([]byte(valid))
	if err != nil || len(issues) != 1 {
		t.Fatalf("decode valid: %v, %+v", err, issues)
	}
	if issues, err = DecodeIssues([]byte(`{"issues":[]}`)); err != nil || len(issues) != 0 {
		t.Fatalf("empty report is valid: %v", err)
	}
	for _, invalid := range []string{
		`[]`, `{}`, `{"issues":null}`, `{"issues":[],"extra":true}`, `{"issues":[]} {"issues":[]}`,
		`{"issues":[{"kind":"Continuity","severity":"Major","quote":"引用","explanation":"说明","suggestedAction":"核对","extra":true}]}`,
		`{"issues":[{"kind":"Unspecified","severity":"Major","quote":"引用","explanation":"说明","suggestedAction":"核对"}]}`,
		`{"issues":[{"kind":"Continuity","quote":"引用","explanation":"说明","suggestedAction":"核对"}]}`,
	} {
		if _, err := DecodeIssues([]byte(invalid)); err == nil {
			t.Fatalf("invalid schema accepted: %s", invalid)
		}
	}
}
