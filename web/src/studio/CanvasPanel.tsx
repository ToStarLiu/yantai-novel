import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  addEdge,
  applyNodeChanges,
  applyEdgeChanges,
  type Node,
  type Edge,
  type NodeChange,
  type EdgeChange,
  type Connection,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Plus, Save, Trash2, Sparkles, Check, Play, Link2 } from "lucide-react";
import { api, id, type Run } from "../api";
import {
  StudioButton,
  StudioCard,
  StudioError,
  Result,
  ChapterSelect,
  useStudioAction,
  type PanelProps,
} from "../Studio";
import { strings } from "../studioApi";
export type BoardNode = {
  id: string;
  type: string;
  label: string;
  x: number;
  y: number;
  content: string;
  chapterId: string;
  entityId: string;
  storyTime: number | null;
  validFrom: number | null;
  validTo: number | null;
  prerequisiteIds: string[];
  observedBy: string[];
};
type BoardEdge = {
  id: string;
  source: string;
  target: string;
  kind: string;
  label: string;
};
type Board = {
  nodes: BoardNode[];
  edges: BoardEdge[];
  conditions: {
    id: string;
    nodeId: string;
    requiresNodeIds: string[];
    expression: string;
  }[];
  endings: {
    id: string;
    nodeId: string;
    title: string;
    conditionIds: string[];
  }[];
};
type Studio = { board: { version: number; data: Board } };
export default function CanvasPanel({
  detail,
  chapterId,
  notify,
  refresh,
}: PanelProps) {
  const query = useQuery({
    queryKey: ["studio", detail.project.id],
    queryFn: () => api<Studio>(`/projects/${detail.project.id}/studio`),
  });
  const [nodes, setNodes] = useState<Node[]>([]),
    [edges, setEdges] = useState<Edge[]>([]),
    [selected, setSelected] = useState(""),
    [version, setVersion] = useState(0),
    [time, setTime] = useState(0),
    [timeFilter, setTimeFilter] = useState(false),
    [completed, setCompleted] = useState<string[]>([]),
    [evaluation, setEvaluation] = useState<unknown>(null),
    [result, setResult] = useState<Run | null>(null);
  const [sceneTitle, setSceneTitle] = useState(""),
    [sceneGoal, setSceneGoal] = useState(""),
    [targetChapter, setTargetChapter] = useState(chapterId);
  const [flow, setFlow] = useState<ReactFlowInstance | null>(null);
  useEffect(() => {
    const timer = setTimeout(
      () => void flow?.fitView({ padding: 0.28, duration: 200 }),
      80,
    );
    return () => clearTimeout(timer);
  }, [nodes.length, flow]);
  const task = useQuery({
    queryKey: ["studio", detail.project.id, "node-run", result?.id],
    queryFn: () => api<Run>(`/runs/${result!.id}`),
    enabled: !!result?.id,
    refetchInterval: 2000,
  });
  const activeRun = task.data || result;
  const [conditions, setConditions] = useState<Board["conditions"]>([]),
    [endings, setEndings] = useState<Board["endings"]>([]),
    [selectedEdge, setSelectedEdge] = useState("");
  const { working, act } = useStudioAction(notify, refresh);
  useEffect(() => {
    if (query.data?.board) {
      setVersion(query.data.board.version);
      setConditions(query.data.board.data.conditions || []);
      setEndings(query.data.board.data.endings || []);
      setNodes(
        (query.data.board.data.nodes || []).map((n) => ({
          id: n.id,
          position: { x: n.x, y: n.y },
          data: { ...n, label: n.label },
          type: "default",
          className: "plot-node " + n.type,
        })),
      );
      setEdges(
        (query.data.board.data.edges || []).map((e) => ({
          ...e,
          data: { kind: e.kind },
          animated: e.kind === "cause",
          type: "smoothstep",
        })),
      );
    }
  }, [query.data]);
  const node = nodes.find((n) => n.id === selected);
  const patch = (value: Partial<BoardNode>) =>
    setNodes((old) =>
      old.map((n) =>
        n.id === selected ? { ...n, data: { ...n.data, ...value } } : n,
      ),
    );
  const nodeChanges = useCallback(
    (changes: NodeChange[]) =>
      setNodes((old) => applyNodeChanges(changes, old)),
    [],
  );
  const edgeChanges = useCallback(
    (changes: EdgeChange[]) =>
      setEdges((old) => applyEdgeChanges(changes, old)),
    [],
  );
  const connect = useCallback(
    (connection: Connection) =>
      setEdges((old) =>
        addEdge(
          {
            ...connection,
            id: id(),
            data: { kind: "cause" },
            label: "导致",
            type: "smoothstep",
            animated: true,
          },
          old,
        ),
      ),
    [],
  );
  const save = async () => {
    const data: Board = {
      nodes: nodes.map((n) => ({
        ...n.data,
        id: n.id,
        x: n.position.x,
        y: n.position.y,
      })) as BoardNode[],
      edges: edges.map((e) => ({
        id: e.id,
        source: e.source,
        target: e.target,
        label: String(e.label || ""),
        kind: String(e.data?.kind || "cause"),
      })),
      conditions,
      endings,
    };
    const res = await api<{ version: number }>(
      `/projects/${detail.project.id}/studio/board`,
      "PUT",
      { expectedVersion: version, data },
    );
    setVersion(res.version);
  };
  const generate = async (
    kind: "node-scene" | "relations/explain" = "node-scene",
  ) => {
    await save();
    if (!targetChapter) throw new Error("先选择一个目标章节。");
    const instructions =
      "根据剧情画布所选节点生成一份场景候选。节点只代表规划，不能擅自改变已确认事实。\n" +
      JSON.stringify(nodes.filter((n) => n.id === selected).map((n) => n.data));
    const run = await api<Run>(
      `/projects/${detail.project.id}/studio/${kind}`,
      "POST",
      {
        intentId: id(),
        chapterId: targetChapter,
        nodeIds: [selected],
        inputRevisionId:
          detail.chapters.find((c) => c.id === targetChapter)
            ?.currentRevisionId || "",
        instructions,
      },
    );
    setResult(run);
    setSceneTitle(String(node?.data.label || "新的场景"));
    setSceneGoal("");
  };
  const shown = nodes.map((n) => ({
    ...n,
    hidden:
      timeFilter &&
      ((n.data.storyTime != null && Number(n.data.storyTime) > time) ||
        (n.data.validFrom != null && Number(n.data.validFrom) > time) ||
        (n.data.validTo != null && Number(n.data.validTo) < time)),
  }));
  return (
    <div className="canvas-layout">
      <div className="canvas-stage">
        <div className="canvas-toolbar">
          <StudioButton
            onClick={() => {
              const next = id();
              setNodes((old) => [
                ...old,
                {
                  id: next,
                  position: {
                    x: 80 + (old.length % 3) * 290,
                    y: 80 + Math.floor(old.length / 3) * 170,
                  },
                  type: "default",
                  className: "plot-node plot",
                  data: {
                    id: next,
                    type: "plot",
                    label: "新的情节",
                    content: "",
                    chapterId,
                    entityId: "",
                    storyTime: null,
                    validFrom: null,
                    validTo: null,
                    prerequisiteIds: [],
                    observedBy: [],
                  },
                },
              ]);
              setSelected(next);
            }}
          >
            <Plus size={14} /> 添加节点
          </StudioButton>
          <StudioButton
            primary
            disabled={working}
            onClick={() => void act(save, "剧情画布已保存")}
          >
            <Save size={14} /> 保存画布
          </StudioButton>
          <span>从节点边缘拖动，建立关系</span>
        </div>
        <ReactFlow
          nodes={shown}
          edges={edges}
          onNodesChange={nodeChanges}
          onEdgesChange={edgeChanges}
          onConnect={connect}
          onNodeClick={(_, n) => setSelected(n.id)}
          onEdgeClick={(_, edge) => setSelectedEdge(edge.id)}
          fitView
          maxZoom={1.3}
          onInit={setFlow}
        >
          <Background gap={22} color="#d9dfd0" />
          <Controls />
          <MiniMap nodeColor="#a9bc94" />
        </ReactFlow>
        <div className="canvas-time">
          <label>
            <input
              type="checkbox"
              checked={timeFilter}
              onChange={(e) => setTimeFilter(e.target.checked)}
            />{" "}
            故事时间投影
          </label>
          <input
            type="range"
            min="0"
            max="100"
            value={time}
            onChange={(e) => setTime(Number(e.target.value))}
          />
          <span>第 {time} 天</span>
        </div>
      </div>
      <div className="canvas-inspector">
        <StudioError error={query.error} />
        {node ? (
          <StudioCard
            title="节点细节"
            subtitle="位置会随拖拽保存，条件限制节点的可执行性。"
          >
            <div className="studio-form">
              <label>
                标题
                <input
                  aria-label="节点标题"
                  value={String(node.data.label || "")}
                  onChange={(e) => patch({ label: e.target.value })}
                />
              </label>
              <label>
                类型
                <select
                  value={String(node.data.type)}
                  onChange={(e) => patch({ type: e.target.value })}
                >
                  {[
                    ["plot", "情节"],
                    ["scene", "场景"],
                    ["character", "人物"],
                    ["place", "地点"],
                    ["faction", "势力"],
                    ["event", "事件"],
                  ].map(([v, l]) => (
                    <option value={v} key={v}>
                      {l}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                节点内容
                <textarea
                  rows={5}
                  value={String(node.data.content || "")}
                  onChange={(e) => patch({ content: e.target.value })}
                />
              </label>
              <label>
                前置情节
                <select
                  multiple
                  value={(node.data.prerequisiteIds as string[]) || []}
                  onChange={(e) =>
                    patch({
                      prerequisiteIds: Array.from(e.target.selectedOptions).map(
                        (o) => o.value,
                      ),
                    })
                  }
                >
                  {nodes
                    .filter((n) => n.id !== selected)
                    .map((n) => (
                      <option key={n.id} value={n.id}>
                        {String(n.data.label)}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                关联实体
                <select
                  value={String(node.data.entityId || "")}
                  onChange={(e) => patch({ entityId: e.target.value })}
                >
                  <option value="">不绑定实体</option>
                  {detail.settings.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </select>
              </label>
              <div className="form-grid">
                {[
                  ["validFrom", "生效日"],
                  ["validTo", "结束日"],
                ].map(([key, label]) => (
                  <label key={key}>
                    {label}
                    <input
                      type="number"
                      value={
                        node.data[key] == null ? "" : Number(node.data[key])
                      }
                      onChange={(e) =>
                        patch({
                          [key]: e.target.value ? Number(e.target.value) : null,
                        })
                      }
                    />
                  </label>
                ))}
              </div>
              <label>
                谁观察到了事件
                <select
                  multiple
                  value={(node.data.observedBy as string[]) || []}
                  onChange={(e) =>
                    patch({
                      observedBy: Array.from(e.target.selectedOptions).map(
                        (o) => o.value,
                      ),
                    })
                  }
                >
                  {detail.settings
                    .filter((s) => s.kind === "character")
                    .map((s) => (
                      <option key={s.id} value={s.id}>
                        {s.name}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                故事时间（天，未知留空）
                <input
                  type="number"
                  value={
                    node.data.storyTime == null
                      ? ""
                      : Number(node.data.storyTime)
                  }
                  onChange={(e) =>
                    patch({
                      storyTime: e.target.value ? Number(e.target.value) : null,
                    })
                  }
                />
              </label>
              <StudioButton
                disabled={working}
                onClick={() =>
                  void act(() => generate(), "场景规划候选任务已创建。")
                }
              >
                <Sparkles size={14} /> 由这个节点生成场景
              </StudioButton>
              <ChapterSelect
                chapters={detail.chapters}
                value={targetChapter}
                onChange={setTargetChapter}
              />
              <StudioButton
                disabled={working}
                onClick={() =>
                  void act(
                    () => generate("relations/explain"),
                    "因果与关系解释任务已创建。",
                  )
                }
              >
                <Link2 size={14} /> 解释节点因果关系
              </StudioButton>
              <button
                className="text-button danger"
                onClick={() => {
                  setNodes(nodes.filter((n) => n.id !== selected));
                  setEdges(
                    edges.filter(
                      (e) => e.source !== selected && e.target !== selected,
                    ),
                  );
                  setConditions(
                    conditions.filter((c) => c.nodeId !== selected),
                  );
                  setEndings(endings.filter((e) => e.nodeId !== selected));
                  setSelected("");
                }}
              >
                <Trash2 size={13} /> 删除节点
              </button>
              <code>{node.id}</code>
            </div>
          </StudioCard>
        ) : (
          <StudioCard
            title="把故事铺开"
            subtitle="点击节点编辑；拖动以组织人物、事件与因果关系。"
          >
            <p className="muted small">
              节点是作者的规划，不会自动成为作品事实。先保存，再以选定节点创建场景候选。
            </p>
          </StudioCard>
        )}
        {selectedEdge && edges.find((e) => e.id === selectedEdge) && (
          <StudioCard title="情节连线">
            <div className="studio-form">
              <label>
                关系类型
                <select
                  value={String(
                    edges.find((e) => e.id === selectedEdge)?.data?.kind ||
                      "cause",
                  )}
                  onChange={(event) =>
                    setEdges(
                      edges.map((e) =>
                        e.id === selectedEdge
                          ? { ...e, data: { kind: event.target.value } }
                          : e,
                      ),
                    )
                  }
                >
                  {[
                    ["cause", "因果"],
                    ["requires", "前置要求"],
                    ["relation", "关系"],
                    ["sequence", "先后顺序"],
                    ["branch", "分支走向"],
                  ].map(([v, l]) => (
                    <option key={v} value={v}>
                      {l}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                连线说明
                <input
                  value={String(
                    edges.find((e) => e.id === selectedEdge)?.label || "",
                  )}
                  onChange={(event) =>
                    setEdges(
                      edges.map((e) =>
                        e.id === selectedEdge
                          ? { ...e, label: event.target.value }
                          : e,
                      ),
                    )
                  }
                />
              </label>
              <button
                className="text-button danger"
                onClick={() =>
                  setEdges(edges.filter((e) => e.id !== selectedEdge))
                }
              >
                删除这条连线
              </button>
            </div>
          </StudioCard>
        )}
        <StudioCard title="前置条件与分支结局">
          <div className="studio-form">
            <StudioButton
              disabled={!selected}
              onClick={() =>
                setConditions([
                  ...conditions,
                  {
                    id: id(),
                    nodeId: selected,
                    requiresNodeIds:
                      (node?.data.prerequisiteIds as string[]) || [],
                    expression: "满足前置情节后可进入",
                  },
                ])
              }
            >
              <Plus size={12} /> 为选中节点添加条件
            </StudioButton>
            {conditions.map((c, i) => (
              <div className="studio-inset" key={c.id}>
                <label>
                  条件说明
                  <input
                    value={c.expression}
                    onChange={(e) =>
                      setConditions(
                        conditions.map((x, j) =>
                          i === j ? { ...x, expression: e.target.value } : x,
                        ),
                      )
                    }
                  />
                </label>
                <small>
                  {String(
                    nodes.find((n) => n.id === c.nodeId)?.data.label ||
                      c.nodeId,
                  )}
                </small>
                <label>
                  必须已完成的节点
                  <select
                    multiple
                    value={c.requiresNodeIds}
                    onChange={(e) =>
                      setConditions(
                        conditions.map((x, j) =>
                          j === i
                            ? {
                                ...x,
                                requiresNodeIds: Array.from(
                                  e.target.selectedOptions,
                                ).map((o) => o.value),
                              }
                            : x,
                        ),
                      )
                    }
                  >
                    {nodes
                      .filter((n) => n.id !== c.nodeId)
                      .map((n) => (
                        <option key={n.id} value={n.id}>
                          {String(n.data.label)}
                        </option>
                      ))}
                  </select>
                </label>
                <button
                  className="text-button danger"
                  onClick={() =>
                    setConditions(conditions.filter((x) => x.id !== c.id))
                  }
                >
                  删除条件
                </button>
              </div>
            ))}
            <StudioButton
              disabled={!selected}
              onClick={() =>
                setEndings([
                  ...endings,
                  {
                    id: id(),
                    nodeId: selected,
                    title: "新的结局",
                    conditionIds: conditions
                      .filter((c) => c.nodeId === selected)
                      .map((c) => c.id),
                  },
                ])
              }
            >
              <Plus size={12} /> 将选中节点设为结局
            </StudioButton>
            {endings.map((ending, i) => (
              <div className="studio-inset" key={ending.id}>
                <label>
                  结局名
                  <input
                    value={ending.title}
                    onChange={(e) =>
                      setEndings(
                        endings.map((x, j) =>
                          j === i ? { ...x, title: e.target.value } : x,
                        ),
                      )
                    }
                  />
                </label>
                <label>
                  需要满足的条件
                  <select
                    multiple
                    value={ending.conditionIds}
                    onChange={(e) =>
                      setEndings(
                        endings.map((x, j) =>
                          j === i
                            ? {
                                ...x,
                                conditionIds: Array.from(
                                  e.target.selectedOptions,
                                ).map((o) => o.value),
                              }
                            : x,
                        ),
                      )
                    }
                  >
                    {conditions.map((c) => (
                      <option value={c.id} key={c.id}>
                        {c.expression}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  className="text-button danger"
                  onClick={() =>
                    setEndings(endings.filter((x) => x.id !== ending.id))
                  }
                >
                  删除结局定义
                </button>
              </div>
            ))}
          </div>
        </StudioCard>
        <StudioCard title="因果条件检查">
          <div className="studio-form">
            <label>
              已发生 / 已完成节点
              <select
                multiple
                value={completed}
                onChange={(e) =>
                  setCompleted(
                    Array.from(e.target.selectedOptions).map((o) => o.value),
                  )
                }
              >
                {nodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {String(n.data.label)}
                  </option>
                ))}
              </select>
            </label>
            <StudioButton
              disabled={working}
              onClick={() =>
                void act(async () => {
                  await save();
                  setEvaluation(
                    await api(
                      `/projects/${detail.project.id}/studio/board/evaluate`,
                      "POST",
                      { completedNodeIds: completed },
                    ),
                  );
                })
              }
            >
              <Play size={13} /> 检查可达情节
            </StudioButton>
            <Result value={evaluation} />
          </div>
        </StudioCard>
        {activeRun && (
          <StudioCard
            title={
              activeRun.type === "analysis" ? "因果解释候选" : "场景规划候选"
            }
            subtitle={activeRun.status}
          >
            <Result value={activeRun.output.text || activeRun.error} />
            {activeRun.type === "planning" &&
              activeRun.status === "Succeeded" && (
                <div className="studio-form">
                  <label>
                    采用场景名
                    <input
                      value={sceneTitle}
                      onChange={(e) => setSceneTitle(e.target.value)}
                    />
                  </label>
                  <label>
                    编辑场景目标与规划内容
                    <textarea
                      rows={7}
                      value={sceneGoal || activeRun.output.text || ""}
                      onChange={(e) => setSceneGoal(e.target.value)}
                    />
                  </label>
                  <StudioButton
                    primary
                    disabled={working || activeRun.stale}
                    onClick={() =>
                      void act(async () => {
                        const current = await api<{
                          scenes: {
                            version: number;
                            data: { scenes: unknown[] };
                          };
                        }>(`/projects/${detail.project.id}/studio`);
                        await api(
                          `/projects/${detail.project.id}/studio/scenes/apply-candidate`,
                          "POST",
                          {
                            expectedVersion: current.scenes.version,
                            runId: activeRun.id,
                            scene: {
                              chapterId: activeRun.chapterId,
                              order: current.scenes.data.scenes.length + 1,
                              title: sceneTitle,
                              goal: sceneGoal || activeRun.output.text,
                              conflict: "",
                              causes: [],
                              observedBy: [],
                              forbiddenChanges: [],
                              handoff: {
                                endLocation: "",
                                presentCharacters: [],
                                newFacts: [],
                                inventoryChanges: [],
                                openActions: [],
                                requiredContinuity: [],
                                forbiddenChanges: [],
                              },
                            },
                          },
                        );
                      }, "场景候选已由作者确认加入场景序列。")
                    }
                  >
                    <Check size={13} /> 确认加入场景序列
                  </StudioButton>
                </div>
              )}
          </StudioCard>
        )}
      </div>
    </div>
  );
}
