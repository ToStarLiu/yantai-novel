package workbench

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/url"
	"novel/internal/job"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type MediaConfig struct {
	BaseURL, APIKey, ImageModel, SpeechModel string
	HTTPClient                               *http.Client
}
type MediaRequest struct {
	Kind         string `json:"kind"`
	ChapterID    string `json:"chapterId"`
	Prompt       string `json:"prompt"`
	Model        string `json:"model"`
	Voice        string `json:"voice"`
	Size         string `json:"size"`
	Instructions string `json:"instructions"`
}
type MediaAsset struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"projectId"`
	ChapterID         string          `json:"chapterId"`
	RevisionID        string          `json:"revisionId"`
	Kind              string          `json:"kind"`
	Status            string          `json:"status"`
	Input             MediaRequest    `json:"input"`
	Mime              string          `json:"mime"`
	Error             string          `json:"error"`
	CostStatus        string          `json:"costStatus"`
	CreatedAt         time.Time       `json:"createdAt"`
	ProviderRequestID string          `json:"providerRequestId"`
	Usage             json.RawMessage `json:"usage"`
}

const mediaColumns = `id,project_id,chapter_id,revision_id,kind,status,input,mime,error,cost_status,created_at,provider_request_id,usage`

func scanMedia(row pgx.Row) (MediaAsset, error) {
	var a MediaAsset
	var raw []byte
	err := row.Scan(&a.ID, &a.ProjectID, &a.ChapterID, &a.RevisionID, &a.Kind, &a.Status, &raw, &a.Mime, &a.Error, &a.CostStatus, &a.CreatedAt, &a.ProviderRequestID, &a.Usage)
	if err != nil {
		return a, noRows(err)
	}
	err = json.Unmarshal(raw, &a.Input)
	return a, err
}
func (s *Service) mediaConfig() MediaConfig {
	c := s.MediaConfig
	if c.BaseURL == "" {
		c.BaseURL = os.Getenv("MEDIA_BASE_URL")
		if c.BaseURL == "" {
			c.BaseURL = os.Getenv("LLM_BASE_URL")
		}
	}
	if c.APIKey == "" {
		c.APIKey = os.Getenv("MEDIA_API_KEY")
		if c.APIKey == "" {
			c.APIKey = os.Getenv("LLM_API_KEY")
		}
	}
	if c.ImageModel == "" {
		c.ImageModel = os.Getenv("IMAGE_MODEL")
	}
	if c.SpeechModel == "" {
		c.SpeechModel = os.Getenv("TTS_MODEL")
	}
	return c
}
func (s *Service) MediaCapabilities() map[string]any {
	c := s.mediaConfig()
	return map[string]any{"configured": c.BaseURL != "" && c.APIKey != "", "imageModel": c.ImageModel, "speechModel": c.SpeechModel, "maxSpeechCharacters": 4000, "costStatus": "Unknown"}
}
func (s *Service) SubmitMedia(ctx context.Context, pid string, in MediaRequest) (MediaAsset, error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return MediaAsset{}, err
	}
	c := s.mediaConfig()
	if in.Kind != "cover" && in.Kind != "tts" {
		return MediaAsset{}, fmt.Errorf("媒体类型应为cover或tts")
	}
	revisionID := ""
	if in.Kind == "tts" {
		if in.ChapterID != "" {
			ch, err := s.GetChapter(ctx, in.ChapterID)
			if err != nil {
				return MediaAsset{}, err
			}
			if ch.ProjectID != pid {
				return MediaAsset{}, ErrNotFound
			}
			if ch.Revision == nil {
				return MediaAsset{}, fmt.Errorf("请先保存章节正文")
			}
			in.Prompt = ch.Revision.PlainText
			revisionID = ch.Revision.ID
		}
		if in.Model == "" {
			in.Model = c.SpeechModel
		}
		if in.Voice == "" {
			in.Voice = "alloy"
		}
		if utf8.RuneCountInString(in.Prompt) > 4000 {
			return MediaAsset{}, fmt.Errorf("单次朗读最多4000字，请按场景分段")
		}
	} else {
		if in.Model == "" {
			in.Model = c.ImageModel
		}
		if in.Size == "" {
			in.Size = "1024x1536"
		}
		if in.Size != "1024x1536" && in.Size != "1024x1024" && in.Size != "1536x1024" {
			return MediaAsset{}, fmt.Errorf("封面尺寸无效")
		}
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 64000 || len(in.Instructions) > 4000 || in.Model == "" {
		return MediaAsset{}, fmt.Errorf("请填写内容并配置对应媒体模型")
	}
	if c.BaseURL == "" || c.APIKey == "" {
		return MediaAsset{}, fmt.Errorf("请配置媒体接口与密钥环境变量")
	}
	if pref, err := s.Preferences(ctx, pid); err != nil {
		return MediaAsset{}, err
	} else if pref.MaxCost != nil {
		return MediaAsset{}, fmt.Errorf("媒体调用单价未知，当前金额预算下不自动发送")
	}
	raw, _ := json.Marshal(in)
	id := job.NewID()
	_, err := s.DB.Exec(ctx, "INSERT INTO wb_media(id,project_id,chapter_id,revision_id,kind,status,input) VALUES($1,$2,$3,$4,$5,'Pending',$6)", id, pid, in.ChapterID, revisionID, in.Kind, raw)
	if err != nil {
		return MediaAsset{}, err
	}
	return s.GetMedia(ctx, pid, id)
}
func (s *Service) GetMedia(ctx context.Context, pid, id string) (MediaAsset, error) {
	return scanMedia(s.DB.QueryRow(ctx, "SELECT "+mediaColumns+" FROM wb_media WHERE project_id=$1 AND id=$2", pid, id))
}
func (s *Service) ListMedia(ctx context.Context, pid string) ([]MediaAsset, error) {
	rows, err := s.DB.Query(ctx, "SELECT "+mediaColumns+" FROM wb_media WHERE project_id=$1 ORDER BY created_at DESC LIMIT 100", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MediaAsset{}
	for rows.Next() {
		v, e := scanMedia(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) MediaContent(ctx context.Context, pid, id string) ([]byte, string, error) {
	var raw []byte
	var mime string
	err := s.DB.QueryRow(ctx, "SELECT content,mime FROM wb_media WHERE project_id=$1 AND id=$2 AND status='Succeeded'", pid, id).Scan(&raw, &mime)
	return raw, mime, noRows(err)
}
func (s *Service) CancelMedia(ctx context.Context, pid, id string) error {
	tag, err := s.DB.Exec(ctx, "UPDATE wb_media SET status='Cancelled',finished_at=now() WHERE id=$1 AND project_id=$2 AND status='Pending'", id, pid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return &Conflict{Message: "媒体调用已发出或已结束，不能安全取消；结果和费用会保留"}
	}
	return nil
}

func (s *Service) AdvanceMedia(ctx context.Context) error {
	// An interrupted external call is never repeated automatically.
	if _, err := s.DB.Exec(ctx, "UPDATE wb_media SET status='NeedsReconciliation',error='执行中断，媒体结果未知，请核对服务账单',finished_at=now() WHERE status='Running' AND started_at<now()-interval '6 minutes'"); err != nil {
		return err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	a, err := scanMedia(tx.QueryRow(ctx, "SELECT "+mediaColumns+" FROM wb_media WHERE status='Pending' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED"))
	if err == ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE wb_media SET status='Running',started_at=now() WHERE id=$1", a.ID); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	data, mime, id, usage, callErr := s.generateMedia(requestCtx, a.Input)
	status, message := "Succeeded", ""
	if callErr != nil {
		status = "NeedsReconciliation"
		message = callErr.Error()
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	if usage == nil {
		usage = []byte(`{}`)
	}
	_, err = s.DB.Exec(saveCtx, "UPDATE wb_media SET status=$2,content=$3,mime=$4,error=$5,provider_request_id=$6,usage=$7,finished_at=now() WHERE id=$1 AND status='Running'", a.ID, status, data, mime, message, id, usage)
	return err
}
func (s *Service) generateMedia(ctx context.Context, in MediaRequest) ([]byte, string, string, []byte, error) {
	c := s.mediaConfig()
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") || base.User != nil || base.RawQuery != "" {
		return nil, "", "", nil, fmt.Errorf("媒体接口配置无效")
	}
	suffix := "/images/generations"
	payload := map[string]any{"model": in.Model, "prompt": in.Prompt, "size": in.Size, "n": 1, "output_format": "png"}
	if in.Kind == "tts" {
		suffix = "/audio/speech"
		payload = map[string]any{"model": in.Model, "input": in.Prompt, "voice": in.Voice, "response_format": "mp3"}
		if in.Instructions != "" {
			payload["instructions"] = in.Instructions
		}
	}
	base.Path = strings.TrimRight(base.Path, "/") + suffix
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", base.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("媒体请求无效")
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if actor := os.Getenv("LLM_ACTOR_AUTHORIZATION"); actor != "" {
		req.Header.Set("x-openai-actor-authorization", actor)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("媒体网络请求结果未知")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 40<<20))
	if err != nil || len(body) >= 40<<20 {
		return nil, "", "", nil, fmt.Errorf("媒体响应中断或超出上限")
	}
	id := resp.Header.Get("x-request-id")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", id, nil, fmt.Errorf("媒体服务返回HTTP %d，未重试", resp.StatusCode)
	}
	if in.Kind == "tts" {
		mime := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
		if mime != "audio/mpeg" && mime != "audio/mp3" && mime != "audio/wav" && mime != "application/octet-stream" {
			return nil, "", id, nil, fmt.Errorf("服务未返回音频")
		}
		return body, "audio/mpeg", id, nil, nil
	}
	var output struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(body, &output) != nil || len(output.Data) == 0 || output.Data[0].B64 == "" {
		return nil, "", id, nil, fmt.Errorf("封面接口未返回base64图片")
	}
	image, err := base64.StdEncoding.DecodeString(output.Data[0].B64)
	if err != nil || len(image) > 32<<20 || !bytes.HasPrefix(image, []byte{137, 80, 78, 71}) {
		return nil, "", id, output.Usage, fmt.Errorf("封面图片格式无效")
	}
	return image, "image/png", id, output.Usage, nil
}
