package workbench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"novel/internal/job"
	"strings"
	"unicode/utf8"
)

type documentNode struct {
	Type    string            `json:"type"`
	Text    string            `json:"text,omitempty"`
	Attrs   json.RawMessage   `json:"attrs,omitempty"`
	Marks   []json.RawMessage `json:"marks,omitempty"`
	Content []documentNode    `json:"content,omitempty"`
}

// NormalizeDocument derives both the model text and its hash on the server.
// The JSON is retained as the authoritative editor representation.
func NormalizeDocument(raw json.RawMessage) (json.RawMessage, string, string, error) {
	if len(raw) > 4<<20 || !utf8.Valid(raw) {
		return nil, "", "", fmt.Errorf("正文超过限制或不是UTF-8")
	}
	var root documentNode
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&root); err != nil || root.Type != "doc" {
		return nil, "", "", fmt.Errorf("正文必须为Tiptap文档")
	}
	if !json.Valid(raw) {
		return nil, "", "", fmt.Errorf("正文JSON无效")
	}
	count := 0
	text, err := nodeText(root, 0, &count)
	if err != nil {
		return nil, "", "", err
	}
	normalized, err := json.Marshal(root)
	if err != nil {
		return nil, "", "", err
	}
	text = strings.TrimSuffix(text, "\n")
	return normalized, text, job.Hash([]byte(text)), nil
}

func nodeText(n documentNode, depth int, count *int) (string, error) {
	*count++
	if depth > 40 || *count > 100000 {
		return "", fmt.Errorf("文档结构过于复杂")
	}
	switch n.Type {
	case "text":
		if len(n.Content) > 0 {
			return "", fmt.Errorf("文字节点不能包含子节点")
		}
		return n.Text, nil
	case "hardBreak":
		return "\n", nil
	case "horizontalRule":
		return "\n", nil
	case "doc", "paragraph", "heading", "bulletList", "orderedList", "listItem", "blockquote", "codeBlock":
	default:
		return "", fmt.Errorf("不支持的正文节点: %s", n.Type)
	}
	if n.Text != "" {
		return "", fmt.Errorf("非文字节点包含无效text")
	}
	var b strings.Builder
	for _, child := range n.Content {
		part, err := nodeText(child, depth+1, count)
		if err != nil {
			return "", err
		}
		b.WriteString(part)
	}
	switch n.Type {
	case "paragraph", "heading", "codeBlock":
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func DocumentFromText(text string) json.RawMessage {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	root := documentNode{Type: "doc", Content: []documentNode{}}
	for _, line := range strings.Split(text, "\n") {
		p := documentNode{Type: "paragraph"}
		if line != "" {
			p.Content = []documentNode{{Type: "text", Text: line}}
		}
		root.Content = append(root.Content, p)
	}
	raw, _ := json.Marshal(root)
	return raw
}
