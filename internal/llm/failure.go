package llm

import "errors"

const (
	Retryable           = "Retryable"
	Permanent           = "Permanent"
	NeedsReconciliation = "NeedsReconciliation"
)

// Failure describes recovery without including credentials, request bodies, or
// provider-controlled error text. The raw response is returned in Response.Raw.
type Failure struct {
	Disposition string
	Kind        string
	StatusCode  int
}

func (f *Failure) Error() string {
	return "model request failed: " + f.Kind + " (" + f.Disposition + ")"
}

// FailureDisposition never assumes that an unknown external side effect can be
// repeated safely. An adapter may establish a more precise disposition.
func FailureDisposition(err error, mayHaveExecuted bool) string {
	if err == nil {
		return ""
	}
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Disposition
	}
	if mayHaveExecuted {
		return NeedsReconciliation
	}
	return Retryable
}
