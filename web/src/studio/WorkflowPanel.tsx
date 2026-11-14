import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Plus,
  Save,
  Play,
  Pause,
  Check,
  X,
  Shield,
  Workflow,
  Layers3,
} from "lucide-react";
import { api, id, statusLabel } from "../api";
import {
  TaskMemoryControl,
  TaskMemoryUsage,
  useTaskAuthorMemory,
} from "../TaskAuthorMemory";
import {
  StudioCard,
  StudioButton,
  StudioError,
  Result,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Step = {
  id: string;
  type: string;
  promptTemplateId: string;
  agentId: string;
  dependsOn: string[];
  condition: string;
  humanGate: boolean;
  maxCost: number | null;
  inputSelection: string;
};
type Flow = { id: string; name: string; version: number; steps: Step[] };
type Agent = {
  id: string;
  name: string;
  taskType: string;
  promptTemplateId: string;
  tools: string[];
  maxOutputTokens: number;
};
type Prompt = {
  id: string;
  taskType: string;
  name: string;
  version: number;
  prompt: string;
  responseSchema?: unknown;
  rules: string[];
};
type Production = {
  id: string;
  ownerId?: string;
  error?: string;
  status: string;
  steps: {
    id: string;
    status: string;
    chapterId: string;
    taskRunId?: string;
    type?: string;
  }[];
  [key: string]: unknown;
};
const taskTypes = [
  "planning",
  "draft",
  "review",
  "revision",
  "memory-extraction",
  "handoff",
  "reader",
  "developmental",
  "voice",
  "publisher",
  "translation",
];
export default function WorkflowPanel({
  detail,
  notify,
  refresh,
  mode,
}: PanelProps & { mode: "workflow" | "production" }) {
  const taskMemory = useTaskAuthorMemory(detail.project.id);
  const workflows = useQuery({
    queryKey: ["studio", detail.project.id, "workflows"],
    queryFn: () => api<Flow[]>("/workflows"),
  });
  const agents = useQuery({
    queryKey: ["studio", detail.project.id, "agents"],
    queryFn: () => api<Agent[]>("/agents"),
  });
  const prompts = useQuery({
    queryKey: ["studio", detail.project.id, "prompts"],
    queryFn: () => api<Prompt[]>("/prompt-templates"),
  });
  const productions = useQuery({
    queryKey: ["studio", detail.project.id, "productions"],
    queryFn: () =>
      api<Production[]>(`/projects/${detail.project.id}/productions`),
    enabled: mode === "production",
    refetchInterval: 3000,
  });
  const [flow, setFlow] = useState<Flow>({
      id: "",
      name: "我的章节创作流程",
      version: 1,
      steps: [],
    }),
    [flowID, setFlowID] = useState(""),
    [agent, setAgent] = useState<Agent>({
      id: "",
      name: "",
      taskType: "review",
      promptTemplateId: "",
      tools: ["read-settings", "read-history"],
      maxOutputTokens: 3000,
    }),
    [prompt, setPrompt] = useState<Prompt>({
      id: "",
      name: "",
      taskType: "review",
      version: 1,
      prompt: "",
      rules: [],
    }),
    [editorTab, setEditorTab] = useState("flow"),
    [start, setStart] = useState(1),
    [end, setEnd] = useState(10),
    [maxCost, setMaxCost] = useState(""),
    [currency, setCurrency] = useState(""),
    [selected, setSelected] = useState(""),
    [schema, setSchema] = useState("");
  const { working, act } = useStudioAction(notify, refresh);
  const current =
    productions.data?.find((p) => p.id === selected) || productions.data?.[0];
  const patchStep = (index: number, value: Partial<Step>) =>
    setFlow({
      ...flow,
      steps: flow.steps.map((s, i) => (i === index ? { ...s, ...value } : s)),
    });
  return mode === "workflow" ? (
    <div>
      <div className="studio-subtabs">
        {[
          ["flow", "流程设计"],
          ["agents", "Agent 职责"],
          ["prompts", "Prompt 版本"],
        ].map(([v, l]) => (
          <button
            key={v}
            className={editorTab === v ? "active" : ""}
            onClick={() => setEditorTab(v)}
          >
            {l}
          </button>
        ))}
      </div>
      {editorTab === "flow" ? (
        <div className="studio-two-col">
          <StudioCard
            title="可配置创作流程"
            subtitle="每一步都有明确产物、依赖、条件和人工审批节点。"
          >
            <div className="studio-form">
              <label>
                已有流程
                <select
                  value={flowID}
                  onChange={(e) => {
                    setFlowID(e.target.value);
                    const f = workflows.data?.find(
                      (f) => f.id === e.target.value,
                    );
                    if (f) setFlow(f);
                  }}
                >
                  <option value="">新建流程</option>
                  {workflows.data?.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.name} · v{f.version}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                流程名
                <input
                  aria-label="流程名"
                  value={flow.name}
                  onChange={(e) => setFlow({ ...flow, name: e.target.value })}
                />
              </label>
              <div className="workflow-path">
                {flow.steps.map((step, i) => (
                  <div key={step.id} className="workflow-step">
                    <span>{i + 1}</span>
                    <div>
                      <label>
                        步骤类型
                        <select
                          value={step.type}
                          onChange={(e) =>
                            patchStep(i, { type: e.target.value })
                          }
                        >
                          {taskTypes.map((t) => (
                            <option key={t}>{t}</option>
                          ))}
                        </select>
                      </label>
                      <label>
                        执行角色
                        <select
                          value={step.agentId}
                          onChange={(e) =>
                            patchStep(i, { agentId: e.target.value })
                          }
                        >
                          <option value="">系统默认</option>
                          {agents.data?.map((a) => (
                            <option key={a.id} value={a.id}>
                              {a.name}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label>
                        Prompt 模板
                        <select
                          value={step.promptTemplateId}
                          onChange={(e) =>
                            patchStep(i, { promptTemplateId: e.target.value })
                          }
                        >
                          <option value="">系统默认</option>
                          {prompts.data?.map((p) => (
                            <option key={p.id} value={p.id}>
                              {p.name} v{p.version}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label>
                        依赖步骤
                        <select
                          multiple
                          value={step.dependsOn}
                          onChange={(e) =>
                            patchStep(i, {
                              dependsOn: Array.from(
                                e.target.selectedOptions,
                              ).map((o) => o.value),
                            })
                          }
                        >
                          {flow.steps.slice(0, i).map((s) => (
                            <option value={s.id} key={s.id}>
                              {s.type} · {s.id.slice(0, 5)}
                            </option>
                          ))}
                        </select>
                      </label>
                      <label>
                        执行条件
                        <select
                          value={step.condition}
                          onChange={(e) =>
                            patchStep(i, { condition: e.target.value })
                          }
                        >
                          <option value="always">始终执行</option>
                          <option value="issues-found">发现审稿问题时</option>
                          <option value="no-issues">没有审稿问题时</option>
                        </select>
                      </label>
                      <label>
                        输入来源
                        <select
                          value={step.inputSelection}
                          onChange={(e) =>
                            patchStep(i, { inputSelection: e.target.value })
                          }
                        >
                          <option value="current">当前正文</option>
                          <option value="previous-candidate">上一候选</option>
                        </select>
                      </label>
                      <label className="check-label">
                        <input
                          type="checkbox"
                          checked={step.humanGate}
                          onChange={(e) =>
                            patchStep(i, { humanGate: e.target.checked })
                          }
                        />{" "}
                        执行前需要作者批准
                      </label>
                      <button
                        className="text-button danger"
                        onClick={() =>
                          setFlow({
                            ...flow,
                            steps: flow.steps.filter((s) => s.id !== step.id),
                          })
                        }
                      >
                        删除此步
                      </button>
                    </div>
                  </div>
                ))}
              </div>
              <StudioButton
                onClick={() =>
                  setFlow({
                    ...flow,
                    steps: [
                      ...flow.steps,
                      {
                        id: id(),
                        type: "review",
                        promptTemplateId: "",
                        agentId: "",
                        dependsOn: flow.steps.length
                          ? [flow.steps[flow.steps.length - 1].id]
                          : [],
                        condition: "always",
                        humanGate: true,
                        maxCost: null,
                        inputSelection: "current",
                      },
                    ],
                  })
                }
              >
                <Plus size={13} /> 添加步骤
              </StudioButton>
              <StudioButton
                primary
                disabled={working || !flow.steps.length}
                onClick={() =>
                  void act(async () => {
                    const r = await api<Flow>("/workflows", "POST", {
                      ...flow,
                      id: "",
                      version: flow.id ? flow.version + 1 : flow.version,
                    });
                    setFlow(r);
                    setFlowID(r.id);
                  }, "流程定义与新版本已保存。")
                }
              >
                <Save size={14} /> 保存流程
              </StudioButton>
              <StudioError error={workflows.error} />
            </div>
          </StudioCard>
          <StudioCard title="明确权限边界">
            <Shield size={30} strokeWidth={1.3} />
            <p className="muted">
              Agent
              只能读取授予的材料、输出定义的产物；没有覆盖正文或直接确认事实的工具权限。
            </p>
            <p className="muted small">
              依赖和条件由服务端验证，非法循环或悬空引用不会执行。已执行任务继续关联当时的
              Prompt 和 Agent 快照。
            </p>
          </StudioCard>
        </div>
      ) : editorTab === "agents" ? (
        <div className="studio-two-col">
          <StudioCard title="创作团队">
            <div className="studio-list">
              {agents.data?.map((a) => (
                <button key={a.id} onClick={() => setAgent(a)}>
                  <Shield size={15} />
                  <span>
                    <strong>{a.name}</strong>
                    <small>
                      {a.taskType} · {a.tools.join(" / ")}
                    </small>
                  </span>
                </button>
              ))}
            </div>
          </StudioCard>
          <StudioCard title="角色职责与工具">
            <form
              className="studio-form"
              onSubmit={(e) => {
                e.preventDefault();
                void act(
                  () => api("/agents", "POST", agent),
                  "Agent 职责已保存",
                );
              }}
            >
              <label>
                角色名
                <input
                  value={agent.name}
                  onChange={(e) => setAgent({ ...agent, name: e.target.value })}
                  required
                />
              </label>
              <label>
                任务职责
                <select
                  value={agent.taskType}
                  onChange={(e) =>
                    setAgent({ ...agent, taskType: e.target.value })
                  }
                >
                  {taskTypes.map((t) => (
                    <option key={t}>{t}</option>
                  ))}
                </select>
              </label>
              <label>
                Prompt 模板
                <select
                  value={agent.promptTemplateId}
                  onChange={(e) =>
                    setAgent({ ...agent, promptTemplateId: e.target.value })
                  }
                >
                  <option value="">默认</option>
                  {prompts.data?.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
              <span className="section-label">允许读取的工具</span>
              {["read-settings", "read-history", "query-memory"].map((t) => (
                <label className="check-label" key={t}>
                  <input
                    type="checkbox"
                    checked={agent.tools.includes(t)}
                    onChange={(e) =>
                      setAgent({
                        ...agent,
                        tools: e.target.checked
                          ? [...agent.tools, t]
                          : agent.tools.filter((v) => v !== t),
                      })
                    }
                  />
                  {t}
                </label>
              ))}
              <label>
                单次输出上限
                <input
                  type="number"
                  min="128"
                  max="32768"
                  value={agent.maxOutputTokens}
                  onChange={(e) =>
                    setAgent({
                      ...agent,
                      maxOutputTokens: Number(e.target.value),
                    })
                  }
                />
              </label>
              <label>
                MCP 工具权限（每行一个）
                <textarea
                  rows={3}
                  value={agent.tools
                    .filter((t) => t.startsWith("mcp:"))
                    .join("\n")}
                  onChange={(e) =>
                    setAgent({
                      ...agent,
                      tools: [
                        ...agent.tools.filter((t) => !t.startsWith("mcp:")),
                        ...e.target.value
                          .split("\n")
                          .map((t) => t.trim())
                          .filter(Boolean),
                      ],
                    })
                  }
                  placeholder="mcp:服务配置ID:工具名"
                />
              </label>
              <StudioButton primary type="submit" disabled={working}>
                保存角色
              </StudioButton>
            </form>
          </StudioCard>
        </div>
      ) : (
        <div className="studio-two-col">
          <StudioCard title="Prompt 版本库">
            <div className="studio-list">
              {prompts.data?.map((p) => (
                <button
                  key={p.id}
                  onClick={() => {
                    setPrompt(p);
                    setSchema(
                      p.responseSchema
                        ? JSON.stringify(p.responseSchema, null, 2)
                        : "",
                    );
                  }}
                >
                  <FileTextIcon />
                  <span>
                    <strong>{p.name}</strong>
                    <small>
                      {p.taskType} · v{p.version}
                    </small>
                  </span>
                </button>
              ))}
            </div>
          </StudioCard>
          <StudioCard title="可审阅的提示模板">
            <form
              className="studio-form"
              onSubmit={(e) => {
                e.preventDefault();
                void act(
                  () =>
                    api("/prompt-templates", "POST", {
                      ...prompt,
                      responseSchema: schema.trim()
                        ? JSON.parse(schema)
                        : undefined,
                      id: "",
                      version: prompt.id ? prompt.version + 1 : prompt.version,
                    }),
                  "Prompt 新版本已保存",
                );
              }}
            >
              <label>
                模板名
                <input
                  value={prompt.name}
                  onChange={(e) =>
                    setPrompt({ ...prompt, name: e.target.value })
                  }
                  required
                />
              </label>
              <label>
                任务类型
                <select
                  value={prompt.taskType}
                  onChange={(e) =>
                    setPrompt({ ...prompt, taskType: e.target.value })
                  }
                >
                  {taskTypes.map((t) => (
                    <option key={t}>{t}</option>
                  ))}
                </select>
              </label>
              <label>
                系统提示
                <textarea
                  rows={15}
                  value={prompt.prompt}
                  onChange={(e) =>
                    setPrompt({ ...prompt, prompt: e.target.value })
                  }
                  required
                />
              </label>
              <label>
                硬规则（每行一条）
                <textarea
                  rows={4}
                  value={prompt.rules.join("\n")}
                  onChange={(e) =>
                    setPrompt({
                      ...prompt,
                      rules: e.target.value.split("\n").filter(Boolean),
                    })
                  }
                />
              </label>
              <label>
                结构化返回 Schema（可选 JSON）
                <textarea
                  rows={5}
                  value={schema}
                  onChange={(e) => setSchema(e.target.value)}
                  placeholder='{"type":"object","properties":{}}'
                />
              </label>
              <StudioButton primary type="submit" disabled={working}>
                <Save size={14} /> 保存新版本
              </StudioButton>
            </form>
          </StudioCard>
        </div>
      )}
    </div>
  ) : (
    <div className="studio-two-col">
      <StudioCard
        title="有审批的章节制作"
        subtitle="按保存的流程逐章推进，最多 50 章，明确预算和停点。"
      >
        <form
          className="studio-form"
          onSubmit={(e) => {
            e.preventDefault();
            void act(async () => {
              const p = await api<Production>(
                `/projects/${detail.project.id}/productions`,
                "POST",
                {
                  workflowId: flowID,
                  ignoreMemory: taskMemory.ignoreMemory,
                  chapterIds: detail.chapters
                    .filter((c) => c.order >= start && c.order <= end)
                    .map((c) => c.id),
                  startOrder: start,
                  endOrder: end,
                  maxChapters: Math.min(50, end - start + 1),
                  maxCost: maxCost ? Number(maxCost) : null,
                  currency,
                  manualGates: true,
                },
              );
              setSelected(p.id);
            }, "批量制作已创建，审批步骤需要你确认。");
          }}
        >
          <label>
            制作流程
            <select
              aria-label="制作流程"
              value={flowID}
              onChange={(e) => setFlowID(e.target.value)}
              required
            >
              <option value="">选择已保存流程</option>
              {workflows.data?.map((f) => (
                <option value={f.id} key={f.id}>
                  {f.name}
                </option>
              ))}
            </select>
          </label>
          <div className="form-grid">
            <label>
              起始章
              <input
                type="number"
                min="1"
                value={start}
                onChange={(e) => setStart(Number(e.target.value))}
              />
            </label>
            <label>
              结束章
              <input
                type="number"
                min={start}
                max={start + 49}
                value={end}
                onChange={(e) => setEnd(Number(e.target.value))}
              />
            </label>
            <label>
              总预算
              <input
                type="number"
                min="0"
                step=".01"
                value={maxCost}
                onChange={(e) => setMaxCost(e.target.value)}
                placeholder="留空为未设金额"
              />
            </label>
            <label>
              币种
              <input
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
              />
            </label>
          </div>
          <TaskMemoryControl
            preference={taskMemory}
            projectId={detail.project.id}
            purpose="批量制作"
            disabled={working}
            note="按创建本次制作的作者保存偏好和范围；一致性审稿、事实提取及交接检查不加载作者偏好。记忆变化后先复核，再继续后续步骤。"
          />
          <p className="muted small">
            制作不会越过人工审批自动确认正文或记忆。未知价格保持未知，不因缺少账单数据假设免费。
          </p>
          <StudioButton primary type="submit" disabled={working || !flowID}>
            <Play size={14} /> 创建批量制作
          </StudioButton>
        </form>
        <div className="studio-list">
          {productions.data?.map((p) => (
            <button key={p.id} onClick={() => setSelected(p.id)}>
              <Layers3 size={15} />
              <span>
                <strong>{p.id.slice(0, 8)}</strong>
                <small>{statusLabel[p.status] || p.status}</small>
              </span>
            </button>
          ))}
        </div>
        <StudioError error={productions.error} />
      </StudioCard>
      {current && (
        <StudioCard
          title="制作进度"
          subtitle={statusLabel[current.status] || current.status}
        >
          {current.error && <p className="warning-box">{current.error}</p>}
          <div className="studio-actions">
            {["pause", "resume", "cancel"].map((action) => (
              <StudioButton
                key={action}
                disabled={working}
                onClick={() =>
                  void act(
                    () =>
                      api(`/productions/${current.id}/${action}`, "POST", {}),
                    "制作状态已更新",
                  )
                }
              >
                {action === "pause" ? (
                  <Pause size={13} />
                ) : action === "resume" ? (
                  <Play size={13} />
                ) : (
                  <X size={13} />
                )}
                {{ pause: "暂停", resume: "恢复", cancel: "取消" }[action]}
              </StudioButton>
            ))}
          </div>
          {current.steps?.map((s) => (
            <div className="production-step" key={s.id}>
              <span className="pill">{statusLabel[s.status] || s.status}</span>
              <strong>
                {detail.chapters.find((c) => c.id === s.chapterId)?.title ||
                  s.chapterId}{" "}
                · {s.type}
              </strong>
              {s.status === "WaitingForUser" && (
                <StudioButton
                  disabled={working}
                  onClick={() =>
                    void act(
                      () =>
                        api(`/productions/${current.id}/approve`, "POST", {
                          stepId: s.id,
                        }),
                      "该步骤已获得作者批准。",
                    )
                  }
                >
                  <Check size={13} /> 批准此步
                </StudioButton>
              )}
              {s.taskRunId && (
                <>
                  <code>任务 {s.taskRunId.slice(0, 8)}</code>
                  <TaskMemoryUsage
                    compact
                    run={{
                      id: s.taskRunId,
                      type: s.type || "",
                      input: { ownerId: current.ownerId },
                      stale: false,
                    }}
                  />
                </>
              )}
            </div>
          ))}
        </StudioCard>
      )}
    </div>
  );
}
function FileTextIcon() {
  return <Workflow size={16} />;
}
