package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"novel/internal/evaluation"
	"novel/internal/generation"
	"novel/internal/job"
	"novel/internal/llm"
	"novel/migrations"
)

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if len(os.Args) != 3 {
			return fmt.Errorf("usage: eval migrate up|down")
		}
		p, err := database(ctx)
		if err != nil {
			return err
		}
		defer p.Close()
		if err = migrations.Apply(ctx, p, os.Args[2]); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		fmt.Println("migration", os.Args[2], "completed")
		return nil
	}
	fixture := flag.String("fixture", "evaluation/fixtures/bridge-3", "fixture directory")
	provider := flag.String("provider", "fake", "fake or configured; configured explicitly permits external calls")
	output := flag.String("output", "evaluation/results/baseline-fake", "new result directory")
	intent := flag.String("intent", "", "stable business intent ID; repeated requests return saved output")
	repeat := flag.Int("repeat", 1, "independent repetitions, 1 to 3; not automatic retries")
	tokens := flag.Int("max-output-tokens", 1600, "per-call output cap, 1 to 4096")
	flag.Parse()
	if *provider != "fake" && *provider != "configured" {
		return fmt.Errorf("provider must be fake or configured")
	}
	if *repeat < 1 || *repeat > 3 || *tokens < 1 || *tokens > 4096 {
		return fmt.Errorf("repeat must be 1..3 and max-output-tokens 1..4096")
	}
	if *intent != "" && *repeat != 1 {
		return fmt.Errorf("an explicit intent requires repeat=1")
	}
	f, err := evaluation.Load(*fixture)
	if err != nil {
		return err
	}
	writer, err := os.ReadFile("prompts/writing/chapter-v1.md")
	if err != nil {
		return err
	}
	reviewer, err := os.ReadFile("prompts/review/continuity-v1.md")
	if err != nil {
		return err
	}
	schema, err := os.ReadFile("schemas/review-issues-v1.json")
	if err != nil {
		return err
	}
	model := "fake-v1"
	var impl llm.Provider
	endpoint, wire := "", "fake"
	if *provider == "configured" {
		key := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
		model = strings.TrimSpace(os.Getenv("LLM_MODEL"))
		endpoint = strings.TrimSpace(os.Getenv("LLM_BASE_URL"))
		wire = strings.TrimSpace(os.Getenv("LLM_WIRE_API"))
		if key == "" || model == "" || endpoint == "" {
			return fmt.Errorf("configured provider needs LLM_BASE_URL, LLM_MODEL and LLM_API_KEY; no external request was sent")
		}
		headers := map[string]string{}
		if v := os.Getenv("LLM_ACTOR_AUTHORIZATION"); v != "" {
			headers["x-openai-actor-authorization"] = v
		}
		impl, err = llm.NewCompatible(llm.Config{BaseURL: endpoint, APIKey: key, Model: model, WireAPI: wire, Headers: headers, Timeout: 120 * time.Second, MaxResponseBytes: 2 << 20})
		if err != nil {
			return err
		}
	} else {
		impl = &llm.Fake{Handler: func(_ context.Context, r llm.Request) (llm.Response, error) {
			text := f.CorrectDraft
			if len(r.ResponseSchema) > 0 {
				text = `{"issues":[]}`
			}
			return llm.Response{Text: text, Raw: []byte(text), ProviderRequestID: "fake-no-network"}, nil
		}}
	}
	// A pre-existing result directory is never overwritten by a new experiment.
	if _, err = os.Stat(*output); err == nil {
		return fmt.Errorf("output already exists; choose a new output directory")
	} else if !os.IsNotExist(err) {
		return err
	}
	p, err := database(ctx)
	if err != nil {
		return err
	}
	defer p.Close()
	if err = migrations.Check(ctx, p); err != nil {
		return err
	}
	if err = os.MkdirAll(*output, 0700); err != nil {
		return err
	}
	reports := []generation.Report{}
	var runErr error
	for i := 0; i < *repeat; i++ {
		intentID := *intent
		if intentID == "" {
			intentID = job.NewID()
		}
		r, e := generation.Run(ctx, generation.Config{Store: job.NewStore(p), Fixture: f, Provider: impl, IntentID: intentID, ProviderName: *provider, Model: model, Endpoint: endpoint, WireAPI: wire, WriterPrompt: string(writer), ReviewPrompt: string(reviewer), ReviewSchema: schema, MaxOutputTokens: *tokens})
		reports = append(reports, r)
		data, _ := json.MarshalIndent(r, "", "  ")
		if err = os.WriteFile(filepath.Join(*output, "run-"+strconv.Itoa(i+1)+".json"), data, 0600); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(*output, "draft-"+strconv.Itoa(i+1)+".md"), []byte(firstDraft(r)), 0600); err != nil {
			return err
		}
		fmt.Printf("run %d: %s, job=%s, saved-samples=%d\n", i+1, r.Status, r.RunID, len(r.Samples))
		if e != nil {
			runErr = e
			break
		}
	}
	if err = writeSummary(*output, reports, *provider); err != nil {
		return err
	}
	return runErr
}

func database(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, fmt.Errorf("DATABASE_URL is required; see scripts/dev.ps1")
	}
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = p.Ping(pingCtx); err != nil {
		p.Close()
		return nil, fmt.Errorf("database unavailable; start PostgreSQL and check DATABASE_URL")
	}
	return p, nil
}

func firstDraft(r generation.Report) string {
	if len(r.Samples) > 0 {
		return r.Samples[0].Text
	}
	return ""
}

func writeSummary(root string, reports []generation.Report, provider string) error {
	var b strings.Builder
	b.WriteString("# 阶段 00 运行结果\n\n")
	if provider == "fake" {
		b.WriteString("本次为假 Provider，仅验证任务、产物和校验流程。不能据此评价模型写作或矛盾检测效果。\n\n")
	}
	b.WriteString("配置：A-prefix-diagnostic（完整第 1、2 章前文）；尚未实施 A/B/C 增益对照。\n\n| 轮次 | 状态 | 请求数 | 输入 Token | 输出 Token | 费用 |\n|---|---|---:|---:|---:|---|\n")
	for i, r := range reports {
		var input, output int64
		known := len(r.Samples) > 0
		for _, s := range r.Samples {
			if s.InputTokens == nil || s.OutputTokens == nil {
				known = false
			} else {
				input += *s.InputTokens
				output += *s.OutputTokens
			}
		}
		in, out := "未知", "未知"
		if known {
			in = strconv.FormatInt(input, 10)
			out = strconv.FormatInt(output, 10)
		}
		fmt.Fprintf(&b, "| %d | %s | %d | %s | %s | 未知 |\n", i+1, r.Status, len(r.Samples), in, out)
	}
	b.WriteString("\n每轮输入、输出、调用 ID、提示版本、耗时与用量见 run-N.json；草稿见 draft-N.md。真实质量需逐项核对错误草稿检测与正确草稿误报，并由作者阅读候选稿。\n\n未配置提供商价格，费用为 null。没有执行自动重试或格式修复调用。\n")
	return os.WriteFile(filepath.Join(root, "summary.md"), []byte(b.String()), 0600)
}
