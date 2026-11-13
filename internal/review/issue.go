package review

type Issue struct {
	Kind            string `json:"kind"`
	Severity        string `json:"severity"`
	Quote           string `json:"quote"`
	Explanation     string `json:"explanation"`
	SuggestedAction string `json:"suggestedAction"`
}
