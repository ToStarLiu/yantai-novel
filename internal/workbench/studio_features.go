package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"novel/internal/job"
	"strings"
)

type CanvasTaskInput struct {
	ChapterID       string   `json:"chapterId"`
	NodeIDs         []string `json:"nodeIds"`
	InputRevisionID string   `json:"inputRevisionId"`
	IntentID        string   `json:"intentId"`
	Instructions    string   `json:"instructions"`
}
type ApplyVoiceInput struct {
	RunID   string `json:"runId"`
	Name    string `json:"name"`
	Content string `json:"content"`
}
type ApplySceneInput struct {
	ExpectedVersion int64       `json:"expectedVersion"`
	RunID           string      `json:"runId"`
	Scene           StudioScene `json:"scene"`
}

// Selected elements are an author planning source, never confirmed story facts.
// Position is retained as composition evidence; geometric distance is not truth.
func (s *Service) CanvasSelectionPrompt(ctx context.Context, pid string, nodeIDs []string) (string, int64, error) {
	board, err := studioRead(ctx, s.DB, pid, "board", "main", StudioBoard{})
	if err != nil {
		return "", 0, err
	}
	if board.Version == 0 || len(nodeIDs) < 1 || len(nodeIDs) > 20 {
		return "", 0, fmt.Errorf("请选择1至20个已保存画布节点")
	}
	selected := map[string]bool{}
	available := map[string]StudioNode{}
	for _, node := range board.Data.Nodes {
		available[node.ID] = node
	}
	nodes := []StudioNode{}
	for _, id := range nodeIDs {
		node, ok := available[id]
		if !ok || selected[id] {
			return "", 0, fmt.Errorf("画布节点不存在、重复或不属于本作品")
		}
		selected[id] = true
		nodes = append(nodes, node)
	}
	edges := []StudioEdge{}
	for _, edge := range board.Data.Edges {
		if selected[edge.Source] || selected[edge.Target] {
			edges = append(edges, edge)
		}
	}
	raw, err := json.Marshal(map[string]any{"boardVersion": board.Version, "selectedNodeIds": nodeIDs, "nodes": nodes, "relatedEdges": edges})
	return "作者选择的画布规划元素（仅候选，不是已发生事实；未选节点ID只有关系端点含义）：\n" + string(raw), board.Version, err
}
func ValidateCanvasSnapshotTx(ctx context.Context, tx pgx.Tx, pid string, version int64) (bool, error) {
	var exists string
	if err := tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR SHARE", pid).Scan(&exists); err != nil {
		return false, noRows(err)
	}
	var current int64
	err := tx.QueryRow(ctx, "SELECT COALESCE((SELECT version FROM wb_studio_documents WHERE project_id=$1 AND kind='board' AND id='main'),0)", pid).Scan(&current)
	return err == nil && current == version, err
}

func (s *Service) SubmitCanvasTask(ctx context.Context, pid string, in CanvasTaskInput, feature string) (Run, error) {
	chapter, err := s.GetChapter(ctx, in.ChapterID)
	if err != nil {
		return Run{}, err
	}
	if chapter.ProjectID != pid {
		return Run{}, ErrNotFound
	}
	_, version, err := s.CanvasSelectionPrompt(ctx, pid, in.NodeIDs)
	if err != nil {
		return Run{}, err
	}
	kind := "planning"
	instructions := "请根据作者选中的节点提出一个可采用的场景方案，明确场景目标、冲突、因果前置、参与者与禁止变化；不要假设节点计划已经发生，不要改写已确认事实。"
	if feature == "relation-explanation" {
		kind = "analysis"
		instructions = "请解释选中节点已有关系的可能叙事意义，区分作者连线、正文确认来源与推测；只提出关系解释候选，不新增、删除或提交任何关系或事实。"
	} else if feature != "canvas-scene" {
		return Run{}, fmt.Errorf("未知画布操作")
	}
	scope := ""
	if kind == "planning" {
		scope = "chapter"
	}
	return s.SubmitRun(ctx, in.ChapterID, RunRequest{IntentID: in.IntentID, TaskType: kind, InputRevisionID: in.InputRevisionID, Instructions: instructions + "\n作者补充：" + in.Instructions, Scope: scope, CanvasNodeIDs: in.NodeIDs, CanvasBoardVersion: version, Feature: feature})
}
func (s *Service) SubmitHookReview(ctx context.Context, cid string, in RunRequest) (Run, error) {
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		c, e := s.GetChapter(ctx, cid)
		if e != nil {
			return Run{}, e
		}
		if e = s.RequireProjectRole(ctx, c.ProjectID, "reviewer"); e != nil {
			return Run{}, e
		}
	}
	in.TaskType = "developmental"
	in.Feature = "hook"
	in.Instructions = "专项检查本章章末悬念与连载钩子。只输出候选报告，包含正文原句证据、未决问题、读者继续阅读动机、与章纲/角色知识是否冲突及可选改进。没有足够证据时明确说明；不可用关键词出现次数给文学总分，不改正文。\n作者补充：" + in.Instructions
	return s.SubmitRun(ctx, cid, in)
}
func (s *Service) ApplyVoiceProfile(ctx context.Context, pid string, in ApplyVoiceInput) (Setting, error) {
	if !validName(in.Name) || strings.TrimSpace(in.Content) == "" || len(in.Content) > 200000 {
		return Setting{}, fmt.Errorf("文风Profile需要名称与最多200000字节的作者确认内容")
	}
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return Setting{}, err
	}
	defer tx.Rollback(ctx)
	run, err := scanRun(tx.QueryRow(ctx, "SELECT "+runColumns+" FROM wb_runs WHERE id=$1 AND project_id=$2", in.RunID, pid))
	if err != nil {
		return Setting{}, err
	}
	if run.Type != "voice" || run.Status != "Succeeded" || run.Stale || run.Output.Text == "" {
		return Setting{}, &Conflict{Message: "作者声音候选来源无效或尚未完成"}
	}
	valid, err := snapshotCurrent(ctx, tx, run)
	if err != nil {
		return Setting{}, err
	}
	if !valid {
		return Setting{}, &Conflict{Message: "作者声音候选所依据的正文、设定或规划已变化"}
	}
	setting := Setting{ID: job.NewID(), ProjectID: pid, Kind: "style", Name: strings.TrimSpace(in.Name), Content: in.Content}
	_, err = tx.Exec(ctx, "INSERT INTO wb_settings(id,project_id,kind,name,content) VALUES($1,$2,'style',$3,$4)", setting.ID, pid, setting.Name, setting.Content)
	if err != nil {
		return setting, err
	}
	if err = s.settingImpactTx(ctx, tx, pid); err != nil {
		return setting, err
	}
	return setting, tx.Commit(ctx)
}
func (s *Service) ApplyCanvasScene(ctx context.Context, pid string, in ApplySceneInput) (StudioDocument[SceneCollection], error) {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	defer tx.Rollback(ctx)
	run, err := scanRun(tx.QueryRow(ctx, "SELECT "+runColumns+" FROM wb_runs WHERE id=$1 AND project_id=$2", in.RunID, pid))
	if err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	var meta struct {
		NodeIDs      []string `json:"canvasNodeIds"`
		BoardVersion int64    `json:"canvasBoardVersion"`
		Feature      string   `json:"feature"`
	}
	raw, _ := json.Marshal(run.Input)
	_ = json.Unmarshal(raw, &meta)
	if run.Type != "planning" || run.Status != "Succeeded" || run.Stale || run.Output.Text == "" || meta.Feature != "canvas-scene" || len(meta.NodeIDs) == 0 || in.Scene.ChapterID != run.ChapterID {
		return StudioDocument[SceneCollection]{}, &Conflict{Message: "场景候选来源或章节不匹配"}
	}
	valid, err := snapshotCurrent(ctx, tx, run)
	if err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	if !valid {
		return StudioDocument[SceneCollection]{}, &Conflict{Message: "场景候选输入已经变化"}
	}
	valid, err = ValidateCanvasSnapshotTx(ctx, tx, pid, meta.BoardVersion)
	if err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	if !valid {
		return StudioDocument[SceneCollection]{}, &Conflict{Message: "画布已变化，请重新生成场景候选"}
	}
	doc, err := studioRead(ctx, tx, pid, "scenes", "main", SceneCollection{Scenes: []StudioScene{}})
	if err != nil {
		return doc, err
	}
	scene := in.Scene
	if scene.ID == "" {
		scene.ID = job.NewID()
	}
	scene.NodeIDs = meta.NodeIDs
	scene.BoardVersion = meta.BoardVersion
	scene.SourceRunID = run.ID
	if len(scene.Goal) > 200000 || len(scene.Conflict) > 200000 {
		return doc, fmt.Errorf("场景目标或冲突超过200000字节")
	}
	doc.Data.Scenes = append(doc.Data.Scenes, scene)
	raw, _ = json.Marshal(doc.Data)
	if err = validateStudioArchiveDoc(ctx, tx, pid, "scenes", raw); err != nil {
		return doc, err
	}
	if err = validateSceneCanvasRefs(ctx, tx, pid, doc.Data); err != nil {
		return doc, err
	}
	saved, err := studioSaveTx(ctx, tx, pid, "scenes", "main", in.ExpectedVersion, doc.Data)
	if err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}
func validateSceneCanvasRefs(ctx context.Context, db snapshotDB, pid string, scenes SceneCollection) error {
	board, err := studioRead(ctx, db, pid, "board", "main", StudioBoard{})
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, n := range board.Data.Nodes {
		ids[n.ID] = true
	}
	for _, scene := range scenes.Scenes {
		if err = studioIDsUnique(scene.NodeIDs, ids); err != nil {
			return err
		}
	}
	return nil
}
