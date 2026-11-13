package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"novel/internal/job"
)

type StudioDocument[T any] struct {
	Version int64 `json:"version"`
	Data    T     `json:"data"`
}
type StudioWrite[T any] struct {
	ExpectedVersion int64 `json:"expectedVersion"`
	Data            T     `json:"data"`
}
type StudioNode struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Label           string   `json:"label"`
	X               float64  `json:"x"`
	Y               float64  `json:"y"`
	Content         string   `json:"content"`
	ChapterID       string   `json:"chapterId"`
	EntityID        string   `json:"entityId"`
	StoryTime       *int     `json:"storyTime"`
	ValidFrom       *int     `json:"validFrom"`
	ValidTo         *int     `json:"validTo"`
	PrerequisiteIDs []string `json:"prerequisiteIds"`
	ObservedBy      []string `json:"observedBy"`
}
type StudioEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
}
type StudioCondition struct {
	ID              string   `json:"id"`
	NodeID          string   `json:"nodeId"`
	RequiresNodeIDs []string `json:"requiresNodeIds"`
	Expression      string   `json:"expression"`
}
type StudioEnding struct {
	ID           string   `json:"id"`
	NodeID       string   `json:"nodeId"`
	Title        string   `json:"title"`
	ConditionIDs []string `json:"conditionIds"`
}
type StudioBoard struct {
	Nodes      []StudioNode      `json:"nodes"`
	Edges      []StudioEdge      `json:"edges"`
	Conditions []StudioCondition `json:"conditions"`
	Endings    []StudioEnding    `json:"endings"`
}
type BoardEvaluation struct {
	AvailableNodeIDs []string            `json:"availableNodeIds"`
	MatchedEndingIDs []string            `json:"matchedEndingIds"`
	Unmet            map[string][]string `json:"unmet"`
}
type StoryVolume struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Goal         string `json:"goal"`
	Summary      string `json:"summary"`
	ChapterCount int    `json:"chapterCount"`
}
type StoryBrief struct {
	PlotPlan          string        `json:"plotPlan"`
	Genre             string        `json:"genre"`
	Audience          string        `json:"audience"`
	TargetWords       int           `json:"targetWords"`
	TargetChapters    int           `json:"targetChapters"`
	POV               string        `json:"pov"`
	Platform          string        `json:"platform"`
	Theme             string        `json:"theme"`
	SellingPoint      string        `json:"sellingPoint"`
	CoreConflict      string        `json:"coreConflict"`
	ProtagonistDesire string        `json:"protagonistDesire"`
	FailureCost       string        `json:"failureCost"`
	EndingDirection   string        `json:"endingDirection"`
	WorldRules        []string      `json:"worldRules"`
	Relationships     string        `json:"relationships"`
	Restrictions      string        `json:"restrictions"`
	Style             string        `json:"style"`
	Pace              string        `json:"pace"`
	Volumes           []StoryVolume `json:"volumes"`
}
type SceneHandoff struct {
	EndLocation        string   `json:"endLocation"`
	PresentCharacters  []string `json:"presentCharacters"`
	NewFacts           []string `json:"newFacts"`
	InventoryChanges   []string `json:"inventoryChanges"`
	OpenActions        []string `json:"openActions"`
	RequiredContinuity []string `json:"requiredContinuity"`
	ForbiddenChanges   []string `json:"forbiddenChanges"`
}
type StudioScene struct {
	NodeIDs          []string     `json:"nodeIds,omitempty"`
	BoardVersion     int64        `json:"boardVersion,omitempty"`
	SourceRunID      string       `json:"sourceRunId,omitempty"`
	ID               string       `json:"id"`
	ChapterID        string       `json:"chapterId"`
	Order            int          `json:"order"`
	Title            string       `json:"title"`
	Goal             string       `json:"goal"`
	Conflict         string       `json:"conflict"`
	Causes           []string     `json:"causes"`
	ObservedBy       []string     `json:"observedBy"`
	ForbiddenChanges []string     `json:"forbiddenChanges"`
	Handoff          SceneHandoff `json:"handoff"`
}
type SceneCollection struct {
	Scenes []StudioScene `json:"scenes"`
}
type CharacterArc struct {
	EntityID            string  `json:"entityId"`
	Arc                 string  `json:"arc"`
	Emotion             string  `json:"emotion"`
	Goal                string  `json:"goal"`
	BeliefImportance    float64 `json:"beliefImportance"`
	RecencyWeight       float64 `json:"recencyWeight"`
	ForgetAfterChapters int     `json:"forgetAfterChapters"`
	Notes               string  `json:"notes"`
}
type CharacterArcs struct {
	Characters []CharacterArc `json:"characters"`
}
type StudioReference struct {
	ID         string            `json:"id"`
	Version    int64             `json:"version"`
	Title      string            `json:"title"`
	Text       string            `json:"text"`
	SourceNote string            `json:"sourceNote"`
	Analysis   ReferenceAnalysis `json:"analysis"`
	CreatedAt  time.Time         `json:"createdAt"`
}
type ReferenceSection struct {
	Title     string `json:"title"`
	Paragraph int    `json:"paragraph"`
}
type ReferenceTerm struct {
	Term  string `json:"term"`
	Count int    `json:"count"`
}
type ReferenceAnalysis struct {
	WordCount      int                `json:"wordCount"`
	ParagraphCount int                `json:"paragraphCount"`
	DialogueRatio  float64            `json:"dialogueRatio"`
	TopTerms       []ReferenceTerm    `json:"topTerms"`
	Structure      []ReferenceSection `json:"structure"`
	WritingProfile string             `json:"writingProfile"`
}
type ReferenceHit struct {
	ReferenceID string  `json:"referenceId"`
	Title       string  `json:"title"`
	Quote       string  `json:"quote"`
	Score       float64 `json:"score"`
}
type StudioComment struct {
	AuthorID      string    `json:"authorId,omitempty"`
	AuthorName    string    `json:"authorName,omitempty"`
	ID            string    `json:"id"`
	Version       int64     `json:"version"`
	RevisionID    string    `json:"revisionId"`
	PlainTextHash string    `json:"plainTextHash"`
	Quote         string    `json:"quote"`
	Prefix        string    `json:"prefix"`
	Suffix        string    `json:"suffix"`
	Text          string    `json:"text"`
	Resolved      bool      `json:"resolved"`
	AnchorStatus  string    `json:"anchorStatus"`
	CreatedAt     time.Time `json:"createdAt"`
}
type StudioCommentInput struct {
	RevisionID string `json:"revisionId"`
	Quote      string `json:"quote"`
	Prefix     string `json:"prefix"`
	Suffix     string `json:"suffix"`
	Text       string `json:"text"`
}
type BranchLink struct {
	ID              string    `json:"id"`
	SourceProjectID string    `json:"sourceProjectId"`
	TargetProjectID string    `json:"targetProjectId"`
	SourceEpoch     int64     `json:"sourceEpoch"`
	SourceTitle     string    `json:"sourceTitle"`
	Title           string    `json:"title"`
	CreatedAt       time.Time `json:"createdAt"`
}
type WikiSource struct {
	FactID       string `json:"factId"`
	RevisionID   string `json:"revisionId"`
	Quote        string `json:"quote"`
	ChapterOrder int    `json:"chapterOrder"`
	TruthStatus  string `json:"truthStatus"`
	CharacterID  string `json:"characterId"`
}
type WikiClaim struct {
	Predicate  string       `json:"predicate"`
	Object     string       `json:"object"`
	Supporting []WikiSource `json:"supporting"`
	Opposing   []WikiSource `json:"opposing"`
}
type WikiBelief struct {
	Fact                Fact    `json:"fact"`
	RecallWeight        float64 `json:"recallWeight"`
	ForgottenProjection bool    `json:"forgottenProjection"`
}
type WikiPage struct {
	EntityID    string       `json:"entityId"`
	Name        string       `json:"name"`
	Kind        string       `json:"kind"`
	AuthorNotes string       `json:"authorNotes"`
	Facts       []Fact       `json:"facts"`
	Claims      []WikiClaim  `json:"claims"`
	Supporting  []WikiSource `json:"supporting"`
	Opposing    []WikiSource `json:"opposing"`
	Beliefs     []WikiBelief `json:"beliefs"`
}
type SandboxParameters struct {
	InitialResources int    `json:"initialResources"`
	InitialTension   int    `json:"initialTension"`
	ResourceDelta    int    `json:"resourceDelta"`
	TensionDelta     int    `json:"tensionDelta"`
	EventLabel       string `json:"eventLabel"`
}
type SandboxActor struct {
	EntityID  string `json:"entityId"`
	Name      string `json:"name"`
	Resources int    `json:"resources"`
	Tension   int    `json:"tension"`
	Location  string `json:"location"`
	Goal      string `json:"goal"`
}
type SandboxState struct {
	Turn   int            `json:"turn"`
	Actors []SandboxActor `json:"actors"`
}
type SandboxEvent struct {
	ID       string `json:"id"`
	Turn     int    `json:"turn"`
	EntityID string `json:"entityId"`
	Action   string `json:"action"`
	Cause    string `json:"cause"`
	Result   string `json:"result"`
}
type SandboxDiff struct {
	EntityID string `json:"entityId"`
	Field    string `json:"field"`
	Before   int    `json:"before"`
	After    int    `json:"after"`
}
type SandboxCandidate struct {
	BaseCheckpointID string         `json:"baseCheckpointId"`
	Seed             int64          `json:"seed"`
	Before           SandboxState   `json:"before"`
	After            SandboxState   `json:"after"`
	Events           []SandboxEvent `json:"events"`
	Diff             []SandboxDiff  `json:"diff"`
}
type SandboxCheckpoint struct {
	ID        string         `json:"id"`
	State     SandboxState   `json:"state"`
	Events    []SandboxEvent `json:"events"`
	CreatedAt time.Time      `json:"createdAt"`
}
type Sandbox struct {
	ID              string              `json:"id"`
	Version         int64               `json:"version"`
	Title           string              `json:"title"`
	EntityIDs       []string            `json:"entityIds"`
	InitialEvidence []WikiSource        `json:"initialEvidence"`
	Parameters      SandboxParameters   `json:"parameters"`
	State           SandboxState        `json:"state"`
	Candidate       *SandboxCandidate   `json:"candidate"`
	History         []SandboxCheckpoint `json:"history"`
	Frozen          bool                `json:"frozen"`
	CreatedAt       time.Time           `json:"createdAt"`
}
type SandboxCreate struct {
	Title      string            `json:"title"`
	EntityIDs  []string          `json:"entityIds"`
	Steps      int               `json:"steps"`
	Seed       int64             `json:"seed"`
	Parameters SandboxParameters `json:"parameters"`
}
type StudioView struct {
	Board      StudioDocument[StudioBoard]     `json:"board"`
	Brief      StudioDocument[StoryBrief]      `json:"brief"`
	Scenes     StudioDocument[SceneCollection] `json:"scenes"`
	Arcs       StudioDocument[CharacterArcs]   `json:"arcs"`
	References []StudioReference               `json:"references"`
	Comments   []StudioComment                 `json:"comments"`
	Sandboxes  []Sandbox                       `json:"sandboxes"`
}

type StudioPromptSnapshot struct {
	Data     json.RawMessage  `json:"data"`
	Versions map[string]int64 `json:"versions"`
}

// StudioPromptContext freezes only author planning documents that actually
// influence generation. Canvas coordinates, private comments, references and
// speculative sandboxes remain outside factual generation context.
func (s *Service) StudioPromptContext(ctx context.Context, pid string) (StudioPromptSnapshot, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return StudioPromptSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1", pid).Scan(&id); err != nil {
		return StudioPromptSnapshot{}, noRows(err)
	}
	rows, err := tx.Query(ctx, "SELECT kind,version,data FROM wb_studio_documents WHERE project_id=$1 AND id='main' AND kind IN ('brief','scenes','arcs') ORDER BY kind", pid)
	if err != nil {
		return StudioPromptSnapshot{}, err
	}
	result := StudioPromptSnapshot{Versions: map[string]int64{"brief": 0, "scenes": 0, "arcs": 0}}
	data := map[string]json.RawMessage{}
	for rows.Next() {
		var kind string
		var version int64
		var raw json.RawMessage
		if err = rows.Scan(&kind, &version, &raw); err != nil {
			rows.Close()
			return result, err
		}
		result.Versions[kind] = version
		data[kind] = raw
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	result.Data, err = json.Marshal(data)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// ValidateStudioPromptTx takes a shared project lock. Studio writes take the
// matching exclusive lock, preventing a version change until the caller's
// result transaction commits.
func ValidateStudioPromptTx(ctx context.Context, tx pgx.Tx, pid string, versions map[string]int64) (bool, error) {
	if versions == nil {
		return true, nil
	}
	var id string
	if err := tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR SHARE", pid).Scan(&id); err == pgx.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	current := map[string]int64{"brief": 0, "scenes": 0, "arcs": 0}
	rows, err := tx.Query(ctx, "SELECT kind,version FROM wb_studio_documents WHERE project_id=$1 AND id='main' AND kind IN ('brief','scenes','arcs')", pid)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var kind string
		var version int64
		if err = rows.Scan(&kind, &version); err != nil {
			rows.Close()
			return false, err
		}
		current[kind] = version
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(versions) != 3 {
		return false, nil
	}
	for kind, version := range current {
		if versions[kind] != version {
			return false, nil
		}
	}
	return true, nil
}

func studioRead[T any](ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, pid, kind, id string, empty T) (StudioDocument[T], error) {
	result := StudioDocument[T]{Data: empty}
	var raw []byte
	err := db.QueryRow(ctx, "SELECT version,data FROM wb_studio_documents WHERE project_id=$1 AND kind=$2 AND id=$3", pid, kind, id).Scan(&result.Version, &raw)
	if err == pgx.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(raw, &result.Data)
	return result, err
}
func studioSaveTx[T any](ctx context.Context, tx pgx.Tx, pid, kind, id string, expected int64, data T) (StudioDocument[T], error) {
	var current int64
	err := tx.QueryRow(ctx, "SELECT version FROM wb_studio_documents WHERE project_id=$1 AND kind=$2 AND id=$3 FOR UPDATE", pid, kind, id).Scan(&current)
	if err != nil && err != pgx.ErrNoRows {
		return StudioDocument[T]{}, err
	}
	if current != expected {
		return StudioDocument[T]{}, &Conflict{Message: fmt.Sprintf("%s已更新，当前版本%d，请刷新后重试", kind, current)}
	}
	raw, err := json.Marshal(data)
	if err != nil || len(raw) > 8<<20 {
		return StudioDocument[T]{}, fmt.Errorf("工作室文档无效或超过8MiB")
	}
	version := current + 1
	if _, err = tx.Exec(ctx, "INSERT INTO wb_studio_documents(project_id,kind,id,version,data) VALUES($1,$2,$3,$4,$5) ON CONFLICT(project_id,kind,id) DO UPDATE SET version=$4,data=$5,updated_at=now()", pid, kind, id, version, raw); err != nil {
		return StudioDocument[T]{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO wb_studio_history(project_id,kind,id,version,data) VALUES($1,$2,$3,$4,$5)", pid, kind, id, version, raw); err != nil {
		return StudioDocument[T]{}, err
	}
	return StudioDocument[T]{Version: version, Data: data}, nil
}
func (s *Service) studioTx(ctx context.Context, pid string) (pgx.Tx, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var id string
	if err = tx.QueryRow(ctx, "SELECT id FROM wb_projects WHERE id=$1 FOR UPDATE", pid).Scan(&id); err != nil {
		tx.Rollback(ctx)
		return nil, noRows(err)
	}
	return tx, nil
}
func studioSave[T any](ctx context.Context, s *Service, pid, kind, id string, expected int64, data T) (StudioDocument[T], error) {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioDocument[T]{}, err
	}
	defer tx.Rollback(ctx)
	result, err := studioSaveTx(ctx, tx, pid, kind, id, expected, data)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (s *Service) Studio(ctx context.Context, pid string) (StudioView, error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return StudioView{}, err
	}
	v := StudioView{}
	var err error
	if v.Board, err = studioRead(ctx, s.DB, pid, "board", "main", StudioBoard{Nodes: []StudioNode{}, Edges: []StudioEdge{}, Conditions: []StudioCondition{}, Endings: []StudioEnding{}}); err != nil {
		return v, err
	}
	if v.Brief, err = studioRead(ctx, s.DB, pid, "brief", "main", StoryBrief{WorldRules: []string{}, Volumes: []StoryVolume{}}); err != nil {
		return v, err
	}
	if v.Scenes, err = studioRead(ctx, s.DB, pid, "scenes", "main", SceneCollection{Scenes: []StudioScene{}}); err != nil {
		return v, err
	}
	if v.Arcs, err = studioRead(ctx, s.DB, pid, "arcs", "main", CharacterArcs{Characters: []CharacterArc{}}); err != nil {
		return v, err
	}
	if v.References, err = s.StudioReferences(ctx, pid); err != nil {
		return v, err
	}
	if v.Comments, err = s.StudioComments(ctx, pid); err != nil {
		return v, err
	}
	v.Sandboxes, err = s.StudioSandboxes(ctx, pid)
	return v, err
}
func studioID(id string) bool {
	if len(id) == 0 || len(id) > 100 {
		return false
	}
	for _, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) < 1000000 }
func studioIDsUnique(ids []string, available map[string]bool) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] || !available[id] {
			return fmt.Errorf("引用不存在、重复或不属于本作品：%s", id)
		}
		seen[id] = true
	}
	return nil
}
func (s *Service) studioEntities(ctx context.Context, pid string) (map[string]Setting, map[string]bool, map[string]bool, error) {
	detail, err := s.GetProject(ctx, pid)
	if err != nil {
		return nil, nil, nil, err
	}
	entities := map[string]Setting{}
	characters := map[string]bool{}
	chapters := map[string]bool{}
	for _, e := range detail.Settings {
		entities[e.ID] = e
		if e.Kind == "character" {
			characters[e.ID] = true
		}
	}
	for _, c := range detail.Chapters {
		chapters[c.ID] = true
	}
	return entities, characters, chapters, nil
}
func (s *Service) SaveStudioBoard(ctx context.Context, pid string, in StudioWrite[StudioBoard]) (StudioDocument[StudioBoard], error) {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioDocument[StudioBoard]{}, err
	}
	defer tx.Rollback(ctx)
	b := in.Data
	if len(b.Nodes) > 2000 || len(b.Edges) > 6000 || len(b.Conditions) > 2000 || len(b.Endings) > 500 {
		return StudioDocument[StudioBoard]{}, fmt.Errorf("画布超过节点或边上限")
	}
	entities, characters, chapters, err := s.studioEntities(ctx, pid)
	if err != nil {
		return StudioDocument[StudioBoard]{}, err
	}
	nodes := map[string]bool{}
	allIDs := map[string]bool{}
	graph := map[string][]string{}
	for _, n := range b.Nodes {
		if !studioID(n.ID) || allIDs[n.ID] || !validName(n.Label) || !finite(n.X) || !finite(n.Y) || len(n.Content) > 200000 {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("节点ID、名称、位置或内容无效")
		}
		switch n.Type {
		case "plot", "scene", "character", "place", "faction", "event":
		default:
			return StudioDocument[StudioBoard]{}, fmt.Errorf("不支持的节点类型")
		}
		if n.ChapterID != "" && !chapters[n.ChapterID] {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("节点章节不属于本作品")
		}
		if n.EntityID != "" && entities[n.EntityID].ID == "" {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("节点实体不属于本作品")
		}
		if n.ValidFrom != nil && n.ValidTo != nil && *n.ValidFrom > *n.ValidTo {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("时间区间倒置")
		}
		nodes[n.ID] = true
		allIDs[n.ID] = true
		if err = studioIDsUnique(n.ObservedBy, characters); err != nil {
			return StudioDocument[StudioBoard]{}, err
		}
	}
	for _, n := range b.Nodes {
		if err = studioIDsUnique(n.PrerequisiteIDs, nodes); err != nil {
			return StudioDocument[StudioBoard]{}, err
		}
		for _, prereq := range n.PrerequisiteIDs {
			if prereq == n.ID {
				return StudioDocument[StudioBoard]{}, fmt.Errorf("节点不能依赖自己")
			}
			graph[n.ID] = append(graph[n.ID], prereq)
		}
	}
	for _, e := range b.Edges {
		if !studioID(e.ID) || allIDs[e.ID] || !nodes[e.Source] || !nodes[e.Target] || e.Source == e.Target {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("边ID或端点无效")
		}
		allIDs[e.ID] = true
		switch e.Kind {
		case "cause", "requires":
			graph[e.Target] = append(graph[e.Target], e.Source)
		case "relation", "sequence", "branch":
		default:
			return StudioDocument[StudioBoard]{}, fmt.Errorf("边类型必须为cause/requires/relation/sequence/branch")
		}
	}
	conditions := map[string]bool{}
	for _, c := range b.Conditions {
		if !studioID(c.ID) || allIDs[c.ID] || !nodes[c.NodeID] || len(c.Expression) > 10000 {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("分支条件无效")
		}
		if err = studioIDsUnique(c.RequiresNodeIDs, nodes); err != nil {
			return StudioDocument[StudioBoard]{}, err
		}
		allIDs[c.ID] = true
		conditions[c.ID] = true
		graph[c.NodeID] = append(graph[c.NodeID], c.RequiresNodeIDs...)
	}
	for _, e := range b.Endings {
		if !studioID(e.ID) || allIDs[e.ID] || !nodes[e.NodeID] || !validName(e.Title) {
			return StudioDocument[StudioBoard]{}, fmt.Errorf("结局节点无效")
		}
		allIDs[e.ID] = true
		if err = studioIDsUnique(e.ConditionIDs, conditions); err != nil {
			return StudioDocument[StudioBoard]{}, err
		}
	}
	if err = acyclic(graph); err != nil {
		return StudioDocument[StudioBoard]{}, err
	}
	scenes, err := studioRead(ctx, tx, pid, "scenes", "main", SceneCollection{})
	if err != nil {
		return StudioDocument[StudioBoard]{}, err
	}
	for _, scene := range scenes.Data.Scenes {
		if err = studioIDsUnique(scene.NodeIDs, nodes); err != nil {
			return StudioDocument[StudioBoard]{}, &Conflict{Message: "被场景引用的画布节点不能删除，请先编辑对应场景引用"}
		}
	}
	saved, err := studioSaveTx(ctx, tx, pid, "board", "main", in.ExpectedVersion, b)
	if err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}
func acyclic(graph map[string][]string) error {
	color := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if color[id] == 1 {
			return fmt.Errorf("前置条件或因果依赖存在循环")
		}
		if color[id] == 2 {
			return nil
		}
		color[id] = 1
		for _, p := range graph[id] {
			if err := visit(p); err != nil {
				return err
			}
		}
		color[id] = 2
		return nil
	}
	for id := range graph {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) EvaluateStudioBoard(ctx context.Context, pid string, completed []string) (BoardEvaluation, error) {
	doc, err := studioRead(ctx, s.DB, pid, "board", "main", StudioBoard{})
	if err != nil {
		return BoardEvaluation{}, err
	}
	nodes := map[string]bool{}
	done := map[string]bool{}
	for _, n := range doc.Data.Nodes {
		nodes[n.ID] = true
	}
	if err = studioIDsUnique(completed, nodes); err != nil {
		return BoardEvaluation{}, err
	}
	for _, id := range completed {
		done[id] = true
	}
	out := BoardEvaluation{AvailableNodeIDs: []string{}, MatchedEndingIDs: []string{}, Unmet: map[string][]string{}}
	conditions := map[string]bool{}
	for _, c := range doc.Data.Conditions {
		ok := true
		for _, id := range c.RequiresNodeIDs {
			if !done[id] {
				ok = false
			}
		}
		conditions[c.ID] = ok
	}
	for _, n := range doc.Data.Nodes {
		missing := []string{}
		for _, id := range n.PrerequisiteIDs {
			if !done[id] {
				missing = append(missing, id)
			}
		}
		for _, e := range doc.Data.Edges {
			if (e.Kind == "requires" || e.Kind == "cause") && e.Target == n.ID && !done[e.Source] {
				missing = append(missing, e.Source)
			}
		}
		for _, c := range doc.Data.Conditions {
			if c.NodeID == n.ID && !conditions[c.ID] {
				for _, id := range c.RequiresNodeIDs {
					if !done[id] {
						missing = append(missing, id)
					}
				}
			}
		}
		if len(missing) == 0 && !done[n.ID] {
			out.AvailableNodeIDs = append(out.AvailableNodeIDs, n.ID)
		} else if len(missing) > 0 {
			out.Unmet[n.ID] = missing
		}
	}
	for _, e := range doc.Data.Endings {
		ok := done[e.NodeID] && len(out.Unmet[e.NodeID]) == 0
		for _, id := range e.ConditionIDs {
			ok = ok && conditions[id]
		}
		if ok {
			out.MatchedEndingIDs = append(out.MatchedEndingIDs, e.ID)
		}
	}
	return out, nil
}
func (s *Service) SaveStudioBrief(ctx context.Context, pid string, in StudioWrite[StoryBrief]) (StudioDocument[StoryBrief], error) {
	b := in.Data
	if b.TargetWords < 0 || b.TargetWords > 100000000 || b.TargetChapters < 0 || b.TargetChapters > 100000 || len(b.Volumes) > 1000 || len(b.WorldRules) > 500 {
		return StudioDocument[StoryBrief]{}, fmt.Errorf("规划篇幅或卷纲规模无效")
	}
	ids := map[string]bool{}
	for _, v := range b.Volumes {
		if !studioID(v.ID) || ids[v.ID] || !validName(v.Title) || v.ChapterCount < 0 || v.ChapterCount > 100000 {
			return StudioDocument[StoryBrief]{}, fmt.Errorf("卷纲ID或篇幅无效")
		}
		ids[v.ID] = true
	}
	return studioSave(ctx, s, pid, "brief", "main", in.ExpectedVersion, b)
}
func (s *Service) SaveStudioScenes(ctx context.Context, pid string, in StudioWrite[SceneCollection]) (StudioDocument[SceneCollection], error) {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	defer tx.Rollback(ctx)
	_, characters, chapters, err := s.studioEntities(ctx, pid)
	if err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	if len(in.Data.Scenes) > 5000 {
		return StudioDocument[SceneCollection]{}, fmt.Errorf("场景数量超过上限")
	}
	ids := map[string]bool{}
	orders := map[string]bool{}
	graph := map[string][]string{}
	for _, scene := range in.Data.Scenes {
		if !studioID(scene.ID) || ids[scene.ID] || !chapters[scene.ChapterID] || scene.Order < 1 || !validName(scene.Title) {
			return StudioDocument[SceneCollection]{}, fmt.Errorf("场景ID、章节或顺序无效")
		}
		key := fmt.Sprint(scene.ChapterID, ":", scene.Order)
		if orders[key] {
			return StudioDocument[SceneCollection]{}, fmt.Errorf("同章场景顺序不能重复")
		}
		orders[key] = true
		ids[scene.ID] = true
		if err = studioIDsUnique(scene.ObservedBy, characters); err != nil {
			return StudioDocument[SceneCollection]{}, err
		}
		if err = studioIDsUnique(scene.Handoff.PresentCharacters, characters); err != nil {
			return StudioDocument[SceneCollection]{}, err
		}
	}
	for _, scene := range in.Data.Scenes {
		if err = studioIDsUnique(scene.Causes, ids); err != nil {
			return StudioDocument[SceneCollection]{}, err
		}
		graph[scene.ID] = scene.Causes
	}
	if err = acyclic(graph); err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	if err = validateSceneCanvasRefs(ctx, tx, pid, in.Data); err != nil {
		return StudioDocument[SceneCollection]{}, err
	}
	saved, err := studioSaveTx(ctx, tx, pid, "scenes", "main", in.ExpectedVersion, in.Data)
	if err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}
func (s *Service) SaveStudioArcs(ctx context.Context, pid string, in StudioWrite[CharacterArcs]) (StudioDocument[CharacterArcs], error) {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioDocument[CharacterArcs]{}, err
	}
	defer tx.Rollback(ctx)
	_, characters, _, err := s.studioEntities(ctx, pid)
	if err != nil {
		return StudioDocument[CharacterArcs]{}, err
	}
	seen := map[string]bool{}
	for _, c := range in.Data.Characters {
		if !characters[c.EntityID] || seen[c.EntityID] || c.BeliefImportance < 0 || c.BeliefImportance > 1 || c.RecencyWeight < 0 || c.RecencyWeight > 1 || math.IsNaN(c.BeliefImportance) || math.IsNaN(c.RecencyWeight) || c.ForgetAfterChapters < 0 || c.ForgetAfterChapters > 100000 {
			return StudioDocument[CharacterArcs]{}, fmt.Errorf("人物弧实体、权重或遗忘投影无效")
		}
		seen[c.EntityID] = true
	}
	saved, err := studioSaveTx(ctx, tx, pid, "arcs", "main", in.ExpectedVersion, in.Data)
	if err != nil {
		return saved, err
	}
	return saved, tx.Commit(ctx)
}

func studioList[T any](ctx context.Context, s *Service, pid, kind string) ([]T, error) {
	rows, err := s.DB.Query(ctx, "SELECT data,version FROM wb_studio_documents WHERE project_id=$1 AND kind=$2 ORDER BY created_at,id", pid, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var raw []byte
		var version int64
		if err = rows.Scan(&raw, &version); err != nil {
			return nil, err
		}
		var data map[string]json.RawMessage
		if err = json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		data["version"], _ = json.Marshal(version)
		raw, _ = json.Marshal(data)
		var value T
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
func (s *Service) StudioReferences(ctx context.Context, pid string) ([]StudioReference, error) {
	return studioList[StudioReference](ctx, s, pid, "reference")
}
func (s *Service) AddStudioReference(ctx context.Context, pid, title, text, sourceNote string) (StudioReference, error) {
	if !validName(title) || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || len(text) > 4<<20 || len(sourceNote) > 10000 {
		return StudioReference{}, fmt.Errorf("参考资料为空、格式无效或超过4MiB")
	}
	ref := StudioReference{ID: job.NewID(), Version: 1, Title: title, Text: text, SourceNote: sourceNote, Analysis: analyzeReference(text), CreatedAt: time.Now().UTC()}
	_, err := studioSave(ctx, s, pid, "reference", ref.ID, 0, ref)
	return ref, err
}
func analyzeReference(text string) ReferenceAnalysis {
	r := ReferenceAnalysis{WordCount: utf8.RuneCountInString(text), TopTerms: []ReferenceTerm{}, Structure: []ReferenceSection{}}
	counts := map[string]int{}
	quoted := false
	dialogue := 0
	for _, c := range text {
		if strings.ContainsRune("“「『", c) {
			quoted = true
		} else if strings.ContainsRune("”」』", c) {
			quoted = false
		} else if quoted {
			dialogue++
		}
	}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r.ParagraphCount++
		if strings.HasPrefix(line, "#") || chapterHeading.MatchString(line) {
			r.Structure = append(r.Structure, ReferenceSection{Title: strings.TrimSpace(strings.TrimLeft(line, "#")), Paragraph: r.ParagraphCount})
		}
		for _, term := range tokenize(line) {
			if utf8.RuneCountInString(term) >= 2 {
				counts[term]++
			}
		}
	}
	for term, count := range counts {
		r.TopTerms = append(r.TopTerms, ReferenceTerm{Term: term, Count: count})
	}
	sort.Slice(r.TopTerms, func(i, j int) bool {
		if r.TopTerms[i].Count != r.TopTerms[j].Count {
			return r.TopTerms[i].Count > r.TopTerms[j].Count
		}
		return r.TopTerms[i].Term < r.TopTerms[j].Term
	})
	if len(r.TopTerms) > 20 {
		r.TopTerms = r.TopTerms[:20]
	}
	if r.WordCount > 0 {
		r.DialogueRatio = float64(dialogue) / float64(r.WordCount)
	}
	average := 0
	if r.ParagraphCount > 0 {
		average = r.WordCount / r.ParagraphCount
	}
	r.WritingProfile = fmt.Sprintf("资料统计：共%d字符、%d段，平均每段约%d字符，对话引号内字符约%.1f%%。这些是结构特征，不是照搬表达的指令。", r.WordCount, r.ParagraphCount, average, r.DialogueRatio*100)
	return r
}
func (s *Service) SearchStudioReferences(ctx context.Context, pid, query string) ([]ReferenceHit, error) {
	if len(query) > 2000 {
		return nil, fmt.Errorf("参考查询过长")
	}
	if _, err := s.GetProject(ctx, pid); err != nil {
		return nil, err
	}
	refs, err := s.StudioReferences(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := []ReferenceHit{}
	for _, ref := range refs {
		for _, chunk := range chunks(ref.Text) {
			score, _ := relevance(chunk, query, tokenize(query), nil)
			if score > 0 {
				out = append(out, ReferenceHit{ReferenceID: ref.ID, Title: ref.Title, Quote: chunk, Score: score})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}
func (s *Service) DeleteStudioReference(ctx context.Context, pid, id string) error {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, "DELETE FROM wb_studio_documents WHERE project_id=$1 AND kind='reference' AND id=$2", pid, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, "DELETE FROM wb_studio_history WHERE project_id=$1 AND kind='reference' AND id=$2", pid, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) StudioComments(ctx context.Context, pid string) ([]StudioComment, error) {
	return studioList[StudioComment](ctx, s, pid, "comment")
}
func (s *Service) AddStudioComment(ctx context.Context, pid string, in StudioCommentInput) (StudioComment, error) {
	revision, err := s.GetRevision(ctx, in.RevisionID)
	if err != nil {
		return StudioComment{}, err
	}
	chapter, err := s.GetChapter(ctx, revision.ChapterID)
	if err != nil {
		return StudioComment{}, err
	}
	if chapter.ProjectID != pid {
		return StudioComment{}, ErrNotFound
	}
	if strings.TrimSpace(in.Quote) == "" || !strings.Contains(revision.PlainText, in.Quote) || strings.TrimSpace(in.Text) == "" || len(in.Text) > 50000 {
		return StudioComment{}, fmt.Errorf("批注引用不在指定正文或批注为空")
	}
	anchor := "Located"
	needle := in.Prefix + in.Quote + in.Suffix
	if !strings.Contains(revision.PlainText, needle) {
		return StudioComment{}, fmt.Errorf("批注上下文不匹配")
	}
	if strings.Count(revision.PlainText, needle) != 1 {
		anchor = "Ambiguous"
	}
	out := StudioComment{ID: job.NewID(), Version: 1, RevisionID: revision.ID, PlainTextHash: revision.Hash, Quote: in.Quote, Prefix: in.Prefix, Suffix: in.Suffix, Text: in.Text, AnchorStatus: anchor, CreatedAt: time.Now().UTC()}
	if p, ok := PrincipalFromContext(ctx); ok {
		out.AuthorID = p.ID
		out.AuthorName = p.DisplayName
	}
	_, err = studioSave(ctx, s, pid, "comment", out.ID, 0, out)
	return out, err
}
func (s *Service) UpdateStudioComment(ctx context.Context, pid, id string, expected int64, text string, resolved bool) (StudioComment, error) {
	doc, err := studioRead(ctx, s.DB, pid, "comment", id, StudioComment{})
	if err != nil {
		return doc.Data, err
	}
	if doc.Version == 0 {
		return doc.Data, ErrNotFound
	}
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin && p.ID != doc.Data.AuthorID {
		role, e := s.ProjectRole(ctx, pid)
		if e != nil {
			return doc.Data, e
		}
		if roleLevel(role) < roleLevel("editor") {
			return doc.Data, ErrForbidden
		}
	}
	if strings.TrimSpace(text) == "" || len(text) > 50000 {
		return doc.Data, fmt.Errorf("批注内容无效")
	}
	doc.Data.Text = text
	doc.Data.Resolved = resolved
	updated, err := studioSave(ctx, s, pid, "comment", id, expected, doc.Data)
	updated.Data.Version = updated.Version
	return updated.Data, err
}

func wikiSource(f Fact) WikiSource {
	return WikiSource{FactID: f.ID, RevisionID: f.SourceRevisionID, Quote: f.Evidence.Quote, ChapterOrder: f.ChapterOrder, TruthStatus: f.TruthStatus, CharacterID: f.CharacterID}
}
func (s *Service) StudioWiki(ctx context.Context, pid string) ([]WikiPage, error) {
	detail, err := s.GetProject(ctx, pid)
	if err != nil {
		return nil, err
	}
	state, err := s.State(ctx, pid)
	if err != nil {
		return nil, err
	}
	arcDoc, err := studioRead(ctx, s.DB, pid, "arcs", "main", CharacterArcs{})
	if err != nil {
		return nil, err
	}
	arcs := map[string]CharacterArc{}
	for _, a := range arcDoc.Data.Characters {
		arcs[a.EntityID] = a
	}
	pages := []WikiPage{}
	indices := map[string]int{}
	maxOrder := 0
	for _, c := range detail.Chapters {
		if c.Order > maxOrder {
			maxOrder = c.Order
		}
	}
	for _, entity := range detail.Settings {
		indices[entity.ID] = len(pages)
		pages = append(pages, WikiPage{EntityID: entity.ID, Name: entity.Name, Kind: entity.Kind, AuthorNotes: entity.Content, Facts: []Fact{}, Claims: []WikiClaim{}, Supporting: []WikiSource{}, Opposing: []WikiSource{}, Beliefs: []WikiBelief{}})
	}
	for _, f := range state.Facts {
		if index, ok := indices[f.Subject]; ok {
			pages[index].Facts = append(pages[index].Facts, f)
		}
		if f.Kind == "Knowledge" {
			if index, ok := indices[f.CharacterID]; ok {
				a := arcs[f.CharacterID]
				importance := a.BeliefImportance
				if importance == 0 {
					importance = .5
				}
				recency := a.RecencyWeight
				age := maxOrder - f.ChapterOrder
				if age < 0 {
					age = 0
				}
				weight := importance / (1 + recency*float64(age))
				forgotten := a.ForgetAfterChapters > 0 && age >= a.ForgetAfterChapters
				pages[index].Beliefs = append(pages[index].Beliefs, WikiBelief{Fact: f, RecallWeight: weight, ForgottenProjection: forgotten})
			}
		}
	}
	// Historical confirmations are grouped by exact predicate/value. Different
	// values are opposing sources to inspect, not automatic claims of a conflict.
	rows, err := s.DB.Query(ctx, `SELECT f.id,f.project_id,f.chapter_id,f.chapter_order,f.source_revision_id,f.state_id,f.kind,f.subject,f.predicate,f.object,f.character_id,f.truth_status,f.story_time,f.evidence FROM wb_facts f JOIN wb_states st ON st.id=f.state_id AND st.valid JOIN wb_chapters c ON c.id=f.chapter_id AND c.final_revision_id=f.source_revision_id AND NOT c.deleted WHERE f.project_id=$1 AND f.active ORDER BY f.chapter_order,f.created_at,f.id`, pid)
	if err != nil {
		return nil, err
	}
	facts := []Fact{}
	for rows.Next() {
		var f Fact
		var raw []byte
		if err = rows.Scan(&f.ID, &f.ProjectID, &f.ChapterID, &f.ChapterOrder, &f.SourceRevisionID, &f.StateID, &f.Kind, &f.Subject, &f.Predicate, &f.Object, &f.CharacterID, &f.TruthStatus, &f.StoryTime, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(raw, &f.Evidence); err != nil {
			rows.Close()
			return nil, err
		}
		facts = append(facts, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range pages {
		claims := map[string]int{}
		for _, f := range facts {
			if f.Subject != pages[i].EntityID {
				continue
			}
			source := wikiSource(f)
			if f.Kind == "Knowledge" && f.TruthStatus != "True" {
				pages[i].Opposing = append(pages[i].Opposing, source)
			} else {
				pages[i].Supporting = append(pages[i].Supporting, source)
			}
			key := f.Kind + "\x00" + f.Predicate + "\x00" + f.Object
			index, ok := claims[key]
			if !ok {
				index = len(pages[i].Claims)
				claims[key] = index
				pages[i].Claims = append(pages[i].Claims, WikiClaim{Predicate: f.Predicate, Object: f.Object, Supporting: []WikiSource{}, Opposing: []WikiSource{}})
			}
			pages[i].Claims[index].Supporting = append(pages[i].Claims[index].Supporting, source)
		}
		for ci := range pages[i].Claims {
			claim := &pages[i].Claims[ci]
			for _, f := range facts {
				if f.Subject == pages[i].EntityID && f.Predicate == claim.Predicate && f.Object != claim.Object {
					claim.Opposing = append(claim.Opposing, wikiSource(f))
				}
			}
		}
	}
	return pages, nil
}
func cloneSandboxState(state SandboxState) SandboxState {
	out := SandboxState{Turn: state.Turn, Actors: append([]SandboxActor{}, state.Actors...)}
	return out
}
func bounded(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
func simulateSandbox(box Sandbox, steps int, seed int64) (SandboxCandidate, error) {
	if box.Frozen {
		return SandboxCandidate{}, &Conflict{Message: "沙盒已冻结，不能继续模拟"}
	}
	if steps < 1 || steps > 20 {
		return SandboxCandidate{}, fmt.Errorf("模拟步数必须介于1至20")
	}
	if len(box.History) >= 500 {
		return SandboxCandidate{}, fmt.Errorf("沙盒检查点达到500，请创建新分支沙盒")
	}
	candidate := SandboxCandidate{Seed: seed, Before: cloneSandboxState(box.State), After: cloneSandboxState(box.State), Events: []SandboxEvent{}, Diff: []SandboxDiff{}}
	if len(box.History) > 0 {
		candidate.BaseCheckpointID = box.History[len(box.History)-1].ID
	}
	for step := 0; step < steps; step++ {
		candidate.After.Turn++
		for i := range candidate.After.Actors {
			actor := &candidate.After.Actors[i]
			oldResource, oldTension := actor.Resources, actor.Tension
			action := "推进目标"
			cost := 1 + int((uint64(seed)+uint64(candidate.After.Turn)+uint64(i))%3)
			effect := -1
			gain := 0
			if actor.Resources < cost {
				action = "休整并寻找补给"
				cost = 0
				effect = 3
				gain = 3
			} else if actor.Tension >= 70 {
				action = "谈判缓和冲突"
				cost *= 2
				effect = -5
			}
			actor.Resources = bounded(actor.Resources+box.Parameters.ResourceDelta+gain-cost, 0, 100000)
			actor.Tension = bounded(actor.Tension+box.Parameters.TensionDelta+effect, 0, 100)
			label := box.Parameters.EventLabel
			if label == "" {
				label = "局势推进"
			}
			candidate.Events = append(candidate.Events, SandboxEvent{ID: job.NewID(), Turn: candidate.After.Turn, EntityID: actor.EntityID, Action: label + "：" + actor.Name + action, Cause: fmt.Sprintf("作者参数资源增量%d、紧张增量%d；此前资源%d/紧张%d", box.Parameters.ResourceDelta, box.Parameters.TensionDelta, oldResource, oldTension), Result: fmt.Sprintf("付出资源%d，资源%d→%d，紧张%d→%d", cost, oldResource, actor.Resources, oldTension, actor.Tension)})
		}
	}
	for i, actor := range candidate.After.Actors {
		before := candidate.Before.Actors[i]
		if before.Resources != actor.Resources {
			candidate.Diff = append(candidate.Diff, SandboxDiff{EntityID: actor.EntityID, Field: "resources", Before: before.Resources, After: actor.Resources})
		}
		if before.Tension != actor.Tension {
			candidate.Diff = append(candidate.Diff, SandboxDiff{EntityID: actor.EntityID, Field: "tension", Before: before.Tension, After: actor.Tension})
		}
	}
	return candidate, nil
}
func (s *Service) StudioSandboxes(ctx context.Context, pid string) ([]Sandbox, error) {
	return studioList[Sandbox](ctx, s, pid, "sandbox")
}
func (s *Service) CreateStudioSandbox(ctx context.Context, pid string, in SandboxCreate) (Sandbox, error) {
	entities, _, _, err := s.studioEntities(ctx, pid)
	if err != nil {
		return Sandbox{}, err
	}
	if !validName(in.Title) || len(in.EntityIDs) < 1 || len(in.EntityIDs) > 30 {
		return Sandbox{}, fmt.Errorf("请选择1至30个实体并填写沙盒名称")
	}
	available := map[string]bool{}
	for id := range entities {
		available[id] = true
	}
	if err = studioIDsUnique(in.EntityIDs, available); err != nil {
		return Sandbox{}, err
	}
	params := in.Parameters
	if params.InitialResources < 0 || params.InitialResources > 1000 || params.InitialTension < 0 || params.InitialTension > 100 || params.ResourceDelta < -100 || params.ResourceDelta > 100 || params.TensionDelta < -100 || params.TensionDelta > 100 || len(params.EventLabel) > 1000 {
		return Sandbox{}, fmt.Errorf("沙盒参数超出范围")
	}
	state, err := s.State(ctx, pid)
	if err != nil {
		return Sandbox{}, err
	}
	arcs, err := studioRead(ctx, s.DB, pid, "arcs", "main", CharacterArcs{})
	if err != nil {
		return Sandbox{}, err
	}
	goals := map[string]string{}
	for _, a := range arcs.Data.Characters {
		goals[a.EntityID] = a.Goal
	}
	box := Sandbox{ID: job.NewID(), Version: 1, Title: in.Title, EntityIDs: append([]string{}, in.EntityIDs...), Parameters: params, State: SandboxState{Actors: []SandboxActor{}}, InitialEvidence: []WikiSource{}, History: []SandboxCheckpoint{}, CreatedAt: time.Now().UTC()}
	for _, id := range in.EntityIDs {
		actor := SandboxActor{EntityID: id, Name: entities[id].Name, Resources: params.InitialResources, Tension: params.InitialTension, Goal: goals[id]}
		for _, f := range state.Facts {
			if f.Subject == id && f.Kind != "Knowledge" {
				if f.Predicate == "location" {
					actor.Location = f.Object
				}
				if f.Predicate == "goal" {
					actor.Goal = f.Object
				}
				box.InitialEvidence = append(box.InitialEvidence, wikiSource(f))
			}
		}
		box.State.Actors = append(box.State.Actors, actor)
	}
	box.History = append(box.History, SandboxCheckpoint{ID: job.NewID(), State: cloneSandboxState(box.State), Events: []SandboxEvent{}, CreatedAt: time.Now().UTC()})
	if in.Steps > 0 {
		candidate, e := simulateSandbox(box, in.Steps, in.Seed)
		if e != nil {
			return box, e
		}
		box.Candidate = &candidate
	}
	_, err = studioSave(ctx, s, pid, "sandbox", box.ID, 0, box)
	return box, err
}
func (s *Service) StudioSandboxAction(ctx context.Context, pid, id, action string, expected int64, steps int, seed int64, checkpoint string) (Sandbox, error) {
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return Sandbox{}, err
	}
	defer tx.Rollback(ctx)
	doc, err := studioRead(ctx, tx, pid, "sandbox", id, Sandbox{})
	if err != nil {
		return doc.Data, err
	}
	if doc.Version == 0 {
		return doc.Data, ErrNotFound
	}
	if doc.Version != expected {
		return doc.Data, &Conflict{Message: "沙盒版本已变化，请刷新"}
	}
	box := doc.Data
	switch action {
	case "simulate":
		candidate, e := simulateSandbox(box, steps, seed)
		if e != nil {
			return box, e
		}
		box.Candidate = &candidate
	case "confirm":
		if box.Frozen {
			return box, &Conflict{Message: "沙盒已冻结"}
		}
		if box.Candidate == nil {
			return box, fmt.Errorf("没有可确认的模拟候选")
		}
		if len(box.History) == 0 || box.Candidate.BaseCheckpointID != box.History[len(box.History)-1].ID {
			return box, &Conflict{Message: "模拟候选基线已变化"}
		}
		box.State = cloneSandboxState(box.Candidate.After)
		box.History = append(box.History, SandboxCheckpoint{ID: job.NewID(), State: cloneSandboxState(box.State), Events: box.Candidate.Events, CreatedAt: time.Now().UTC()})
		box.Candidate = nil
	case "freeze":
		box.Frozen = true
		box.Candidate = nil
	case "rollback":
		index := -1
		for i, c := range box.History {
			if c.ID == checkpoint {
				index = i
				break
			}
		}
		if index < 0 {
			return box, fmt.Errorf("回滚检查点不存在")
		}
		box.State = cloneSandboxState(box.History[index].State)
		box.History = append(box.History, SandboxCheckpoint{ID: job.NewID(), State: cloneSandboxState(box.State), Events: []SandboxEvent{{ID: job.NewID(), Turn: box.State.Turn, Action: "作者回滚", Cause: checkpoint, Result: "仅恢复沙盒状态，正文与确认事实不变"}}, CreatedAt: time.Now().UTC()})
		box.Candidate = nil
	default:
		return box, fmt.Errorf("未知沙盒操作")
	}
	saved, err := studioSaveTx(ctx, tx, pid, "sandbox", id, expected, box)
	if err != nil {
		return box, err
	}
	box.Version = saved.Version
	return box, tx.Commit(ctx)
}

type studioArchiveDocument struct {
	Kind    string          `json:"kind"`
	ID      string          `json:"id"`
	Version int64           `json:"version"`
	Data    json.RawMessage `json:"data"`
}
type StudioArchive struct {
	SchemaVersion int                     `json:"schemaVersion"`
	ProjectID     string                  `json:"projectId"`
	Documents     []studioArchiveDocument `json:"documents"`
	History       []studioArchiveDocument `json:"history,omitempty"`
}

func (s *Service) exportStudioTx(ctx context.Context, tx pgx.Tx, pid string) (json.RawMessage, error) {
	out := StudioArchive{SchemaVersion: 1, ProjectID: pid, Documents: []studioArchiveDocument{}}
	rows, err := tx.Query(ctx, "SELECT kind,id,version,data FROM wb_studio_documents WHERE project_id=$1 ORDER BY kind,id", pid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d studioArchiveDocument
		if err = rows.Scan(&d.Kind, &d.ID, &d.Version, &d.Data); err != nil {
			return nil, err
		}
		out.Documents = append(out.Documents, d)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = tx.Query(ctx, "SELECT kind,id,version,data FROM wb_studio_history WHERE project_id=$1 ORDER BY kind,id,version", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d studioArchiveDocument
		if err = rows.Scan(&d.Kind, &d.ID, &d.Version, &d.Data); err != nil {
			return nil, err
		}
		out.History = append(out.History, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
func remapStudioValue(value any, ids map[string]string) any {
	switch v := value.(type) {
	case string:
		if id, ok := ids[v]; ok {
			return id
		}
		return v
	case []any:
		for i := range v {
			v[i] = remapStudioValue(v[i], ids)
		}
		return v
	case map[string]any:
		for key, x := range v {
			v[key] = remapStudioValue(x, ids)
		}
		return v
	}
	return value
}
func collectStudioIDs(value any, ids map[string]string) {
	switch v := value.(type) {
	case []any:
		for _, x := range v {
			collectStudioIDs(x, ids)
		}
	case map[string]any:
		if id, ok := v["id"].(string); ok && id != "" {
			if _, exists := ids[id]; !exists {
				ids[id] = job.NewID()
			}
		}
		for _, x := range v {
			collectStudioIDs(x, ids)
		}
	}
}
func (s *Service) ImportStudioTx(ctx context.Context, tx pgx.Tx, pid string, raw json.RawMessage, idMap map[string]string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if len(raw) > 16<<20 {
		return fmt.Errorf("工作室归档过大")
	}
	var archive StudioArchive
	if err := json.Unmarshal(raw, &archive); err != nil || archive.SchemaVersion != 1 || idMap[archive.ProjectID] != pid {
		return fmt.Errorf("工作室归档来源或版本无效")
	}
	if len(archive.Documents) > 5000 || len(archive.History) > 50000 {
		return fmt.Errorf("工作室归档文档过多")
	}
	parsed := []any{}
	historyParsed := []any{}
	seen := map[string]bool{}
	for _, doc := range archive.History {
		if doc.Version < 1 || doc.ID == "" {
			return fmt.Errorf("工作室历史版本无效")
		}
		var value any
		if err := json.Unmarshal(doc.Data, &value); err != nil {
			return err
		}
		collectStudioIDs(value, idMap)
		if doc.ID != "main" {
			if _, ok := idMap[doc.ID]; !ok {
				idMap[doc.ID] = job.NewID()
			}
		}
		historyParsed = append(historyParsed, value)
	}
	for _, doc := range archive.Documents {
		switch doc.Kind {
		case "board", "brief", "scenes", "arcs", "reference", "comment", "sandbox":
		default:
			return fmt.Errorf("未知工作室文档类型")
		}
		key := doc.Kind + "\x00" + doc.ID
		if doc.ID == "" || seen[key] || doc.Version < 1 {
			return fmt.Errorf("归档文档ID重复或版本无效")
		}
		seen[key] = true
		var value any
		if err := json.Unmarshal(doc.Data, &value); err != nil {
			return err
		}
		collectStudioIDs(value, idMap)
		if doc.ID != "main" {
			if _, ok := idMap[doc.ID]; !ok {
				idMap[doc.ID] = job.NewID()
			}
		}
		parsed = append(parsed, value)
	}
	for i, doc := range archive.Documents {
		value := remapStudioValue(parsed[i], idMap)
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		id := doc.ID
		if id != "main" {
			id = idMap[id]
		}
		if err = validateStudioArchiveDoc(ctx, tx, pid, doc.Kind, data); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO wb_studio_documents(project_id,kind,id,version,data) VALUES($1,$2,$3,$4,$5)", pid, doc.Kind, id, doc.Version, data); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO wb_studio_history(project_id,kind,id,version,data) VALUES($1,$2,$3,$4,$5)", pid, doc.Kind, id, doc.Version, data); err != nil {
			return err
		}
	}
	versionsSeen := map[string]bool{}
	for i, doc := range archive.History {
		key := fmt.Sprintf("%s\x00%s\x00%d", doc.Kind, doc.ID, doc.Version)
		if versionsSeen[key] || !seen[doc.Kind+"\x00"+doc.ID] {
			return fmt.Errorf("工作室历史重复或没有活动文档")
		}
		versionsSeen[key] = true
		var activeVersion int64
		for _, d := range archive.Documents {
			if d.Kind == doc.Kind && d.ID == doc.ID {
				activeVersion = d.Version
				break
			}
		}
		if doc.Version > activeVersion {
			return fmt.Errorf("历史版本超出活动版本")
		}
		data, err := json.Marshal(remapStudioValue(historyParsed[i], idMap))
		if err != nil {
			return err
		}
		// Historical deleted nodes may intentionally be absent from today's board.
		// Core project/revision/entity ownership and each snapshot's own graph remain validated.
		if err = validateStudioArchiveDoc(ctx, tx, pid, doc.Kind, data); err != nil {
			return err
		}
		id := doc.ID
		if id != "main" {
			id = idMap[id]
		}
		if doc.Version == activeVersion {
			var current json.RawMessage
			if err = tx.QueryRow(ctx, "SELECT data FROM wb_studio_documents WHERE project_id=$1 AND kind=$2 AND id=$3", pid, doc.Kind, id).Scan(&current); err != nil {
				return err
			}
			var a, b any
			_ = json.Unmarshal(data, &a)
			_ = json.Unmarshal(current, &b)
			sameA, _ := json.Marshal(a)
			sameB, _ := json.Marshal(b)
			if string(sameA) != string(sameB) {
				return fmt.Errorf("活动工作室版本与历史不一致")
			}
			continue
		}
		if _, err = tx.Exec(ctx, "INSERT INTO wb_studio_history(project_id,kind,id,version,data) VALUES($1,$2,$3,$4,$5)", pid, doc.Kind, id, doc.Version, data); err != nil {
			return err
		}
	}
	scenes, err := studioRead(ctx, tx, pid, "scenes", "main", SceneCollection{})
	if err != nil {
		return err
	}
	if err = validateSceneCanvasRefs(ctx, tx, pid, scenes.Data); err != nil {
		return err
	}
	return nil
}
func validateStudioArchiveDoc(ctx context.Context, tx pgx.Tx, pid, kind string, raw []byte) error {
	// Revalidate every core reference against the newly imported project. This
	// prevents a forged archive from preserving links to another author's draft.
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return err
	}
	var walk func(any) error
	walk = func(v any) error {
		switch value := v.(type) {
		case []any:
			for _, x := range value {
				if err := walk(x); err != nil {
					return err
				}
			}
		case map[string]any:
			for key, x := range value {
				if key == "observedBy" || key == "presentCharacters" || key == "entityIds" {
					if list, ok := x.([]any); ok {
						for _, item := range list {
							id, ok := item.(string)
							if !ok {
								return fmt.Errorf("归档实体引用必须为字符串")
							}
							var exists bool
							query := "SELECT EXISTS(SELECT 1 FROM wb_settings WHERE id=$1 AND project_id=$2)"
							if key != "entityIds" {
								query = "SELECT EXISTS(SELECT 1 FROM wb_settings WHERE id=$1 AND project_id=$2 AND kind='character')"
							}
							if err := tx.QueryRow(ctx, query, id, pid).Scan(&exists); err != nil {
								return err
							}
							if !exists {
								return fmt.Errorf("归档角色或实体列表不属于本作品")
							}
						}
					}
				}
				if text, ok := x.(string); ok && text != "" {
					table := ""
					switch key {
					case "chapterId":
						table = "wb_chapters"
					case "revisionId", "sourceRevisionId":
						table = "wb_revisions"
					case "entityId", "characterId":
						table = "wb_settings"
					}
					if table != "" {
						var exists bool
						query := "SELECT EXISTS(SELECT 1 FROM " + table + " WHERE id=$1 AND project_id=$2)"
						if table == "wb_revisions" {
							query = "SELECT EXISTS(SELECT 1 FROM wb_revisions r JOIN wb_chapters c ON c.id=r.chapter_id WHERE r.id=$1 AND c.project_id=$2)"
						}
						if err := tx.QueryRow(ctx, query, text, pid).Scan(&exists); err != nil {
							return err
						}
						if !exists {
							return fmt.Errorf("归档%s引用不属于本作品", key)
						}
					}
				}
				if err := walk(x); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return err
	}
	switch kind {
	case "board":
		var board StudioBoard
		if err := json.Unmarshal(raw, &board); err != nil {
			return err
		}
		ids := map[string]bool{}
		graph := map[string][]string{}
		for _, n := range board.Nodes {
			if !studioID(n.ID) || ids[n.ID] || !finite(n.X) || !finite(n.Y) || !validName(n.Label) {
				return fmt.Errorf("归档画布节点无效")
			}
			ids[n.ID] = true
		}
		for _, n := range board.Nodes {
			if err := studioIDsUnique(n.PrerequisiteIDs, ids); err != nil {
				return err
			}
			graph[n.ID] = append(graph[n.ID], n.PrerequisiteIDs...)
		}
		for _, edge := range board.Edges {
			if !ids[edge.Source] || !ids[edge.Target] {
				return fmt.Errorf("归档画布边引用悬空")
			}
			if edge.Kind == "cause" || edge.Kind == "requires" {
				graph[edge.Target] = append(graph[edge.Target], edge.Source)
			}
		}
		conditions := map[string]bool{}
		for _, condition := range board.Conditions {
			if !ids[condition.NodeID] || condition.ID == "" || conditions[condition.ID] {
				return fmt.Errorf("归档分支条件无效")
			}
			conditions[condition.ID] = true
			if err := studioIDsUnique(condition.RequiresNodeIDs, ids); err != nil {
				return err
			}
			graph[condition.NodeID] = append(graph[condition.NodeID], condition.RequiresNodeIDs...)
		}
		for _, ending := range board.Endings {
			if !ids[ending.NodeID] {
				return fmt.Errorf("归档结局悬空")
			}
			if err := studioIDsUnique(ending.ConditionIDs, conditions); err != nil {
				return err
			}
		}
		if err := acyclic(graph); err != nil {
			return err
		}
	case "scenes":
		var scenes SceneCollection
		if err := json.Unmarshal(raw, &scenes); err != nil {
			return err
		}
		ids := map[string]bool{}
		graph := map[string][]string{}
		orders := map[string]bool{}
		for _, scene := range scenes.Scenes {
			key := fmt.Sprint(scene.ChapterID, ":", scene.Order)
			if !studioID(scene.ID) || ids[scene.ID] || scene.Order < 1 || orders[key] || !validName(scene.Title) {
				return fmt.Errorf("归档场景无效")
			}
			ids[scene.ID] = true
			orders[key] = true
		}
		for _, scene := range scenes.Scenes {
			if err := studioIDsUnique(scene.Causes, ids); err != nil {
				return err
			}
			graph[scene.ID] = scene.Causes
		}
		if err := acyclic(graph); err != nil {
			return err
		}
	case "arcs":
		var arcs CharacterArcs
		if err := json.Unmarshal(raw, &arcs); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, arc := range arcs.Characters {
			if seen[arc.EntityID] || arc.BeliefImportance < 0 || arc.BeliefImportance > 1 || arc.RecencyWeight < 0 || arc.RecencyWeight > 1 || arc.ForgetAfterChapters < 0 {
				return fmt.Errorf("归档人物弧无效")
			}
			seen[arc.EntityID] = true
		}
	case "sandbox":
		var sandbox Sandbox
		if err := json.Unmarshal(raw, &sandbox); err != nil {
			return err
		}
		if len(sandbox.History) == 0 || len(sandbox.EntityIDs) == 0 {
			return fmt.Errorf("归档沙盒无初始状态")
		}
		selected := map[string]bool{}
		for _, id := range sandbox.EntityIDs {
			selected[id] = true
		}
		for _, actor := range sandbox.State.Actors {
			if !selected[actor.EntityID] || actor.Resources < 0 || actor.Tension < 0 || actor.Tension > 100 {
				return fmt.Errorf("归档沙盒状态不符")
			}
		}
	}
	if kind == "comment" {
		var comment StudioComment
		if err := json.Unmarshal(raw, &comment); err != nil {
			return err
		}
		var text, hash string
		if err := tx.QueryRow(ctx, "SELECT plain_text,hash FROM wb_revisions WHERE id=$1", comment.RevisionID).Scan(&text, &hash); err != nil || hash != comment.PlainTextHash || !strings.Contains(text, comment.Quote) {
			return fmt.Errorf("归档批注证据不匹配")
		}
	}
	return nil
}
func (s *Service) StudioBranches(ctx context.Context, pid string) ([]BranchLink, error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return nil, err
	}
	query := "SELECT id,source_project_id,target_project_id,source_epoch,source_title,title,created_at FROM wb_branch_links WHERE (source_project_id=$1 OR target_project_id=$1)"
	args := []any{pid}
	if p, ok := PrincipalFromContext(ctx); ok && !p.Admin {
		query += " AND source_project_id IN (SELECT project_id FROM wb_project_members WHERE user_id=$2) AND target_project_id IN (SELECT project_id FROM wb_project_members WHERE user_id=$2)"
		args = append(args, p.ID)
	}
	rows, err := s.DB.Query(ctx, query+" ORDER BY created_at", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BranchLink{}
	for rows.Next() {
		var link BranchLink
		if err = rows.Scan(&link.ID, &link.SourceProjectID, &link.TargetProjectID, &link.SourceEpoch, &link.SourceTitle, &link.Title, &link.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}
func (s *Service) ForkStudioProject(ctx context.Context, pid, title string, expectedEpoch int64) (ProjectDetail, error) {
	if !validName(title) {
		return ProjectDetail{}, fmt.Errorf("请填写分支名称")
	}
	archive, err := s.ExportProject(ctx, pid)
	if err != nil {
		return ProjectDetail{}, err
	}
	if archive.Project.Epoch != expectedEpoch {
		return ProjectDetail{}, &Conflict{Message: "原作品已变化，请刷新后创建分支"}
	}
	source := archive.Project
	archive.Project.Title = title
	imported, err := s.ImportProject(ctx, archive)
	if err != nil {
		return imported, err
	}
	_, err = s.DB.Exec(ctx, "INSERT INTO wb_branch_links(id,source_project_id,target_project_id,source_epoch,source_title,title) VALUES($1,$2,$3,$4,$5,$6)", job.NewID(), pid, imported.Project.ID, source.Epoch, source.Title, title)
	if err != nil {
		_, _ = s.DB.Exec(context.Background(), "DELETE FROM wb_projects WHERE id=$1", imported.Project.ID)
		return ProjectDetail{}, err
	}
	return imported, nil
}

func (s *Service) StudioBoardAt(ctx context.Context, pid string, day int) (StudioDocument[StudioBoard], error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return StudioDocument[StudioBoard]{}, err
	}
	doc, err := studioRead(ctx, s.DB, pid, "board", "main", StudioBoard{})
	if err != nil {
		return doc, err
	}
	full := doc.Data
	view := StudioBoard{Nodes: []StudioNode{}, Edges: []StudioEdge{}, Conditions: []StudioCondition{}, Endings: []StudioEnding{}}
	visible := map[string]bool{}
	for _, n := range full.Nodes {
		if n.StoryTime != nil && *n.StoryTime > day || n.ValidFrom != nil && *n.ValidFrom > day || n.ValidTo != nil && *n.ValidTo < day {
			continue
		}
		view.Nodes = append(view.Nodes, n)
		visible[n.ID] = true
	}
	for i := range view.Nodes {
		ids := []string{}
		for _, id := range view.Nodes[i].PrerequisiteIDs {
			if visible[id] {
				ids = append(ids, id)
			}
		}
		view.Nodes[i].PrerequisiteIDs = ids
	}
	for _, e := range full.Edges {
		if visible[e.Source] && visible[e.Target] {
			view.Edges = append(view.Edges, e)
		}
	}
	conditions := map[string]bool{}
	for _, c := range full.Conditions {
		if !visible[c.NodeID] {
			continue
		}
		all := true
		for _, id := range c.RequiresNodeIDs {
			all = all && visible[id]
		}
		if all {
			view.Conditions = append(view.Conditions, c)
			conditions[c.ID] = true
		}
	}
	for _, e := range full.Endings {
		if !visible[e.NodeID] {
			continue
		}
		all := true
		for _, id := range e.ConditionIDs {
			all = all && conditions[id]
		}
		if all {
			view.Endings = append(view.Endings, e)
		}
	}
	doc.Data = view
	return doc, nil
}

type StudioHistoryEntry struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id"`
	Version   int64           `json:"version"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"createdAt"`
}

func (s *Service) StudioHistory(ctx context.Context, pid, kind, id string) ([]StudioHistoryEntry, error) {
	if _, err := s.GetProject(ctx, pid); err != nil {
		return nil, err
	}
	switch kind {
	case "board", "brief", "scenes", "arcs":
	default:
		return nil, fmt.Errorf("该文档类型不支持历史恢复")
	}
	if id == "" {
		id = "main"
	}
	rows, err := s.DB.Query(ctx, "SELECT kind,id,version,data,created_at FROM wb_studio_history WHERE project_id=$1 AND kind=$2 AND id=$3 ORDER BY version DESC LIMIT 200", pid, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StudioHistoryEntry{}
	for rows.Next() {
		var v StudioHistoryEntry
		if err = rows.Scan(&v.Kind, &v.ID, &v.Version, &v.Data, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) RestoreStudioHistory(ctx context.Context, pid, kind, id string, expected, version int64) (StudioHistoryEntry, error) {
	switch kind {
	case "board", "brief", "scenes", "arcs":
	default:
		return StudioHistoryEntry{}, fmt.Errorf("该文档类型不支持历史恢复")
	}
	if id == "" {
		id = "main"
	}
	tx, err := s.studioTx(ctx, pid)
	if err != nil {
		return StudioHistoryEntry{}, err
	}
	defer tx.Rollback(ctx)
	var raw json.RawMessage
	if err = tx.QueryRow(ctx, "SELECT data FROM wb_studio_history WHERE project_id=$1 AND kind=$2 AND id=$3 AND version=$4", pid, kind, id, version).Scan(&raw); err != nil {
		return StudioHistoryEntry{}, noRows(err)
	}
	if err = validateStudioArchiveDoc(ctx, tx, pid, kind, raw); err != nil {
		return StudioHistoryEntry{}, err
	}
	saved, err := studioSaveTx(ctx, tx, pid, kind, id, expected, raw)
	if err != nil {
		return StudioHistoryEntry{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return StudioHistoryEntry{}, err
	}
	return StudioHistoryEntry{Kind: kind, ID: id, Version: saved.Version, Data: saved.Data, CreatedAt: time.Now().UTC()}, nil
}
