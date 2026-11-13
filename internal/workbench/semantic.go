package workbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"novel/internal/job"
)

type EmbeddingConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	APIKeyEnv string `json:"apiKeyEnv"`
	Model     string `json:"model"`
	Dimension int    `json:"dimension"`
	Kind      string `json:"kind"`
	Enabled   bool   `json:"enabled"`
}
type SemanticQuery struct {
	Query
	Mode                string `json:"mode"`
	EmbeddingProviderID string `json:"embeddingProviderId"`
	RerankProviderID    string `json:"rerankProviderId"`
}
type ContextSummary struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"projectId"`
	Epoch     int64      `json:"epoch"`
	Summary   string     `json:"summary"`
	Sources   []Citation `json:"sources"`
	Derived   bool       `json:"derived"`
}

func (s *Service) SaveEmbeddingConfig(ctx context.Context, c EmbeddingConfig) (EmbeddingConfig, error) {
	if !validName(c.Name) || !validEndpoint(c.BaseURL) || c.Model == "" || (c.Kind != "embedding" && c.Kind != "rerank") || c.Dimension < 1 || c.Dimension > 65536 {
		return c, fmt.Errorf("Embedding/Rerank配置无效")
	}
	if c.APIKeyEnv != "" && !envReference.MatchString(c.APIKeyEnv) {
		return c, fmt.Errorf("密钥必须引用环境变量")
	}
	c.ID = job.NewID()
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_embedding_configs(id,name,base_url,api_key_env,model,dimension,kind,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", c.ID, c.Name, c.BaseURL, c.APIKeyEnv, c.Model, c.Dimension, c.Kind, c.Enabled)
	return c, err
}
func (s *Service) ListEmbeddingConfigs(ctx context.Context) ([]EmbeddingConfig, error) {
	rows, err := s.DB.Query(ctx, "SELECT id,name,base_url,api_key_env,model,dimension,kind,enabled FROM wb_embedding_configs ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmbeddingConfig{}
	for rows.Next() {
		var c EmbeddingConfig
		if err = rows.Scan(&c.ID, &c.Name, &c.BaseURL, &c.APIKeyEnv, &c.Model, &c.Dimension, &c.Kind, &c.Enabled); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) embeddingConfig(ctx context.Context, id, kind string) (EmbeddingConfig, error) {
	configs, err := s.ListEmbeddingConfigs(ctx)
	if err != nil {
		return EmbeddingConfig{}, err
	}
	for _, c := range configs {
		if c.ID == id && c.Kind == kind && c.Enabled {
			return c, nil
		}
	}
	return EmbeddingConfig{}, fmt.Errorf("找不到启用的%s配置", kind)
}
func semanticHTTP(ctx context.Context, c EmbeddingConfig, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("语义接口URL无效")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKeyEnv != "" && os.Getenv(c.APIKeyEnv) == "" {
		return nil, fmt.Errorf("语义服务密钥环境变量未配置")
	}
	if key := os.Getenv(c.APIKeyEnv); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("语义服务请求失败，未自动重试")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, (16<<20)+1))
	if err != nil || len(body) > 16<<20 {
		return nil, fmt.Errorf("语义服务响应无效或过大")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("语义服务HTTP状态%d，未自动重试", res.StatusCode)
	}
	return body, nil
}
func embedTexts(ctx context.Context, c EmbeddingConfig, texts []string) ([][]float64, error) {
	if len(texts) == 0 || len(texts) > 64 {
		return nil, fmt.Errorf("Embedding批量需1至64段")
	}
	raw, err := semanticHTTP(ctx, c, "/embeddings", map[string]any{"model": c.Model, "input": texts, "encoding_format": "float", "dimensions": c.Dimension})
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.Data) != len(texts) {
		return nil, fmt.Errorf("Embedding数量不符")
	}
	vectors := make([][]float64, len(texts))
	for _, item := range response.Data {
		if item.Index < 0 || item.Index >= len(texts) || vectors[item.Index] != nil || len(item.Embedding) != c.Dimension {
			return nil, fmt.Errorf("Embedding索引或维度不符")
		}
		for _, value := range item.Embedding {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("Embedding包含无效数值")
			}
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}
func cosine(a, b []float64) (float64, error) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, fmt.Errorf("向量维度不同")
	}
	maxA, maxB := 0.0, 0.0
	for i, v := range a {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.IsNaN(b[i]) || math.IsInf(b[i], 0) {
			return 0, fmt.Errorf("向量含无效值")
		}
		maxA = math.Max(maxA, math.Abs(v))
		maxB = math.Max(maxB, math.Abs(b[i]))
	}
	if maxA == 0 || maxB == 0 {
		return 0, nil
	}
	// Scale first: valid finite vectors may contain values large enough that
	// naive squared norms overflow and yield a NaN ranking.
	dot, x, y := 0.0, 0.0, 0.0
	for i, v := range a {
		av, bv := v/maxA, b[i]/maxB
		dot += av * bv
		x += av * av
		y += bv * bv
	}
	return math.Max(-1, math.Min(1, dot/math.Sqrt(x*y))), nil
}
func (s *Service) semanticVisible(ctx context.Context, q Query) ([]Citation, error) {
	if q.Purpose != "" && q.Purpose != "AuthorReview" && q.Purpose != "CharacterKnowledge" {
		return nil, fmt.Errorf("检索用途无效")
	}
	var epoch int64
	if err := s.DB.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", q.ProjectID).Scan(&epoch); err != nil {
		return nil, noRows(err)
	}
	if q.ProjectEpoch != 0 && q.ProjectEpoch != epoch {
		return nil, &Conflict{Message: "作品版本已变化，请刷新语义检索"}
	}
	if q.Purpose == "CharacterKnowledge" {
		return s.Search(ctx, q)
	}
	before := 2147483647
	if q.BeforeChapterID != "" {
		if err := s.DB.QueryRow(ctx, "SELECT sort_order FROM wb_chapters WHERE id=$1 AND project_id=$2 AND NOT deleted", q.BeforeChapterID, q.ProjectID).Scan(&before); err != nil {
			return nil, fmt.Errorf("语义查询章节不属于作品")
		}
	}
	rows, err := s.DB.Query(ctx, `SELECT k.id,k.chapter_id,c.title,k.revision_id,c.sort_order,k.content,k.source_hash FROM wb_chunks k JOIN wb_chapters c ON c.id=k.chapter_id AND c.project_id=k.project_id AND c.final_revision_id=k.revision_id AND NOT c.deleted AND NOT c.needs_review JOIN wb_revisions r ON r.id=k.revision_id AND r.hash=k.source_hash WHERE k.project_id=$1 AND k.active AND c.sort_order<$2 AND EXISTS(SELECT 1 FROM wb_states st WHERE st.chapter_id=c.id AND st.source_revision_id=k.revision_id AND st.valid) ORDER BY c.sort_order,k.chunk_order LIMIT 10000`, q.ProjectID, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Citation{}
	for rows.Next() {
		c := Citation{ProjectID: q.ProjectID, Kind: "Passage"}
		if err = rows.Scan(&c.ID, &c.ChapterID, &c.ChapterTitle, &c.RevisionID, &c.ChapterOrder, &c.Content, &c.SourceHash); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) RebuildEmbeddings(ctx context.Context, projectID, configID string) (int, error) {
	config, err := s.embeddingConfig(ctx, configID, "embedding")
	if err != nil {
		return 0, err
	}
	if err = s.Reindex(ctx, projectID); err != nil {
		return 0, err
	}
	sources, err := s.semanticVisible(ctx, Query{ProjectID: projectID, Purpose: "AuthorReview"})
	if err != nil {
		return 0, err
	}
	count := 0
	for offset := 0; offset < len(sources); offset += 64 {
		end := offset + 64
		if end > len(sources) {
			end = len(sources)
		}
		texts := []string{}
		for _, source := range sources[offset:end] {
			texts = append(texts, source.Content)
		}
		vectors, err := embedTexts(ctx, config, texts)
		if err != nil {
			return count, err
		}
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return count, err
		}
		for i, source := range sources[offset:end] {
			raw, _ := json.Marshal(vectors[i])
			tag, saveErr := tx.Exec(ctx, `INSERT INTO wb_embeddings(chunk_id,config_id,model,dimension,source_hash,vector) SELECT k.id,$2,$3,$4,$5,$6 FROM wb_chunks k JOIN wb_chapters c ON c.id=k.chapter_id AND c.final_revision_id=k.revision_id AND NOT c.deleted AND NOT c.needs_review WHERE k.id=$1 AND k.active AND k.source_hash=$5 ON CONFLICT(chunk_id,config_id) DO UPDATE SET model=EXCLUDED.model,dimension=EXCLUDED.dimension,source_hash=EXCLUDED.source_hash,vector=EXCLUDED.vector,created_at=now()`, source.ID, config.ID, config.Model, config.Dimension, source.SourceHash, raw)
			err = saveErr
			if err != nil {
				break
			}
			count += int(tag.RowsAffected())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return count, err
		}
		if err = tx.Commit(ctx); err != nil {
			return count, err
		}
	}
	return count, nil
}
func (s *Service) SemanticSearch(ctx context.Context, in SemanticQuery) ([]Citation, error) {
	if in.Mode == "" {
		in.Mode = "keyword"
	}
	if in.Mode != "keyword" && in.Mode != "vector" && in.Mode != "hybrid" {
		return nil, fmt.Errorf("检索模式应为keyword/vector/hybrid")
	}
	keyword, err := s.Search(ctx, in.Query)
	if err != nil {
		return nil, err
	}
	result := keyword
	if in.Mode != "keyword" && in.Purpose != "CharacterKnowledge" {
		config, err := s.embeddingConfig(ctx, in.EmbeddingProviderID, "embedding")
		if err != nil {
			return nil, err
		}
		queryVectors, err := embedTexts(ctx, config, []string{in.Text})
		if err != nil {
			return nil, err
		}
		visible, err := s.semanticVisible(ctx, in.Query)
		if err != nil {
			return nil, err
		}
		vectorResults := []Citation{}
		visibleIDs := make([]string, 0, len(visible))
		byID := map[string]Citation{}
		for _, candidate := range visible {
			visibleIDs = append(visibleIDs, candidate.ID)
			byID[candidate.ID] = candidate
		}
		// Fetch vectors for the already filtered sources in one round trip.
		// One SELECT per chunk makes long manuscripts needlessly slow.
		rows, queryErr := s.DB.Query(ctx, "SELECT chunk_id,vector,model,dimension,source_hash FROM wb_embeddings WHERE config_id=$1 AND chunk_id=ANY($2::text[]) ORDER BY chunk_id", config.ID, visibleIDs)
		if queryErr != nil {
			return nil, queryErr
		}
		for rows.Next() {
			var id string
			var raw []byte
			var model string
			var dimension int
			var hash string
			if err = rows.Scan(&id, &raw, &model, &dimension, &hash); err != nil {
				rows.Close()
				return nil, err
			}
			candidate := byID[id]
			if model != config.Model || dimension != config.Dimension || hash != candidate.SourceHash {
				continue
			}
			var vector []float64
			if json.Unmarshal(raw, &vector) != nil {
				rows.Close()
				return nil, fmt.Errorf("已存向量格式无效")
			}
			score, err := cosine(queryVectors[0], vector)
			if err != nil {
				rows.Close()
				return nil, err
			}
			if score <= 0 {
				continue
			}
			candidate.Score = score
			candidate.Reason = "语义向量余弦相似度；模型=" + model
			vectorResults = append(vectorResults, candidate)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		sort.SliceStable(vectorResults, func(i, j int) bool { return vectorResults[i].Score > vectorResults[j].Score })
		if in.Mode == "vector" {
			result = vectorResults
		} else {
			ranks := map[string]float64{}
			candidates := map[string]Citation{}
			for _, list := range [][]Citation{keyword, vectorResults} {
				for rank, c := range list {
					ranks[c.ID] += 1 / float64(60+rank+1)
					candidates[c.ID] = c
				}
			}
			result = []Citation{}
			for id, c := range candidates {
				c.Score = ranks[id]
				c.Reason = "关键词与语义向量排名融合"
				result = append(result, c)
			}
			sort.SliceStable(result, func(i, j int) bool {
				if result[i].Score == result[j].Score {
					return result[i].ID < result[j].ID
				}
				return result[i].Score > result[j].Score
			})
		}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if len(result) > limit*3 {
		result = result[:limit*3]
	}
	if in.RerankProviderID != "" && len(result) > 0 {
		config, err := s.embeddingConfig(ctx, in.RerankProviderID, "rerank")
		if err != nil {
			return nil, err
		}
		documents := []string{}
		for _, c := range result {
			documents = append(documents, c.Content)
		}
		raw, err := semanticHTTP(ctx, config, "/rerank", map[string]any{"model": config.Model, "query": in.Text, "documents": documents, "top_n": limit})
		if err != nil {
			return nil, err
		}
		var response struct {
			Results []struct {
				Index int     `json:"index"`
				Score float64 `json:"relevance_score"`
			} `json:"results"`
		}
		if json.Unmarshal(raw, &response) != nil {
			return nil, fmt.Errorf("Rerank响应无效")
		}
		reranked := []Citation{}
		seen := map[int]bool{}
		for _, item := range response.Results {
			if item.Index < 0 || item.Index >= len(result) || seen[item.Index] || math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
				return nil, fmt.Errorf("Rerank索引或分数无效")
			}
			seen[item.Index] = true
			c := result[item.Index]
			c.Score = item.Score
			c.Reason += "；Rerank模型=" + config.Model
			reranked = append(reranked, c)
		}
		result = reranked
	}
	// Revalidate hard visibility after remote calls. A concurrent manuscript
	// change while embedding/rerank runs cannot return an invalid old source.
	current, err := s.semanticVisible(ctx, in.Query)
	if err != nil {
		return nil, err
	}
	allowed := map[string]string{}
	for _, c := range current {
		allowed[c.ID] = c.SourceHash
	}
	safe := []Citation{}
	for _, c := range result {
		if allowed[c.ID] == c.SourceHash {
			safe = append(safe, c)
		}
	}
	if len(safe) > limit {
		safe = safe[:limit]
	}
	return safe, nil
}
func (s *Service) CompressContext(ctx context.Context, q Query, maxRunes int) (ContextSummary, error) {
	if maxRunes < 100 || maxRunes > 20000 {
		return ContextSummary{}, fmt.Errorf("压缩预算应为100至20000字")
	}
	sources, err := s.Search(ctx, q)
	if err != nil {
		return ContextSummary{}, err
	}
	var b strings.Builder
	retained := []Citation{}
	for _, source := range sources {
		remaining := maxRunes - len([]rune(b.String()))
		marker := "[来源" + source.ID + "] "
		reserve := len([]rune(marker)) + 1
		if remaining <= reserve {
			break
		}
		runes := []rune(source.Content)
		if len(runes) > remaining-reserve {
			runes = runes[:remaining-reserve]
		}
		fmt.Fprintf(&b, "%s%s\n", marker, string(runes))
		retained = append(retained, source)
	}
	return s.SaveContextSummary(ctx, q.ProjectID, b.String(), retained)
}
func (s *Service) SaveContextSummary(ctx context.Context, pid, summary string, sources []Citation) (ContextSummary, error) {
	out := ContextSummary{ID: job.NewID(), ProjectID: pid, Summary: summary, Sources: sources, Derived: true}
	if len(summary) > 100000 || len(sources) > 100 {
		return out, fmt.Errorf("摘要或来源过大")
	}
	if err := s.DB.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", pid).Scan(&out.Epoch); err != nil {
		return out, err
	}
	if err := s.validateSummarySources(ctx, out); err != nil {
		return out, err
	}
	raw, _ := json.Marshal(sources)
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_context_summaries(id,project_id,epoch,summary,sources) VALUES($1,$2,$3,$4,$5)", out.ID, pid, out.Epoch, summary, raw)
	return out, err
}
func (s *Service) validateSummarySources(ctx context.Context, value ContextSummary) error {
	var epoch int64
	if err := s.DB.QueryRow(ctx, "SELECT epoch FROM wb_projects WHERE id=$1", value.ProjectID).Scan(&epoch); err != nil {
		return err
	}
	if epoch != value.Epoch {
		return &Conflict{Message: "摘要输入已过期，请重新压缩"}
	}
	for _, source := range value.Sources {
		if source.ProjectID != value.ProjectID {
			return fmt.Errorf("摘要跨作品来源被拒绝")
		}
		var exists bool
		if source.Kind == "Knowledge" {
			if err := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_facts f JOIN wb_states st ON st.id=f.state_id AND st.valid JOIN wb_chapters c ON c.id=f.chapter_id AND c.final_revision_id=f.source_revision_id AND NOT c.deleted WHERE f.id=$1 AND f.project_id=$2 AND f.source_revision_id=$3 AND f.active AND f.evidence->>'plainTextHash'=$4 AND c.sort_order=$5 AND c.id=$6 AND f.object=$7)", source.ID, value.ProjectID, source.RevisionID, source.SourceHash, source.ChapterOrder, source.ChapterID, source.Content).Scan(&exists); err != nil {
				return err
			}
		} else {
			if err := s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wb_chunks k JOIN wb_chapters c ON c.id=k.chapter_id AND c.final_revision_id=k.revision_id AND NOT c.deleted AND NOT c.needs_review WHERE k.id=$1 AND k.project_id=$2 AND k.revision_id=$3 AND k.source_hash=$4 AND k.active AND c.sort_order=$5 AND c.id=$6 AND k.content=$7)", source.ID, value.ProjectID, source.RevisionID, source.SourceHash, source.ChapterOrder, source.ChapterID, source.Content).Scan(&exists); err != nil {
				return err
			}
		}
		if !exists {
			return &Conflict{Message: "摘要来源已失效，请重新压缩"}
		}
	}
	return nil
}
func (s *Service) GetContextSummary(ctx context.Context, id string) (ContextSummary, error) {
	var out ContextSummary
	var sources []byte
	err := s.DB.QueryRow(ctx, "SELECT id,project_id,epoch,summary,sources FROM wb_context_summaries WHERE id=$1", id).Scan(&out.ID, &out.ProjectID, &out.Epoch, &out.Summary, &sources)
	if err != nil {
		return out, noRows(err)
	}
	if err = json.Unmarshal(sources, &out.Sources); err != nil {
		return out, err
	}
	out.Derived = true
	return out, s.validateSummarySources(ctx, out)
}

// SummaryForTask revalidates sources and the current chapter boundary. It can
// be used as derived context, but never replaces Revision evidence validation.
func (s *Service) SummaryForTask(ctx context.Context, id, projectID, chapterID string) (ContextSummary, error) {
	value, err := s.GetContextSummary(ctx, id)
	if err != nil {
		return value, err
	}
	if value.ProjectID != projectID {
		return value, fmt.Errorf("压缩摘要不属于本作品")
	}
	var order int
	if err = s.DB.QueryRow(ctx, "SELECT sort_order FROM wb_chapters WHERE id=$1 AND project_id=$2 AND NOT deleted", chapterID, projectID).Scan(&order); err != nil {
		return value, ErrNotFound
	}
	for _, source := range value.Sources {
		if source.ChapterOrder >= order {
			return value, &Conflict{Message: "压缩摘要包含当前章或未来章节信息"}
		}
	}
	return value, nil
}
