package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// DecodeIssues enforces the same closed object shape and enums as
// schemas/review-issues-v1.json. JSON format repair is deliberately not automatic.
func DecodeIssues(raw []byte) ([]Issue, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("review report exceeds size limit")
	}
	var report struct {
		Issues *[]Issue `json:"issues"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return nil, errors.New("review report does not match review-issues-v1 schema")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("review report must contain a single JSON object")
	}
	if report.Issues == nil {
		return nil, errors.New("review report must include an issues array")
	}
	for index, issue := range *report.Issues {
		if err := validateShape(issue); err != nil {
			return nil, fmt.Errorf("issue %d: %w", index+1, err)
		}
	}
	return *report.Issues, nil
}

func validateShape(issue Issue) error {
	switch issue.Kind {
	case "Continuity", "CharacterKnowledge", "Timeline":
	default:
		return errors.New("unsupported issue kind")
	}
	switch issue.Severity {
	case "Minor", "Major", "Critical":
	default:
		return errors.New("unsupported issue severity")
	}
	if strings.TrimSpace(issue.Quote) == "" || strings.TrimSpace(issue.Explanation) == "" || strings.TrimSpace(issue.SuggestedAction) == "" {
		return errors.New("quote, explanation, and suggestedAction must be nonempty")
	}
	return nil
}

// ValidateIssues establishes that each quote is verbatim evidence in the exact
// input snapshot. This does not establish the semantic correctness of a claim;
// precision and recall still require independently labelled contradictions.
func ValidateIssues(source string, issues []Issue) error {
	for index, issue := range issues {
		if err := validateShape(issue); err != nil {
			return fmt.Errorf("issue %d: %w", index+1, err)
		}
		if !strings.Contains(source, issue.Quote) {
			return fmt.Errorf("issue %d: evidence quote is absent from the input snapshot", index+1)
		}
	}
	return nil
}
