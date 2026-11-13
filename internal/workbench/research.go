package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
	"novel/internal/job"
)

type ResearchLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}
type ResearchSource struct {
	URL         string         `json:"url"`
	FetchedAt   time.Time      `json:"fetchedAt"`
	Title       string         `json:"title"`
	Text        string         `json:"text"`
	Excerpt     string         `json:"excerpt"`
	Links       []ResearchLink `json:"links"`
	ReferenceID string         `json:"referenceId"`
	Error       string         `json:"error"`
}
type ResearchReport struct {
	ID        string           `json:"id"`
	ProjectID string           `json:"projectId"`
	Topic     string           `json:"topic"`
	Status    string           `json:"status"`
	Sources   []ResearchSource `json:"sources"`
	CreatedAt time.Time        `json:"createdAt"`
}
type ResearchSearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}
type ApplyPlanInput struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Scope           string `json:"scope"`
	VolumeID        string `json:"volumeId"`
	Text            string `json:"text"`
	RunID           string `json:"runId"`
}

const researchMaxBytes = 2 << 20

var researchDeniedPrefixes = []netip.Prefix{netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("64:ff9b::/96")}

func researchPublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, prefix := range researchDeniedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
func researchURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || len(raw) > 4096 {
		return nil, fmt.Errorf("仅允许不带认证信息的公开HTTP(S)地址")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		return nil, fmt.Errorf("禁止本机或内部网络地址")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !researchPublicIP(ip) {
		return nil, fmt.Errorf("禁止私网、回环或保留IP地址")
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return nil, fmt.Errorf("公开资料抓取只允许80或443端口")
	}
	u.Fragment = ""
	return u, nil
}
func researchResolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !researchPublicIP(ip) {
			return nil, fmt.Errorf("禁止内部地址")
		}
		return []netip.Addr{ip}, nil
	}
	values, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(values) == 0 {
		return nil, fmt.Errorf("公开来源域名无法解析")
	}
	for _, ip := range values {
		if !researchPublicIP(ip) {
			return nil, fmt.Errorf("来源域名解析到私网或保留地址")
		}
	}
	return values, nil
}
func defaultResearchClient() *http.Client {
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, ForceAttemptHTTP2: true, MaxIdleConns: 5, IdleConnTimeout: 20 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := researchResolve(ctx, host)
		if err != nil {
			return nil, err
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		var last error
		for _, ip := range ips {
			conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}
}
func guardedResearchClient(client *http.Client) *http.Client {
	if client == nil {
		client = defaultResearchClient()
	}
	copy := *client
	copy.Timeout = 10 * time.Second
	copy.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("来源重定向过多")
		}
		_, err := researchURL(req.URL.String())
		if err != nil {
			return err
		}
		req.Header.Del("Authorization")
		return nil
	}
	return &copy
}
func fetchResearch(ctx context.Context, client *http.Client, raw string) (ResearchSource, error) {
	source := ResearchSource{URL: raw, FetchedAt: time.Now().UTC(), Links: []ResearchLink{}}
	u, err := researchURL(raw)
	if err != nil {
		return source, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return source, fmt.Errorf("来源请求无效")
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9")
	req.Header.Set("User-Agent", "NovelWorkbench-Research/1.0 (author-requested public-source research)")
	response, err := client.Do(req)
	if err != nil {
		return source, fmt.Errorf("公开来源请求失败或被网络策略拒绝")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return source, fmt.Errorf("公开来源返回HTTP %d", response.StatusCode)
	}
	contentType, params, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentType != "text/html" && contentType != "application/xhtml+xml" && contentType != "text/plain" {
		return source, fmt.Errorf("来源不是HTML或纯文本")
	}
	rawBody, err := io.ReadAll(io.LimitReader(response.Body, researchMaxBytes+1))
	if err != nil {
		return source, fmt.Errorf("读取公开来源失败")
	}
	if len(rawBody) > researchMaxBytes {
		return source, fmt.Errorf("单个公开来源超过2MiB上限")
	}
	charset := strings.ToLower(params["charset"])
	if charset != "" && charset != "utf-8" && charset != "utf8" {
		encoding, err := htmlindex.Get(charset)
		if err != nil {
			return source, fmt.Errorf("来源字符编码不支持")
		}
		rawBody, err = encoding.NewDecoder().Bytes(rawBody)
		if err != nil {
			return source, fmt.Errorf("来源字符编码无效")
		}
	}
	if !utf8.Valid(rawBody) {
		return source, fmt.Errorf("来源文本不是可读取的UTF-8，请提供明确charset的页面")
	}
	source.URL = response.Request.URL.String()
	if contentType == "text/plain" {
		source.Title = response.Request.URL.Hostname()
		source.Text = strings.TrimSpace(string(rawBody))
	} else {
		source.Title, source.Text, source.Links = parseResearchHTML(string(rawBody), response.Request.URL)
	}
	if strings.TrimSpace(source.Text) == "" {
		return source, fmt.Errorf("来源没有可读取的正文；可能需要登录或JavaScript渲染")
	}
	if source.Title == "" {
		source.Title = u.Hostname()
	}
	runes := []rune(source.Text)
	if len(runes) > 1200 {
		runes = runes[:1200]
	}
	source.Excerpt = string(runes)
	return source, nil
}

var researchTitle = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
var researchScripts = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>|<noscript\b[^>]*>.*?</noscript\s*>|<!--.*?-->`)
var researchBlocks = regexp.MustCompile(`(?is)</?(?:p|div|section|article|h[1-6]|li|tr|br|main|header|footer)\b[^>]*>`)
var researchTags = regexp.MustCompile(`(?is)<[^>]+>`)
var researchLinkPattern = regexp.MustCompile(`(?is)<a\b[^>]*href\s*=\s*(?:"([^"]+)"|'([^']+)')[^>]*>(.*?)</a\s*>`)

func cleanResearchText(s string) string {
	s = researchTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}
func parseResearchHTML(input string, base *url.URL) (string, string, []ResearchLink) {
	title := ""
	if match := researchTitle.FindStringSubmatch(input); len(match) > 1 {
		title = cleanResearchText(match[1])
	}
	input = researchScripts.ReplaceAllString(input, "")
	links := []ResearchLink{}
	seen := map[string]bool{}
	for _, match := range researchLinkPattern.FindAllStringSubmatch(input, 200) {
		raw := match[1]
		if raw == "" {
			raw = match[2]
		}
		u, err := url.Parse(html.UnescapeString(raw))
		if err != nil {
			continue
		}
		absolute := base.ResolveReference(u)
		safe, err := researchURL(absolute.String())
		if err != nil || seen[safe.String()] {
			continue
		}
		label := cleanResearchText(match[3])
		if label == "" {
			continue
		}
		links = append(links, ResearchLink{Label: label, URL: safe.String()})
		seen[safe.String()] = true
	}
	input = researchTitle.ReplaceAllString(input, "")
	input = researchBlocks.ReplaceAllString(input, "\n")
	input = researchTags.ReplaceAllString(input, "")
	input = html.UnescapeString(input)
	paragraphs := []string{}
	for _, line := range strings.Split(input, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			paragraphs = append(paragraphs, line)
		}
	}
	return title, strings.Join(paragraphs, "\n"), links
}
func (s *Service) Research(ctx context.Context, pid, topic string, urls []string) (ResearchReport, error) {
	return s.ResearchWithClient(ctx, pid, topic, urls, nil)
}
func (s *Service) ResearchWithClient(ctx context.Context, pid, topic string, urls []string, client *http.Client) (ResearchReport, error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return ResearchReport{}, err
	}
	if strings.TrimSpace(topic) == "" || len(topic) > 2000 || len(urls) < 1 || len(urls) > 5 {
		return ResearchReport{}, fmt.Errorf("研究主题必填且需提供1至5个公开URL")
	}
	seen := map[string]bool{}
	for _, u := range urls {
		if seen[u] {
			return ResearchReport{}, fmt.Errorf("来源URL重复")
		}
		seen[u] = true
	}
	result := ResearchReport{ID: job.NewID(), ProjectID: pid, Topic: topic, Status: "Succeeded", Sources: []ResearchSource{}, CreatedAt: time.Now().UTC()}
	httpClient := guardedResearchClient(client)
	success := 0
	for _, raw := range urls {
		source, err := fetchResearch(ctx, httpClient, raw)
		if err != nil {
			source.Error = err.Error()
		} else {
			ref, e := s.AddStudioReference(ctx, pid, source.Title, source.Text, "公开来源："+source.URL+"；抓取于"+source.FetchedAt.Format(time.RFC3339)+"；研究主题："+topic)
			if e != nil {
				return result, e
			}
			source.ReferenceID = ref.ID
			success++
		}
		result.Sources = append(result.Sources, source)
	}
	if success == 0 {
		result.Status = "Failed"
	} else if success < len(urls) {
		result.Status = "Partial"
	}
	raw, _ := json.Marshal(result.Sources)
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_research_reports(id,project_id,topic,status,sources,created_at) VALUES($1,$2,$3,$4,$5,$6)", result.ID, pid, topic, result.Status, raw, result.CreatedAt)
	return result, err
}
func (s *Service) ListResearch(ctx context.Context, pid string) ([]ResearchReport, error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, "SELECT id,project_id,topic,status,sources,created_at FROM wb_research_reports WHERE project_id=$1 ORDER BY created_at DESC LIMIT 100", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResearchReport{}
	for rows.Next() {
		var r ResearchReport
		var raw []byte
		if err = rows.Scan(&r.ID, &r.ProjectID, &r.Topic, &r.Status, &raw, &r.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &r.Sources); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func ResearchSearchAvailable() bool { return strings.TrimSpace(os.Getenv("RESEARCH_SEARCH_URL")) != "" }
func ResearchSearch(ctx context.Context, query string) ([]ResearchSearchHit, error) {
	return researchSearchWithClient(ctx, query, nil)
}
func researchSearchWithClient(ctx context.Context, query string, client *http.Client) ([]ResearchSearchHit, error) {
	if len(query) > 2000 || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("请输入检索词")
	}
	endpoint := strings.TrimSpace(os.Getenv("RESEARCH_SEARCH_URL"))
	if endpoint == "" {
		return nil, fmt.Errorf("尚未配置联网搜索服务，请直接输入公开来源URL")
	}
	u, err := researchURL(endpoint)
	if err != nil {
		return nil, fmt.Errorf("联网搜索服务必须使用公开HTTP(S)地址")
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("count", "10")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if key := os.Getenv("RESEARCH_SEARCH_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(key))
	}
	response, err := guardedResearchClient(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("联网搜索服务不可用")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("联网搜索服务返回HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, researchMaxBytes+1))
	if err != nil || len(raw) > researchMaxBytes {
		return nil, fmt.Errorf("联网搜索响应过大或读取失败")
	}
	var decoded struct {
		Results  []ResearchSearchHit `json:"results"`
		WebPages struct {
			Value []ResearchSearchHit `json:"value"`
		} `json:"webPages"`
	}
	if json.Unmarshal(raw, &decoded) != nil {
		return nil, fmt.Errorf("联网搜索响应必须为results链接列表")
	}
	if len(decoded.Results) == 0 {
		decoded.Results = decoded.WebPages.Value
	}
	hits := []ResearchSearchHit{}
	for _, hit := range decoded.Results {
		u, err := researchURL(hit.URL)
		if err != nil {
			continue
		}
		hit.URL = u.String()
		if len(hit.Title) > 1000 {
			hit.Title = hit.Title[:1000]
		}
		hits = append(hits, hit)
		if len(hits) == 10 {
			break
		}
	}
	return hits, nil
}

func (s *Service) ApplyStudioPlan(ctx context.Context, pid string, in ApplyPlanInput) (StudioDocument[StoryBrief], error) {
	if in.Scope != "book" && in.Scope != "volume" {
		return StudioDocument[StoryBrief]{}, fmt.Errorf("规划采用范围必须为book或volume")
	}
	if strings.TrimSpace(in.Text) == "" || len(in.Text) > 200000 {
		return StudioDocument[StoryBrief]{}, fmt.Errorf("规划正文为空或过长")
	}
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	defer tx.Rollback(ctx)
	run, err := scanRun(tx.QueryRow(ctx, "SELECT "+runColumns+" FROM wb_runs WHERE id=$1 AND project_id=$2", in.RunID, pid))
	if err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	if run.Type != "planning" || run.Status != "Succeeded" || run.Stale || run.Output.Text == "" {
		return StudioDocument[StoryBrief]{}, &Conflict{Message: "规划候选来源无效或尚未完成"}
	}
	current, err := snapshotCurrent(ctx, tx, run)
	if err != nil {
		return StudioDocument[StoryBrief]{}, err
	}
	if !current {
		return StudioDocument[StoryBrief]{}, &Conflict{Message: "规划候选的正文、设定或工作室版本已变化"}
	}
	var scope string
	var volumeID string
	var input map[string]json.RawMessage
	raw, _ := json.Marshal(run.Input)
	_ = json.Unmarshal(raw, &input)
	_ = json.Unmarshal(input["scope"], &scope)
	_ = json.Unmarshal(input["volumeId"], &volumeID)
	if scope != in.Scope {
		return StudioDocument[StoryBrief]{}, fmt.Errorf("规划候选范围与采用范围不同")
	}
	if in.Scope == "volume" && volumeID != in.VolumeID {
		return StudioDocument[StoryBrief]{}, fmt.Errorf("规划候选不属于该卷")
	}
	brief, err := studioRead(ctx, tx, pid, "brief", "main", StoryBrief{WorldRules: []string{}, Volumes: []StoryVolume{}})
	if err != nil {
		return brief, err
	}
	if brief.Version != in.ExpectedVersion {
		return brief, &Conflict{Message: "全书规划已更新，请刷新"}
	}
	if in.Scope == "book" {
		brief.Data.PlotPlan = in.Text
	} else {
		found := false
		for i, v := range brief.Data.Volumes {
			if v.ID == in.VolumeID {
				brief.Data.Volumes[i].Summary = in.Text
				found = true
				break
			}
		}
		if !found {
			return brief, fmt.Errorf("卷ID不属于当前作品规划")
		}
	}
	saved, err := studioSaveTx(ctx, tx, pid, "brief", "main", in.ExpectedVersion, brief.Data)
	if err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}

// ResearchComparison exposes long verbatim overlaps as author-review evidence,
// never silently mixes a reference document into canonical memory.
type ResearchOverlap struct {
	ReferenceID string `json:"referenceId"`
	Title       string `json:"title"`
	Quote       string `json:"quote"`
	Length      int    `json:"length"`
}

func (s *Service) ReferenceOverlap(ctx context.Context, pid, revisionID string) ([]ResearchOverlap, error) {
	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return nil, err
	}
	chapter, err := s.GetChapter(ctx, revision.ChapterID)
	if err != nil || chapter.ProjectID != pid {
		return nil, ErrNotFound
	}
	refs, err := s.StudioReferences(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := []ResearchOverlap{}
	for _, ref := range refs {
		reference := []rune(ref.Text)
		if len(reference) > 500000 {
			return nil, fmt.Errorf("参考资料过长，请按片段检查")
		}
		seen := map[string]bool{}
		for start := 0; start+20 <= len(reference) && len(out) < 30; start += 10 {
			needle := string(reference[start : start+20])
			if strings.TrimSpace(needle) == "" || seen[needle] || !strings.Contains(revision.PlainText, needle) {
				continue
			}
			end := start + 20
			for end < len(reference) && end-start < 200 && strings.Contains(revision.PlainText, string(reference[start:end+1])) {
				end++
			}
			quote := string(reference[start:end])
			seen[needle] = true
			out = append(out, ResearchOverlap{ReferenceID: ref.ID, Title: ref.Title, Quote: quote, Length: end - start})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Length > out[j].Length })
	return out, nil
}
