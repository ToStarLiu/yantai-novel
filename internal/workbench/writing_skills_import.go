package workbench

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"net/url"
	"novel/internal/job"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxSkillDocument = 100000
const maxSkillBundle = 524288

type WritingSkillPreviewInput struct {
	Format        string `json:"format"`
	Content       string `json:"content,omitempty"`
	Filename      string `json:"filename,omitempty"`
	ArchiveBase64 string `json:"archiveBase64,omitempty"`
	URL           string `json:"url,omitempty"`
}

var skillSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)

func parseWritingSkill(content string, resources []WritingSkillResource, source WritingSkillSource) (SkillPreview, error) {
	source.Verification = ""
	if source.URL != "" {
		u, e := url.Parse(source.URL)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || strings.ContainsAny(source.URL, "\r\n") {
			return SkillPreview{}, fmt.Errorf("Skill来源声明需为不含凭据的HTTP(S)链接")
		}
		source.Verification = "declared"
	}
	out := SkillPreview{Content: strings.TrimPrefix(content, "\ufeff"), Resources: []WritingSkillResource{}, Source: source, Warnings: []string{}}
	if len(content) > maxSkillDocument || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return out, fmt.Errorf("SKILL.md需为UTF-8文本，最多100KB")
	}
	normalized := strings.ReplaceAll(out.Content, "\r\n", "\n")
	out.Content = normalized
	if !strings.HasPrefix(normalized, "---\n") {
		return out, fmt.Errorf("SKILL.md需要YAML头部，包含name和description")
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return out, fmt.Errorf("SKILL.md的YAML头部未闭合")
	}
	end += 4
	if len(normalized) > end+4 && normalized[end+4] != '\n' {
		return out, fmt.Errorf("Skill的YAML结束分隔符需要独立一行")
	}
	if end > 16000 {
		return out, fmt.Errorf("Skill元信息过长")
	}
	var front map[string]any
	if err := yaml.Unmarshal([]byte(normalized[4:end]), &front); err != nil {
		return out, fmt.Errorf("Skill的YAML元信息无效")
	}
	out.Name, _ = front["name"].(string)
	out.Description, _ = front["description"].(string)
	if !skillSlug.MatchString(out.Name) || strings.TrimSpace(out.Description) == "" || len([]rune(out.Description)) > 2000 {
		return out, fmt.Errorf("Skill需有效name（字母、数字、连字符，下划线，最多100字符）和description（最多2000字）")
	}
	body := strings.TrimSpace(normalized[end+4:])
	if body == "" {
		return out, fmt.Errorf("Skill正文不能为空")
	}
	if license, ok := front["license"].(string); ok && out.Source.License == "" {
		out.Source.License = license
	}
	if author, ok := front["author"].(string); ok && out.Source.Author == "" {
		out.Source.Author = author
	}
	if metadata, ok := front["metadata"].(map[string]any); ok {
		if author, ok := metadata["author"].(string); ok && out.Source.Author == "" {
			out.Source.Author = author
		}
	}
	if len(source.URL) > 2000 || len(source.Revision) > 100 || len(out.Source.License) > 500 || len(out.Source.Author) > 500 {
		return out, fmt.Errorf("Skill来源信息过长")
	}
	for _, k := range []string{"allowed-tools", "tools", "command", "hooks", "disable-model-invocation", "context", "model"} {
		if _, exists := front[k]; exists {
			out.Warnings = append(out.Warnings, "元信息“"+k+"”只作记录，不会授权工具、执行命令或切换模型。")
		}
	}
	total := len(content)
	seen := map[string]bool{}
	for _, r := range resources {
		clean, err := validSkillResourcePath(r.Path)
		if err != nil {
			return out, err
		}
		if !allowedSkillResource(clean) {
			out.Warnings = append(out.Warnings, "已忽略非文档文件："+clean)
			continue
		}
		if seen[clean] || clean == "SKILL.md" {
			return out, fmt.Errorf("Skill资料路径重复")
		}
		seen[clean] = true
		if len(r.Content) > maxSkillDocument || !utf8.ValidString(r.Content) || strings.ContainsRune(r.Content, 0) {
			return out, fmt.Errorf("Skill资料须为UTF-8文本，每份不超过100KB")
		}
		total += len(r.Content)
		if total > maxSkillBundle || len(out.Resources) >= 40 {
			return out, fmt.Errorf("Skill资料最多40份，正文与资料合计不超过512KB")
		}
		r.Path = clean
		out.Resources = append(out.Resources, r)
	}
	sort.Slice(out.Resources, func(i, j int) bool { return out.Resources[i].Path < out.Resources[j].Path })
	if strings.Contains(body, "../") {
		out.Warnings = append(out.Warnings, "正文包含跨目录引用；只导入当前Skill目录内资料，其他Skill须另行导入。")
	}
	lower := strings.ToLower(normalized)
	if strings.Contains(lower, "scripts/") || strings.Contains(lower, "bash ") || strings.Contains(lower, "python ") {
		out.Warnings = append(out.Warnings, "此Skill引用脚本或命令，本应用只使用写作指导与文本资料，不执行脚本。")
	}
	if out.Source.License == "" {
		out.Warnings = append(out.Warnings, "未声明许可证；请核对来源授权后再分发。")
	}
	if out.Source.Verification == "declared" {
		out.Warnings = append(out.Warnings, "来源与许可证为资料包声明，本次未联网重新核验。")
	}
	out.Warnings = append(out.Warnings, "导入内容是写作指导，不能覆盖作品事实、人物知识边界、权限、预算或作者采用流程。")
	hashPayload, _ := json.Marshal(struct {
		Content   string
		Resources []WritingSkillResource
	}{out.Content, out.Resources})
	out.Hash = job.Hash(hashPayload)
	return out, nil
}
func validSkillResourcePath(value string) (string, error) {
	if value == "" || len(value) > 400 || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || strings.Contains(value, ":") {
		return "", fmt.Errorf("Skill资料路径无效")
	}
	clean := path.Clean(value)
	if clean != value || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("Skill资料不可跨目录")
	}
	return clean, nil
}
func allowedSkillResource(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == "scripts" || part == "node_modules" || part == ".git" {
			return false
		}
	}
	base := strings.ToLower(path.Base(name))
	if strings.HasPrefix(base, "license") || strings.HasPrefix(base, "licence") || strings.HasPrefix(base, "notice") || strings.HasPrefix(base, "copyright") {
		return true
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".txt", ".json", ".yaml", ".yml":
		return true
	}
	return false
}
func (s *Service) PreviewWritingSkill(ctx context.Context, pid string, in WritingSkillPreviewInput) (SkillPreview, error) {
	if err := s.RequireProjectRole(ctx, pid, "editor"); err != nil {
		return SkillPreview{}, err
	}
	switch in.Format {
	case "", "markdown":
		return parseWritingSkill(in.Content, nil, WritingSkillSource{})
	case "bundle":
		var preview SkillPreview
		if len(in.Content) > maxSkillBundle*2 || json.Unmarshal([]byte(in.Content), &preview) != nil {
			return SkillPreview{}, fmt.Errorf("Skill资料包JSON无效")
		}
		return parseWritingSkill(preview.Content, preview.Resources, preview.Source)
	case "zip":
		return previewWritingSkillZip(in.ArchiveBase64)
	case "github":
		out, err := previewGitHubWritingSkill(ctx, in.URL)
		if err == nil {
			markWritingSkillVerified(&out)
			out.PreviewToken = signWritingSkillPreview(out)
		}
		return out, err
	default:
		return SkillPreview{}, fmt.Errorf("Skill导入格式无效")
	}
}
func previewWritingSkillZip(encoded string) (SkillPreview, error) {
	out := SkillPreview{}
	if len(encoded) > 2<<20 {
		return out, fmt.Errorf("ZIP压缩包过大，最多1MB")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) > 1<<20 {
		return out, fmt.Errorf("ZIP内容无效或过大")
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(reader.File) > 100 {
		return out, fmt.Errorf("ZIP内容无效或文件过多")
	}
	files := map[string]string{}
	total := uint64(0)
	skillPath := ""
	warnings := []string{}
	for _, f := range reader.File {
		name, err := validSkillResourcePath(strings.TrimSuffix(f.Name, "/"))
		if err != nil {
			return out, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&0120000 != 0 || !f.Mode().IsRegular() {
			return out, fmt.Errorf("ZIP不允许符号链接或特殊文件")
		}
		if _, seen := files[name]; seen {
			return out, fmt.Errorf("ZIP文件路径重复")
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > maxSkillDocument || total > maxSkillBundle {
			return out, fmt.Errorf("ZIP解压后内容超出512KB限额")
		}
		if path.Base(name) == "SKILL.md" {
			if skillPath != "" {
				return out, fmt.Errorf("每次仅导入一个Skill，请选择只含一个SKILL.md的目录包")
			}
			skillPath = name
		}
		if !allowedSkillResource(name) {
			warnings = append(warnings, "已忽略非文档文件："+name)
			files[name] = ""
			continue
		}
		r, err := f.Open()
		if err != nil {
			return out, fmt.Errorf("无法读取ZIP条目")
		}
		body, err := io.ReadAll(io.LimitReader(r, maxSkillDocument+1))
		r.Close()
		if err != nil || len(body) > maxSkillDocument {
			return out, fmt.Errorf("ZIP条目超出限额")
		}
		files[name] = string(body)
	}
	if skillPath == "" {
		return out, fmt.Errorf("ZIP中未找到SKILL.md")
	}
	dir := path.Dir(skillPath)
	resources := []WritingSkillResource{}
	for name, body := range files {
		if name == skillPath || !allowedSkillResource(name) {
			continue
		}
		relative := name
		if dir != "." {
			if !strings.HasPrefix(name, dir+"/") {
				warnings = append(warnings, "已忽略Skill目录之外的文件："+name)
				continue
			}
			relative = strings.TrimPrefix(name, dir+"/")
		}
		resources = append(resources, WritingSkillResource{relative, body})
	}
	out, err = parseWritingSkill(files[skillPath], resources, WritingSkillSource{})
	out.Warnings = append(out.Warnings, warnings...)
	return out, err
}

var githubSlug = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
var githubSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func githubSkillLocation(value string) (owner, repo, ref, folder string, err error) {
	u, e := url.Parse(value)
	if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		err = fmt.Errorf("仅支持HTTPS GitHub Skill文件或目录链接")
		return
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch u.Hostname() {
	case "github.com":
		if len(parts) < 5 || (parts[2] != "tree" && parts[2] != "blob") {
			err = fmt.Errorf("请提供GitHub tree或blob链接")
			return
		}
		owner, repo, ref = parts[0], parts[1], parts[3]
		folder = strings.Join(parts[4:], "/")
	case "raw.githubusercontent.com":
		if len(parts) < 4 {
			err = fmt.Errorf("GitHub原文链接不完整")
			return
		}
		owner, repo, ref = parts[0], parts[1], parts[2]
		folder = strings.Join(parts[3:], "/")
	default:
		err = fmt.Errorf("导入来源仅支持github.com或raw.githubusercontent.com")
		return
	}
	if !githubSlug.MatchString(owner) || !githubSlug.MatchString(repo) || !githubSlug.MatchString(ref) {
		err = fmt.Errorf("GitHub仓库或引用无效，请使用固定提交SHA链接")
		return
	}
	if _, e = validSkillResourcePath(folder); e != nil {
		err = e
		return
	}
	if path.Base(folder) == "SKILL.md" {
		folder = path.Dir(folder)
	}
	return
}
func fetchSkillGitHub(ctx context.Context, endpoint string, max int) ([]byte, int, error) {
	u, _ := url.Parse(endpoint)
	if u.Scheme != "https" || (u.Host != "api.github.com" && u.Host != "raw.githubusercontent.com") {
		return nil, 0, fmt.Errorf("Skill来源地址无效")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Yantai-Writing-Skills/0.4")
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("无法连接GitHub，请稍后重试或导入本地Skill文件")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, response.StatusCode, fmt.Errorf("GitHub返回%d，请检查链接或访问额度", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(max+1)))
	if err != nil || len(body) > max {
		return nil, response.StatusCode, fmt.Errorf("GitHub Skill资料超出大小限额")
	}
	return body, response.StatusCode, nil
}
func previewGitHubWritingSkill(ctx context.Context, location string) (SkillPreview, error) {
	out := SkillPreview{}
	owner, repo, ref, folder, err := githubSkillLocation(location)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	sha := ref
	if !githubSHA.MatchString(sha) {
		raw, _, e := fetchSkillGitHub(ctx, "https://api.github.com/repos/"+owner+"/"+repo+"/commits/"+url.PathEscape(ref), 100000)
		if e != nil {
			return out, e
		}
		var result struct {
			SHA string `json:"sha"`
		}
		if json.Unmarshal(raw, &result) != nil || !githubSHA.MatchString(result.SHA) {
			return out, fmt.Errorf("无法解析GitHub提交版本")
		}
		sha = result.SHA
	}
	base := "https://raw.githubusercontent.com/" + owner + "/" + repo + "/" + sha + "/"
	prefix := folder + "/"
	if folder == "." {
		prefix = ""
	}
	body, _, err := fetchSkillGitHub(ctx, base+prefix+"SKILL.md", maxSkillDocument)
	if err != nil {
		return out, err
	}
	resources := []WritingSkillResource{}
	total := len(body)
	warnings := []string{}
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 6 {
			return fmt.Errorf("Skill资料目录过深")
		}
		raw, _, e := fetchSkillGitHub(ctx, "https://api.github.com/repos/"+owner+"/"+repo+"/contents/"+dir+"?ref="+sha, 300000)
		if e != nil {
			return e
		}
		var entries []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Type string `json:"type"`
			Size int    `json:"size"`
		}
		if json.Unmarshal(raw, &entries) != nil {
			return fmt.Errorf("GitHub Skill目录格式无效")
		}
		for _, entry := range entries {
			if entry.Name == "SKILL.md" {
				continue
			}
			if !strings.HasPrefix(entry.Path, prefix) && prefix != "" {
				return fmt.Errorf("GitHub资料路径越界")
			}
			relative := strings.TrimPrefix(entry.Path, prefix)
			if _, e = validSkillResourcePath(relative); e != nil {
				return e
			}
			if entry.Type == "dir" {
				if entry.Name == "scripts" || strings.HasPrefix(entry.Name, ".") || entry.Name == "node_modules" {
					warnings = append(warnings, "已忽略代码目录："+relative)
					continue
				}
				if e = walk(entry.Path, depth+1); e != nil {
					return e
				}
				continue
			}
			if entry.Type != "file" || !allowedSkillResource(relative) {
				warnings = append(warnings, "已忽略非文档条目："+relative)
				continue
			}
			if entry.Size > maxSkillDocument || len(resources) >= 38 || total+entry.Size > maxSkillBundle {
				return fmt.Errorf("Skill资料最多40份，合计512KB；请整理后用本地资料包导入")
			}
			content, _, e := fetchSkillGitHub(ctx, base+entry.Path, maxSkillDocument)
			if e != nil {
				return e
			}
			total += len(content)
			resources = append(resources, WritingSkillResource{relative, string(content)})
		}
		return nil
	}
	if err = walk(strings.TrimSuffix(prefix, "/"), 0); err != nil {
		return out, err
	}
	license := ""
	for _, name := range []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING", "NOTICE"} {
		if len(resources) >= 40 {
			break
		}
		exists := false
		for _, r := range resources {
			exists = exists || r.Path == name || r.Path == "_source/"+name
		}
		if exists {
			continue
		}
		notice, status, e := fetchSkillGitHub(ctx, base+name, maxSkillDocument)
		if status == 404 {
			continue
		}
		if e != nil {
			warnings = append(warnings, "未获取根目录许可文件："+name)
			continue
		}
		total += len(notice)
		if total > maxSkillBundle {
			return out, fmt.Errorf("Skill含许可资料超出512KB")
		}
		resources = append(resources, WritingSkillResource{"_source/" + name, string(notice)})
		if name != "NOTICE" {
			license = "See _source/" + name
			if bytes.Contains(notice, []byte("Apache License")) {
				license = "Apache-2.0"
			} else if bytes.Contains(notice, []byte("MIT License")) {
				license = "MIT"
			}
		}
	}
	source := WritingSkillSource{URL: "https://github.com/" + owner + "/" + repo + "/tree/" + sha + "/" + folder, Revision: sha, License: license, Author: owner}
	out, err = parseWritingSkill(string(body), resources, source)
	out.Warnings = append(out.Warnings, warnings...)
	return out, err
}

var writingSkillPreviewSecret = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("cannot initialize skill preview signing")
	}
	return key
}()

func signWritingSkillPreview(v SkillPreview) string {
	raw, _ := json.Marshal(struct {
		Content   string
		Resources []WritingSkillResource
		Source    WritingSkillSource
	}{v.Content, v.Resources, v.Source})
	mac := hmac.New(sha256.New, writingSkillPreviewSecret)
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func validWritingSkillPreview(v SkillPreview) bool {
	provided, err := hex.DecodeString(v.PreviewToken)
	if err != nil {
		return false
	}
	expected, _ := hex.DecodeString(signWritingSkillPreview(v))
	return hmac.Equal(provided, expected)
}

func markWritingSkillVerified(preview *SkillPreview) {
	preview.Source.Verification = "github-pinned"
	warnings := []string{}
	for _, warning := range preview.Warnings {
		if warning != "来源与许可证为资料包声明，本次未联网重新核验。" {
			warnings = append(warnings, warning)
		}
	}
	preview.Warnings = warnings
}
