import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Sparkles,
  AlignLeft,
  Users,
  Brain,
  Search,
  Activity,
  ArrowUpRight,
  Play,
  Check,
  CheckCheck,
  X,
  RefreshCw,
  ChevronRight,
  ChevronDown,
  LoaderCircle,
  AlertTriangle,
  Quote,
  FileText,
  History,
  Pause,
  Square,
  Copy,
  Plus,
  Trash2,
  Eye,
  ExternalLink,
  Link2,
  Coins,
  Info,
  ArrowRight,
  CheckCircle2,
} from "lucide-react";
import {
  api,
  id,
  message,
  dateTime,
  documentFromText,
  busy,
  statusLabel,
  type Chapter,
  type Session,
  type Detail,
  type Run,
  type Issue,
  type Setting,
  type StateView,
  type Proposal,
  type Citation,
  type MemoryChange,
} from "./api";
import {
  paragraphDiff,
  applyHunks,
  type Hunk,
  canApplyCandidate,
} from "./revisions";
import Copilot, { type CandidateTarget } from "./Copilot";
import type { WritingSkillList } from "./WritingSkills";
import {
  TaskMemoryControl,
  TaskMemoryExclusion,
  TaskMemoryUsage,
  useTaskAuthorMemory,
} from "./TaskAuthorMemory";
import { authorMemoryTaskExclusion } from "./taskMemoryPolicy";
import "./workspaceAssistant.css";

type Props = {
  hidden?: boolean;
  composeRequest?: { id: number; text: string };
  chapter: Chapter;
  detail: Detail;
  currentRevisionId: string;
  currentText: string;
  selection: string;
  config?: { model: string; externalModel: boolean };
  flush: () => Promise<string>;
  onApply: (text: string, runId: string) => Promise<void>;
  onSaved: () => void;
  notify: (text: string) => void;
  onClose: () => void;
};
type Tab =
  "assistant" | "blueprint" | "settings" | "memory" | "search" | "runs";
const tabs: { id: Tab; title: string; label: string; Icon: typeof Sparkles }[] =
  [
    { id: "assistant", title: "灵感助手", label: "对话", Icon: Sparkles },
    { id: "blueprint", title: "本章章纲", label: "章纲", Icon: AlignLeft },
    { id: "settings", title: "人物设定", label: "设定", Icon: Users },
    { id: "memory", title: "故事记忆", label: "记忆", Icon: Brain },
    { id: "search", title: "历史检索", label: "检索", Icon: Search },
    { id: "runs", title: "任务记录", label: "任务", Icon: Activity },
  ];
const taskName: Record<string, string> = {
  draft: "章节起草",
  review: "一致性审稿",
  revision: "定向修订",
  "memory-extraction": "记忆提取",
  planning: "章纲规划",
  rewrite: "选段改写",
  handoff: "场景交接",
};

export default function Assistant(props: Props) {
  const { chapter, detail, notify } = props;
  const [tab, setTab] = useState<Tab>("assistant");
  const [toolsOpen, setToolsOpen] = useState(false);
  const [candidateRequest, setCandidateRequest] = useState<{
    id: number;
    text: string;
    target: CandidateTarget;
  }>();
  const continueCandidate = (candidate: Run) => {
    setCandidateRequest({
      id: Date.now(),
      text: "请继续打磨这份候选，保留已满意的修改，接下来按我的要求调整。",
      target: {
        runId: candidate.id,
        title: taskName[candidate.type] || "写作候选",
      },
    });
    setTab("assistant");
    setToolsOpen(false);
  };
  const [contextOpen, setContextOpen] = useState(false);
  const [copilotActive, setCopilotActive] = useState(false);
  useEffect(() => {
    if (!props.composeRequest) return;
    setTab("assistant");
    setToolsOpen(false);
  }, [props.composeRequest?.id]);
  useEffect(() => {
    if (copilotActive) setToolsOpen(false);
  }, [copilotActive]);
  const memoryContext = useQuery({
    queryKey: ["state", detail.project.id],
    queryFn: () => api<StateView>(`/projects/${detail.project.id}/state`),
    enabled: contextOpen,
    refetchInterval: contextOpen ? 4000 : false,
  });
  const writingSkills = useQuery({
    queryKey: ["writing-skills", detail.project.id],
    queryFn: () =>
      api<WritingSkillList>(`/projects/${detail.project.id}/writing-skills`),
    staleTime: 5000,
  });
  const enabledSkills = writingSkills.data?.items.filter(
    (skill) =>
      skill.enabled &&
      (!skill.taskTypes.length || skill.taskTypes.includes("copilot")),
  );
  const taskMemory = useTaskAuthorMemory(detail.project.id);
  const [instructions, setInstructions] = useState("");
  const [starting, setStarting] = useState(false);
  const [operation, setOperation] = useState("expand");
  const [selectedRun, setSelectedRun] = useState("");
  const [selectedIssues, setSelectedIssues] = useState<Issue[]>([]);
  const [localIssues, setLocalIssues] = useState<Issue[] | null>(null);
  const [useCache, setUseCache] = useState(false),
    [temperature, setTemperature] = useState(""),
    [agentID, setAgentID] = useState(""),
    [promptID, setPromptID] = useState(""),
    [summaryID, setSummaryID] = useState("");
  const [topP, setTopP] = useState(""),
    [mcpInput, setMCPInput] = useState("");
  const agentOptions = useQuery({
    queryKey: ["agents"],
    queryFn: () =>
      api<{ id: string; name: string; taskType: string; tools: string[] }[]>(
        "/agents",
      ),
  });
  const selectedAgent = agentOptions.data?.find(
    (agent) => agent.id === agentID,
  );
  const agentMemoryReason =
    selectedAgent && !selectedAgent.tools?.includes("query-memory")
      ? "所选创作角色未获记忆读取权限，本次不会加载作者偏好。"
      : undefined;
  const promptOptions = useQuery({
    queryKey: ["prompts"],
    queryFn: () =>
      api<{ id: string; name: string; taskType: string; version: number }[]>(
        "/prompt-templates",
      ),
  });
  const qc = useQueryClient();
  const runs = useQuery({
    queryKey: ["runs", detail.project.id],
    queryFn: () => api<Run[]>(`/projects/${detail.project.id}/runs`),
    refetchInterval: 2000,
  });
  const ownRuns = runs.data?.filter((r) => r.chapterId === chapter.id) || [];
  const run = ownRuns.find((r) => r.id === selectedRun) || ownRuns[0];
  useEffect(() => {
    if (!run || !busy(run)) return;
    const source = new EventSource(`/api/runs/${run.id}/events`, {
      withCredentials: true,
    });
    const invalidate = () => {
      void qc.invalidateQueries({ queryKey: ["runs", detail.project.id] });
    };
    source.onmessage = invalidate;
    source.addEventListener("completed", invalidate);
    source.addEventListener("status", invalidate);
    source.addEventListener("output", invalidate);
    source.addEventListener("delta", invalidate);
    source.onerror = () => {
      source.close();
    };
    return () => source.close();
  }, [run?.id, run?.status, qc, detail.project.id]);
  const start = async (type: string) => {
    setToolsOpen(true);
    setStarting(true);
    setTab(
      type === "memory-extraction"
        ? "memory"
        : type === "planning"
          ? "blueprint"
          : "assistant",
    );
    try {
      const revision = await props.flush();
      if (!["draft", "planning"].includes(type) && !revision)
        throw new Error("请先保存本章正文。");
      const r = await api<Run>(`/chapters/${chapter.id}/runs`, "POST", {
        intentId: id(),
        taskType: type,
        ignoreMemory:
          taskMemory.ignoreMemory || !!authorMemoryTaskExclusion(type),
        inputRevisionId: revision,
        instructions,
        selection: props.selection,
        operation,
        useCache,
        temperature: temperature ? Number(temperature) : undefined,
        agentId: agentID,
        promptTemplateId: promptID,
        contextSummaryId: summaryID,
        topP: topP ? Number(topP) : undefined,
        mcpCalls: mcpInput.trim() ? JSON.parse(mcpInput) : undefined,
        selectedIssues: type === "revision" ? selectedIssues : undefined,
      });
      setSelectedRun(r.id);
      await qc.invalidateQueries({ queryKey: ["runs", detail.project.id] });
      notify("任务已保存；离开页面后仍可在任务记录中查看。");
    } catch (e) {
      notify(message(e));
    } finally {
      setStarting(false);
    }
  };
  return (
    <aside
      className="assistant-pane"
      hidden={props.hidden}
      aria-label="章节创作伙伴"
    >
      <div className="assistant-heading">
        <div>
          <span className="assistant-symbol">
            <Sparkles size={15} />
          </span>
          <h2>创作伙伴</h2>
        </div>
        <span className="assistant-online">
          <span className="tiny-dot" />{" "}
          {copilotActive ? "正在推敲" : "本章创作"}
        </span>
        <button
          className="icon-button assistant-mobile-close"
          aria-label="收起创作伙伴"
          onClick={props.onClose}
        >
          <X size={17} />
        </button>
      </div>
      <nav className="assistant-tabs" aria-label="创作伙伴工具">
        {tabs.map(({ id, title, label, Icon }) => (
          <button
            key={id}
            className={tab === id ? "active" : ""}
            onClick={() => {
              setTab(id);
              if (id === "assistant") setToolsOpen(false);
            }}
            title={title}
            aria-label={title}
            aria-pressed={tab === id}
          >
            <Icon size={14} />
            <span>{label}</span>
          </button>
        ))}
      </nav>
      <details
        className="assistant-source-summary"
        open={contextOpen}
        onToggle={(event) => setContextOpen(event.currentTarget.open)}
      >
        <summary>
          <Brain size={13} />
          <span>
            本章资料 <small title={chapter.title}>{chapter.title}</small>
          </span>
          <ChevronDown size={13} />
        </summary>
        <div className="assistant-source-grid">
          <button onClick={() => setTab("blueprint")} aria-label="查看本章章纲">
            <AlignLeft size={15} />
            <span>
              本章章纲
              <strong>{chapter.blueprint.trim() ? "已准备" : "待补充"}</strong>
            </span>
            <ChevronRight size={12} />
          </button>
          <button
            onClick={() => setTab("settings")}
            aria-label="查看本书人物与设定"
          >
            <Users size={15} />
            <span>
              人物与设定<strong>{detail.settings.length} 项</strong>
            </span>
            <ChevronRight size={12} />
          </button>
          <button
            onClick={() => setTab("memory")}
            aria-label="查看已确认故事记忆"
          >
            <Brain size={15} />
            <span>
              全书确认记忆
              <strong>
                {memoryContext.data
                  ? `${memoryContext.data.facts?.length || 0} 条`
                  : memoryContext.isError
                    ? "读取失败"
                    : "读取中…"}
              </strong>
            </span>
            <ChevronRight size={12} />
          </button>
          <div>
            <Sparkles size={15} />
            <span>
              对话写作指南
              <strong>
                {enabledSkills
                  ? `${enabledSkills.length} 项启用`
                  : writingSkills.isError
                    ? "读取失败"
                    : "读取中…"}
              </strong>
            </span>
          </div>
        </div>
        <p>这是可查阅资料的概览。对话按需读取，记忆仍受章节与人物视角限制。</p>
      </details>
      <div
        className={
          "assistant-content " +
          (tab === "assistant"
            ? `assistant-chat-content${toolsOpen ? " tools-mode" : ""}`
            : "")
        }
      >
        <div className="assistant-chat-panel" hidden={tab !== "assistant"}>
          <div className="assistant-conversation" hidden={toolsOpen}>
            <Copilot
              key={detail.project.id}
              {...props}
              onBusyChange={setCopilotActive}
              candidateRequest={candidateRequest}
            />
          </div>
          <details
            className="copilot-writing-tools"
            open={toolsOpen}
            onToggle={(event) => setToolsOpen(event.currentTarget.open)}
          >
            <summary
              onClick={(event) => {
                if (copilotActive && !toolsOpen) {
                  event.preventDefault();
                  notify(
                    "对话仍在运行，可先停止回答或等它完成，再打开单次写作工具。",
                  );
                }
              }}
            >
              <Sparkles size={14} /> 写作工具{" "}
              <span>起草 · 审稿 · 选段修订</span>
              {toolsOpen && <strong>返回对话</strong>}
            </summary>
            <div className="copilot-writing-tools-body">
              <div className="assistant-intro">
                <span className="eyebrow">WRITE WITH INTENTION</span>
                <h3>下一句，往哪里走？</h3>
                <p>以你的章纲为方向，让人物、情节与设定保持连贯。</p>
              </div>
              <div className="context-strip">
                <Brain size={14} />
                <span>
                  {statusLabel[detail.project.health] || detail.project.health}
                </span>
                <span className="context-count">
                  {detail.settings.length} 项设定
                </span>
              </div>
              <label className="instruction-field">
                这次想怎样推进？
                <textarea
                  value={instructions}
                  onChange={(e) => setInstructions(e.target.value)}
                  rows={3}
                  placeholder="例如：收紧节奏，用动作表现不安，停在发现线索时…"
                />
              </label>
              <TaskMemoryControl
                preference={taskMemory}
                projectId={detail.project.id}
                purpose="单次写作"
                excludedReason={agentMemoryReason}
                disabled={starting}
                note="起草、定向修订和选段改写参考作者偏好；一致性审稿和交接检查不加载偏好。"
              />
              <div className="ai-actions">
                <button
                  className="ai-primary-action"
                  disabled={starting}
                  onClick={() => void start("draft")}
                >
                  <span className="action-icon">
                    <Sparkles size={19} />
                  </span>
                  <span>
                    <strong>依章纲起草</strong>
                    <small>生成一份独立候选稿</small>
                  </span>
                  {starting ? (
                    <LoaderCircle size={17} className="spin" />
                  ) : (
                    <ArrowUpRight size={18} />
                  )}
                </button>
                <div className="ai-secondary">
                  <button
                    disabled={starting}
                    onClick={() => void start("review")}
                  >
                    <Eye size={17} />
                    <span>一致性审稿</span>
                  </button>
                  <button
                    disabled={starting || !selectedIssues.length}
                    onClick={() => void start("revision")}
                    title={
                      !selectedIssues.length
                        ? "先选择审稿问题，再进行定向修订"
                        : ""
                    }
                  >
                    <PencilIcon />
                    <span>
                      定向修订
                      {selectedIssues.length
                        ? ` · ${selectedIssues.length}`
                        : ""}
                    </span>
                  </button>
                </div>
              </div>
              <button
                className="text-button local-lint"
                onClick={() =>
                  void props
                    .flush()
                    .then((rid) =>
                      api<Issue[]>(
                        "/chapters/" + chapter.id + "/lint?revisionId=" + rid,
                      ),
                    )
                    .then(setLocalIssues)
                    .catch((e) => notify(message(e)))
                }
              >
                <CheckCircle2 size={13} /> 本地规则检查 · 不调用模型
              </button>
              {localIssues && (
                <div className="local-issues">
                  <span className="section-label">
                    规则检查 · {localIssues.length} 条提示
                  </span>
                  {localIssues.map((issue, i) => (
                    <div className="issue-card" key={i}>
                      <span className="pill amber">{issue.kind}</span>
                      {issue.quote && <blockquote>{issue.quote}</blockquote>}
                      <p>{issue.explanation}</p>
                      <small>{issue.suggestedAction}</small>
                    </div>
                  ))}
                  {!localIssues.length && (
                    <p className="muted small">
                      没有触发本地规则提示。仍请自行复核叙事与风格。
                    </p>
                  )}
                </div>
              )}
              <div className="rewrite-tools">
                <div className="row-between">
                  <span className="section-label">选段推敲</span>
                  <span className="muted small">
                    {props.selection
                      ? `已选 ${props.selection.length} 字`
                      : "在正文中选中一段"}
                  </span>
                </div>
                <div className="rewrite-row">
                  <select
                    aria-label="选段改写方式"
                    value={operation}
                    onChange={(e) => setOperation(e.target.value)}
                  >
                    <option value="expand">扩写细节</option>
                    <option value="compress">压缩冗余</option>
                    <option value="polish">润色表达</option>
                  </select>
                  <button
                    className="button"
                    disabled={starting || !props.selection}
                    onClick={() => void start("rewrite")}
                  >
                    生成候选
                  </button>
                </div>
                <button
                  className="text-button"
                  disabled={starting || !props.currentRevisionId}
                  onClick={() => void start("handoff")}
                >
                  <ArrowRight size={12} /> 生成场景交接摘要
                </button>
              </div>
              {ownRuns.length > 1 && (
                <select
                  className="run-selector"
                  aria-label="切换本章候选任务"
                  value={run?.id || ""}
                  onChange={(e) => setSelectedRun(e.target.value)}
                >
                  {ownRuns.map((r) => (
                    <option value={r.id} key={r.id}>
                      {taskName[r.type] || r.type} · {dateTime(r.createdAt)} ·{" "}
                      {statusLabel[r.status] || r.status}
                    </option>
                  ))}
                </select>
              )}
              <details className="generation-settings">
                <summary>本次生成策略与角色</summary>
                <label className="check-label">
                  <input
                    type="checkbox"
                    checked={useCache}
                    onChange={(e) => setUseCache(e.target.checked)}
                  />{" "}
                  允许复用相同版本、模型与输入的缓存
                </label>
                <label>
                  采样温度（留空用模型默认）
                  <input
                    type="number"
                    min="0"
                    max="2"
                    step=".1"
                    value={temperature}
                    onChange={(e) => setTemperature(e.target.value)}
                  />
                </label>
                <label>
                  Agent 职责
                  <select
                    aria-label="Agent 职责"
                    value={agentID}
                    onChange={(e) => setAgentID(e.target.value)}
                  >
                    <option value="">按任务默认</option>
                    {agentOptions.data?.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name} · {a.taskType}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  Prompt 版本
                  <select
                    value={promptID}
                    onChange={(e) => setPromptID(e.target.value)}
                  >
                    <option value="">按任务默认</option>
                    {promptOptions.data?.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name} · {p.taskType} v{p.version}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  压缩上下文 ID（可选）
                  <input
                    value={summaryID}
                    onChange={(e) => setSummaryID(e.target.value)}
                    placeholder="在工作室创建后粘贴 ID"
                  />
                </label>
                <label>
                  Top P（留空用模型默认）
                  <input
                    type="number"
                    min="0"
                    max="1"
                    step=".05"
                    value={topP}
                    onChange={(e) => setTopP(e.target.value)}
                  />
                </label>
                <label>
                  本次 MCP 只读调用（可选 JSON 数组）
                  <textarea
                    rows={3}
                    value={mcpInput}
                    onChange={(e) => setMCPInput(e.target.value)}
                    placeholder='[{"configId":"服务ID","name":"工具名","arguments":{}}]'
                  />
                </label>
                <p>
                  所选角色和模板必须匹配任务类型。MCP 调用须在服务白名单和 Agent
                  权限中同时授权。压缩内容仍校验作品、版本与未来可见性。
                </p>
              </details>
              <div className="model-disclosure">
                <Info size={12} />
                <p>
                  {props.config?.externalModel
                    ? "任务会向已配置的模型服务发送本章及相关上下文。"
                    : "当前模型为功能测试模式。"}{" "}
                  <span>{props.config?.model || "读取模型配置中"}</span>
                </p>
              </div>
              {run && (
                <RunCard
                  run={run}
                  currentRevisionId={props.currentRevisionId}
                  currentText={props.currentText}
                  onContinue={continueCandidate}
                  onApply={props.onApply}
                  notify={notify}
                  onSelectIssues={setSelectedIssues}
                  selectedIssues={selectedIssues}
                  onRefresh={() =>
                    void qc.invalidateQueries({
                      queryKey: ["runs", detail.project.id],
                    })
                  }
                />
              )}
            </div>
          </details>
        </div>
        {tab === "blueprint" && (
          <Blueprint
            chapter={chapter}
            onSaved={props.onSaved}
            notify={notify}
            onGenerate={() => void start("planning")}
            starting={starting}
            memoryControl={
              <TaskMemoryControl
                preference={taskMemory}
                projectId={detail.project.id}
                purpose="章纲规划"
                excludedReason={agentMemoryReason}
                taskType="planning"
                disabled={starting}
              />
            }
            plans={ownRuns.filter((r) => r.type === "planning")}
          />
        )}
        {tab === "settings" && (
          <SettingsPanel
            detail={detail}
            onSaved={props.onSaved}
            notify={notify}
          />
        )}
        {tab === "memory" && (
          <MemoryPanel
            {...props}
            onExtract={() => void start("memory-extraction")}
            starting={starting}
          />
        )}
        {tab === "search" && (
          <SearchPanel detail={detail} chapter={chapter} notify={notify} />
        )}
        {tab === "runs" && (
          <>
            <div className="panel-section-heading">
              <div>
                <span className="eyebrow">EVERY STEP, KEPT</span>
                <h3>任务与费用</h3>
              </div>
              <Activity size={19} />
            </div>
            <p className="muted small">
              任务独立于页面运行。未知结果需要核对，不会静默重发。
            </p>
            <BudgetPanel projectId={detail.project.id} notify={notify} />
            {runs.isError && (
              <div className="error-box">{message(runs.error)}</div>
            )}
            {!runs.data?.length && (
              <Empty Icon={Activity} text="还没有任务记录。" />
            )}
            {runs.data?.map((r) => (
              <RunCard
                key={r.id}
                run={r}
                compact={r.id !== selectedRun}
                onContinue={continueCandidate}
                currentRevisionId={props.currentRevisionId}
                currentText={props.currentText}
                onApply={props.onApply}
                notify={notify}
                onSelectIssues={setSelectedIssues}
                selectedIssues={selectedIssues}
                onRefresh={() =>
                  void qc.invalidateQueries({
                    queryKey: ["runs", detail.project.id],
                  })
                }
                onOpen={() => {
                  setSelectedRun(r.id);
                  if (r.chapterId === chapter.id) {
                    setToolsOpen(true);
                    setTab("assistant");
                  } else
                    notify(
                      "这是其他章节的任务，可查看记录；请先打开对应章节再采用。",
                    );
                }}
              />
            ))}
          </>
        )}
      </div>
      <div className="assistant-footnote">
        <LeafIcon />
        <span>每次采用，都由你决定。</span>
      </div>
    </aside>
  );
}
function PencilIcon() {
  return <FileText size={17} />;
}
function LeafIcon() {
  return <span className="leaf-mark">❧</span>;
}
function Empty({ Icon, text }: { Icon: typeof Brain; text: string }) {
  return (
    <div className="mini-empty">
      <Icon size={29} strokeWidth={1.2} />
      <p>{text}</p>
    </div>
  );
}

function Blueprint({
  chapter,
  onSaved,
  notify,
  onGenerate,
  starting,
  plans,
  memoryControl,
}: {
  chapter: Chapter;
  onSaved: () => void;
  notify: (s: string) => void;
  onGenerate: () => void;
  starting: boolean;
  plans: Run[];
  memoryControl: ReactNode;
}) {
  const [outline, setOutline] = useState(chapter.blueprint || ""),
    [saving, setSaving] = useState(false);
  return (
    <>
      <div className="panel-section-heading">
        <div>
          <span className="eyebrow">THE CHAPTER BLUEPRINT</span>
          <h3>先看清这一章</h3>
        </div>
        <AlignLeft size={20} />
      </div>
      <p className="muted small">
        写清本章目标、冲突、人物选择与结束位置，给生成一个明确的边界。
      </p>
      <label className="form-label">
        章纲
        <textarea
          className="blueprint-input"
          rows={13}
          value={outline}
          onChange={(e) => setOutline(e.target.value)}
          placeholder={
            "本章发生在哪里？\n谁想要什么，又遇到了什么阻碍？\n人物做出怎样的选择？\n故事停在哪一刻？"
          }
        />
      </label>
      {memoryControl}
      <div className="row">
        <button
          className="button primary full"
          disabled={saving}
          onClick={() => {
            setSaving(true);
            api(`/chapters/${chapter.id}`, "PUT", {
              title: chapter.title,
              volume: chapter.volume,
              blueprint: outline,
            })
              .then(() => {
                onSaved();
                notify("章纲已保存");
              })
              .catch((e) => notify(message(e)))
              .finally(() => setSaving(false));
          }}
        >
          {saving ? (
            <LoaderCircle size={15} className="spin" />
          ) : (
            <Check size={15} />
          )}{" "}
          保存章纲
        </button>
        <button
          className="button full"
          disabled={starting}
          onClick={onGenerate}
        >
          <Sparkles size={14} /> 规划建议
        </button>
      </div>
      {plans
        .filter((r) => r.output?.text)
        .map((plan) => (
          <article className="run-card" key={plan.id}>
            <span className="section-label">
              AI 章纲候选 · {dateTime(plan.createdAt)}
            </span>
            <pre className="plan-text">{plan.output.text}</pre>
            <TaskMemoryUsage run={plan} />
            {plan.stale && (
              <p className="warning-box">
                输入或作者记忆已变化，这份章纲保留为历史，请按当前要求重新生成。
              </p>
            )}
            <button
              className="button full"
              disabled={plan.stale || plan.status !== "Succeeded"}
              onClick={() => {
                setOutline(plan.output.text || "");
                notify("候选已填入章纲编辑区；点击保存后生效。");
              }}
            >
              填入章纲编辑区
            </button>
          </article>
        ))}
      <div className="quiet-tip">
        <span>一点提示</span>
        <p>章纲描述未来计划；它不会自动成为已经发生的故事事实。</p>
      </div>
    </>
  );
}
function SettingsPanel({
  detail,
  onSaved,
  notify,
}: {
  detail: Detail;
  onSaved: () => void;
  notify: (s: string) => void;
}) {
  const templates = useQuery({
    queryKey: ["style-templates"],
    queryFn: () =>
      api<
        {
          id: string;
          name: string;
          description: string;
          instructions: string;
        }[]
      >("/style-templates"),
  });
  const [editing, setEditing] = useState<Setting | null>(null),
    [kind, setKind] = useState("character"),
    [name, setName] = useState(""),
    [content, setContent] = useState(""),
    [saving, setSaving] = useState(false);
  const reset = () => {
    setEditing(null);
    setName("");
    setContent("");
  };
  const categories: { id: string; label: string }[] = [
    { id: "character", label: "人物" },
    { id: "world", label: "世界" },
    { id: "rule", label: "规则" },
    { id: "style", label: "文风" },
    { id: "outline", label: "总纲" },
  ];
  return (
    <>
      <div className="panel-section-heading">
        <div>
          <span className="eyebrow">THE STORY BIBLE</span>
          <h3>你的故事底稿</h3>
        </div>
        <Users size={20} />
      </div>
      <p className="muted small">
        作者确认的设定拥有独立来源，AI 不会静默改写它们。
      </p>
      <div className="segmented">
        {categories.map((c) => (
          <button
            className={kind === c.id ? "active" : ""}
            key={c.id}
            onClick={() => {
              setKind(c.id);
              reset();
            }}
          >
            {c.label}
          </button>
        ))}
      </div>
      {kind === "style" && (
        <label className="form-label">
          从文风模板开始
          <select
            aria-label="文风模板"
            defaultValue=""
            onChange={(e) => {
              const template = templates.data?.find(
                (t) => t.id === e.target.value,
              );
              if (template) {
                setName(template.name);
                setContent(template.instructions);
              }
            }}
          >
            <option value="">选择模板，可在下方自由修改</option>
            {templates.data?.map((t) => (
              <option value={t.id} key={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </label>
      )}
      <div className="setting-list">
        {detail.settings
          .filter((s) => s.kind === kind)
          .map((s) => (
            <article className="setting-card" key={s.id}>
              <div className="row-between">
                <h4>{s.name}</h4>
                <div className="row">
                  <button
                    className="text-button"
                    onClick={() => {
                      setEditing(s);
                      setName(s.name);
                      setContent(s.content);
                    }}
                  >
                    编辑
                  </button>
                  <button
                    className="icon-button danger"
                    aria-label={"删除设定" + s.name}
                    onClick={() => {
                      if (confirm(`删除设定“${s.name}”？`))
                        void api(
                          `/projects/${detail.project.id}/settings/${s.id}`,
                          "DELETE",
                        )
                          .then(onSaved)
                          .catch((e) => notify(message(e)));
                    }}
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              </div>
              <p>{s.content}</p>
            </article>
          ))}
      </div>
      <form
        className="form-stack setting-form"
        onSubmit={(e) => {
          e.preventDefault();
          setSaving(true);
          api(`/projects/${detail.project.id}/settings`, "POST", {
            id: editing?.id,
            kind,
            name,
            content,
          })
            .then(() => {
              reset();
              onSaved();
              notify("设定已保存");
            })
            .catch((e) => notify(message(e)))
            .finally(() => setSaving(false));
        }}
      >
        <div className="row-between">
          <span className="section-label">
            {editing
              ? "编辑设定"
              : "添加" +
                (categories.find((c) => c.id === kind)?.label || "设定")}
          </span>
          {editing && (
            <button type="button" className="text-button" onClick={reset}>
              取消编辑
            </button>
          )}
        </div>
        <input
          aria-label="设定名称"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={kind === "character" ? "人物姓名" : "设定名称"}
          required
        />
        <textarea
          aria-label="设定内容"
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={4}
          placeholder={
            kind === "character"
              ? "身份、欲望、关系与不能改变的特征…"
              : "只记录已确定的内容，未知之处可以保留。"
          }
          required
        />
        <button className="button full" disabled={saving}>
          <Plus size={14} /> {editing ? "保存修改" : "添加设定"}
        </button>
      </form>
    </>
  );
}

function RunCard({
  run,
  currentRevisionId,
  currentText,
  onApply,
  notify,
  onSelectIssues,
  selectedIssues,
  onRefresh,
  compact = false,
  onOpen,
  onContinue,
}: {
  run: Run;
  currentRevisionId: string;
  currentText: string;
  onApply: (text: string, runId: string) => Promise<void>;
  notify: (s: string) => void;
  onSelectIssues: (issues: Issue[]) => void;
  selectedIssues: Issue[];
  onRefresh: () => void;
  compact?: boolean;
  onOpen?: () => void;
  onContinue?: (run: Run) => void;
}) {
  const [showContext, setShowContext] = useState(false),
    [comparing, setComparing] = useState(false),
    [hunks, setHunks] = useState<Hunk[]>([]),
    [working, setWorking] = useState(false);
  const session = useQuery({
    queryKey: ["session"],
    queryFn: () => api<Session>("/session"),
  });
  const canContinueOwn =
    !run.input.ownerId || run.input.ownerId === session.data?.user?.id;
  const text = run.output?.text || "";
  const inputRevision = String(
    run.input?.revisionId || run.input?.inputRevisionId || "",
  );
  const selection = String(run.input?.selection || "");
  const uniqueSelection =
    run.type !== "rewrite" ||
    (selection !== "" && currentText.split(selection).length === 2);
  const canApply =
    run.status === "Succeeded" &&
    canApplyCandidate(inputRevision, currentRevisionId, run.stale) &&
    uniqueSelection;
  const operate = async (action: string) => {
    if (
      action === "regenerate" &&
      !confirm("重新生成会创建新任务，并可能产生额外模型费用。继续？")
    )
      return;
    setWorking(true);
    try {
      await api(`/runs/${run.id}/${action}`, "POST", {});
      onRefresh();
      notify(
        action === "cancel"
          ? "取消请求已记录"
          : action === "pause"
            ? "暂停请求已记录"
            : action === "regenerate"
              ? "已创建新生成任务"
              : "任务状态已更新",
      );
    } catch (e) {
      notify(message(e));
    } finally {
      setWorking(false);
    }
  };
  const issues = run.output?.issues || [];
  return (
    <article
      className={"run-card " + (compact ? "compact" : "")}
      data-run-id={run.id}
    >
      <div className="run-header">
        <span className="run-kind">
          <span className="run-small-icon">
            {run.type === "review" ? <Eye size={13} /> : <Sparkles size={13} />}
          </span>
          {taskName[run.type || run.taskType || ""] || run.type || "生成任务"}
        </span>
        <span className={"run-status " + run.status}>
          {busy(run) ? (
            <LoaderCircle size={11} className="spin" />
          ) : run.status === "Succeeded" ? (
            <Check size={11} />
          ) : null}
          {statusLabel[run.status] || run.status}
        </span>
      </div>
      <div className="run-meta">
        <span>{dateTime(run.createdAt)}</span>
        <span>
          费用{" "}
          {run.costStatus === "Reported"
            ? "已记录"
            : run.costStatus === "Estimated"
              ? "估算"
              : "未知"}
        </span>
      </div>
      {run.stale && (
        <div className="warning-box">
          <AlertTriangle size={13} /> 输入版本已变化，这份结果作为历史候选保留。
        </div>
      )}
      {run.error && <div className="error-box">{run.error}</div>}
      {compact ? (
        <button className="text-button full" onClick={onOpen}>
          查看任务详情 <ChevronRight size={13} />
        </button>
      ) : (
        <>
          <TaskMemoryUsage run={run} />
          {busy(run) && (
            <div className="generating-state">
              <span className="ink-pulse" />
              <p>正在整理故事的下一步…</p>
              <small>可以继续编辑，任务会独立运行。</small>
            </div>
          )}
          {run.status === "NeedsReconciliation" && (
            <div className="warning-box">
              模型可能已执行，但结果尚不能确认。请先核对服务商记录；“重新生成”会创建新任务并可能再次计费。
            </div>
          )}
          {text && (
            <div className="candidate-preview">
              <div className="row-between">
                <span className="section-label">独立候选 · 尚未采用</span>
                <button
                  className="icon-button"
                  aria-label="复制候选稿"
                  title="复制候选稿"
                  onClick={() =>
                    void navigator.clipboard
                      .writeText(text)
                      .then(() => notify("候选稿已复制"))
                      .catch(() => notify("浏览器未允许复制，请手动选择文本。"))
                  }
                >
                  <Copy size={13} />
                </button>
              </div>
              <pre>{text}</pre>
            </div>
          )}
          {onContinue &&
            ["draft", "revision", "rewrite"].includes(run.type) &&
            text && (
              <button
                className="button"
                disabled={!canApply || !canContinueOwn || working}
                onClick={() => onContinue(run)}
              >
                <Sparkles size={13} />
                和创作伙伴继续打磨
              </button>
            )}
          {!!issues.length && (
            <div className="issues-list">
              <span className="section-label">
                发现 {issues.length} 项待核对问题
              </span>
              {issues.map((issue, index) => (
                <div className="issue-card" key={index}>
                  <label className="issue-select">
                    <input
                      type="checkbox"
                      checked={selectedIssues.some(
                        (i) => i.quote === issue.quote && i.kind === issue.kind,
                      )}
                      onChange={(e) =>
                        onSelectIssues(
                          e.target.checked
                            ? [
                                ...selectedIssues.filter(
                                  (i) =>
                                    i.quote !== issue.quote ||
                                    i.kind !== issue.kind,
                                ),
                                issue,
                              ]
                            : selectedIssues.filter(
                                (i) =>
                                  i.quote !== issue.quote ||
                                  i.kind !== issue.kind,
                              ),
                        )
                      }
                    />
                    <span>
                      {issue.kind === "CharacterKnowledge"
                        ? "人物知识"
                        : issue.kind === "Timeline"
                          ? "时间线"
                          : "事实一致性"}
                    </span>
                    <span className="pill amber">
                      {issue.severity === "Critical" ? "严重" : "需复核"}
                    </span>
                  </label>
                  <blockquote>{issue.quote}</blockquote>
                  <p>{issue.explanation}</p>
                  <div className="issue-action">
                    <ArrowRight size={12} />
                    <span>{issue.suggestedAction}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
          {run.type === "review" &&
            run.status === "Succeeded" &&
            !issues.length && (
              <div className="review-clear">
                <CheckCircle2 size={23} />
                <div>
                  <strong>本次未检出有证据的矛盾</strong>
                  <p>仍请保留你的判断；模型审稿不能保证没有遗漏。</p>
                </div>
              </div>
            )}
          {run.type === "rewrite" && !uniqueSelection && (
            <div className="warning-box">
              选段无法唯一定位。请在正文选择一段独特的文本，再生成修订。
            </div>
          )}
          {run.output?.proposalId && (
            <p className="success-box">
              <Brain size={14} /> 记忆提议已生成，请到“故事记忆”逐项审核。
            </p>
          )}
          {text &&
            ["draft", "revision", "rewrite"].includes(
              run.type || run.taskType || "",
            ) && (
              <button
                className="button primary full"
                disabled={!canApply || working}
                title={
                  !canApply ? "当前正文已变化，请重新生成或手工比较采用" : ""
                }
                onClick={() => {
                  const candidate =
                    run.type === "rewrite" &&
                    selection &&
                    currentText.includes(selection)
                      ? currentText.replace(selection, text)
                      : text;
                  setHunks(paragraphDiff(currentText, candidate));
                  setComparing(!comparing);
                }}
              >
                <FileText size={14} /> 比较并选择修订
              </button>
            )}
          {comparing && (
            <div className="diff-panel">
              <div className="row-between">
                <span className="section-label">按段选择采用</span>
                <button
                  className="text-button"
                  onClick={() =>
                    setHunks(hunks.map((h) => ({ ...h, accepted: true })))
                  }
                >
                  全选
                </button>
              </div>
              <p className="muted small">
                只采用你勾选的变更，结果将保存为新的正文版本。
              </p>
              {hunks.map((h) => (
                <div
                  className={"diff-hunk " + (h.changed ? "changed" : "")}
                  key={h.id}
                >
                  {h.changed ? (
                    <>
                      <label>
                        <input
                          type="checkbox"
                          checked={h.accepted}
                          onChange={(e) =>
                            setHunks(
                              hunks.map((x) =>
                                x.id === h.id
                                  ? { ...x, accepted: e.target.checked }
                                  : x,
                              ),
                            )
                          }
                        />{" "}
                        采用这处修改
                      </label>
                      {h.before && (
                        <pre className="diff-removed">{h.before}</pre>
                      )}
                      {h.after && <pre className="diff-added">{h.after}</pre>}
                    </>
                  ) : (
                    <pre>{h.before}</pre>
                  )}
                </div>
              ))}
              <button
                className="button primary full"
                disabled={
                  !canApply ||
                  !hunks.some((h) => h.changed && h.accepted) ||
                  working
                }
                onClick={() => {
                  setWorking(true);
                  onApply(applyHunks(hunks), run.id)
                    .catch((error) => notify(message(error)))
                    .finally(() => setWorking(false));
                }}
              >
                保存选中的修订
              </button>
            </div>
          )}
          <button
            className="context-toggle"
            onClick={() => setShowContext(!showContext)}
          >
            <Link2 size={13} /> 本次使用了哪些上下文？
            {showContext ? (
              <ChevronDown size={13} />
            ) : (
              <ChevronRight size={13} />
            )}
          </button>
          {showContext && (
            <div className="context-inspector">
              <span className="section-label">冻结的输入快照</span>
              <pre>{JSON.stringify(run.input, null, 2)}</pre>
              <div className="token-note">
                输入 {run.usage?.inputTokens ?? "未知"} / 输出{" "}
                {run.usage?.outputTokens ?? "未知"} Token
              </div>
            </div>
          )}
        </>
      )}
      <div className="run-controls">
        {busy(run) && (
          <>
            <button disabled={working} onClick={() => void operate("pause")}>
              <Pause size={12} /> 暂停
            </button>
            <button disabled={working} onClick={() => void operate("cancel")}>
              <Square size={11} /> 取消
            </button>
          </>
        )}
        {run.status === "Paused" && (
          <button disabled={working} onClick={() => void operate("resume")}>
            <Play size={12} /> 恢复
          </button>
        )}
        {run.status === "Failed" && (
          <button disabled={working} onClick={() => void operate("retry")}>
            <RefreshCw size={12} /> 重试失败步骤
          </button>
        )}
        {!busy(run) && run.status !== "Paused" && (
          <button disabled={working} onClick={() => void operate("regenerate")}>
            <Sparkles size={12} /> 重新生成
          </button>
        )}
        <code title={run.id}>{run.id.slice(0, 8)}</code>
      </div>
    </article>
  );
}

function MemoryPanel(
  props: Props & { onExtract: () => void; starting: boolean },
) {
  const { detail, chapter, notify, onSaved } = props;
  const entityName = (value: string) =>
    detail.settings.find((s) => s.id === value)?.name || value;
  const qc = useQueryClient();
  const [selectedProposal, setSelectedProposal] = useState(""),
    [decisions, setDecisions] = useState<Record<string, string>>({}),
    [working, setWorking] = useState(false),
    [editID, setEditID] = useState(""),
    [editText, setEditText] = useState("");
  const state = useQuery({
    queryKey: ["state", detail.project.id],
    queryFn: () => api<StateView>(`/projects/${detail.project.id}/state`),
    refetchInterval: 4000,
  });
  const proposals = useQuery({
    queryKey: ["proposals", detail.project.id],
    queryFn: () =>
      api<Proposal[]>(`/projects/${detail.project.id}/memory-proposals`),
    refetchInterval: 3000,
  });
  const rebuild = useQuery({
    queryKey: ["rebuild", detail.project.id],
    queryFn: () =>
      api<{
        projectId: string;
        epoch: number;
        baseStateId: string;
        chapterIds: string[];
      }>(`/projects/${detail.project.id}/rebuild`),
    enabled: detail.project.health === "RebuildRequired",
  });
  const proposal =
    proposals.data?.find((p) => p.id === selectedProposal) ||
    proposals.data?.find(
      (p) => p.chapterId === chapter.id && p.status !== "Applied",
    ) ||
    proposals.data?.[0];
  useEffect(() => {
    if (proposal)
      setDecisions(
        Object.fromEntries(
          (proposal.changes || []).map((c, i) => [
            c.id || String(i),
            c.decision || "",
          ]),
        ),
      );
  }, [proposal?.id]);
  const changes = proposal?.changes || [];
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["state", detail.project.id] });
    void qc.invalidateQueries({ queryKey: ["proposals", detail.project.id] });
    void qc.invalidateQueries({ queryKey: ["rebuild", detail.project.id] });
    onSaved();
  };
  const decide = async () => {
    if (!proposal) return;
    setWorking(true);
    try {
      await api(`/memory-proposals/${proposal.id}/decisions`, "POST", {
        decisions: changes.map((c, i) => ({
          id: c.id || String(i),
          decision: decisions[c.id || String(i)] || "Reject",
        })),
      });
      refresh();
      notify("审核决定已保存；可以提交记忆状态。");
    } catch (e) {
      notify(message(e));
    } finally {
      setWorking(false);
    }
  };
  const apply = async () => {
    if (!proposal) return;
    setWorking(true);
    try {
      await api(`/memory-proposals/${proposal.id}/apply`, "POST", {
        expectedStateVersionId:
          proposal.baseStateVersionId || proposal.baseStateId || "",
        sourceRevisionId: proposal.sourceRevisionId,
      });
      refresh();
      notify("已确认的记忆变更已提交。");
    } catch (e) {
      notify(message(e));
    } finally {
      setWorking(false);
    }
  };
  return (
    <>
      <div className="panel-section-heading">
        <div>
          <span className="eyebrow">A WORLD THAT REMEMBERS</span>
          <h3>让故事记住发生过的事</h3>
        </div>
        <Brain size={20} />
      </div>
      <div
        className={
          "memory-health " +
          (detail.project.health === "Ready" ? "ready" : "pending")
        }
      >
        <span className="tiny-dot" />
        <strong>
          {statusLabel[detail.project.health] || detail.project.health}
        </strong>
        <small>
          {detail.project.stateVersionId
            ? "版本 " + detail.project.stateVersionId.slice(0, 8)
            : "尚无已确认记忆"}
        </small>
      </div>
      <p className="muted small">
        位置、伤势、物品、人物认知与伏笔。正文定稿后提取，再由你确认；未采用草稿不进入长期记忆。
      </p>
      {detail.project.health === "RebuildRequired" && (
        <div className="warning-box">
          <AlertTriangle size={14} />
          <div>
            旧稿发生了变化。请从下方最早待重建章节开始，逐章定稿、提取并审核记忆。
            {rebuild.data?.chapterIds.map((cid) => {
              const c = detail.chapters.find((c) => c.id === cid);
              return (
                <p key={cid}>
                  第 {c?.order} 章 · {c?.title || cid}
                </p>
              );
            })}
          </div>
        </div>
      )}
      <TaskMemoryExclusion taskType="memory-extraction" />
      <button
        className="button primary full"
        disabled={props.starting || !chapter.finalRevisionId}
        onClick={props.onExtract}
      >
        <Sparkles size={15} />
        {props.starting ? "正在创建提取任务" : "提取本章记忆变更"}
      </button>
      {!chapter.finalRevisionId && (
        <p className="muted small">先将正文定稿，才能为这一章提取记忆。</p>
      )}
      {!!proposals.data?.length && (
        <select
          className="proposal-select"
          aria-label="选择记忆提案"
          value={proposal?.id || ""}
          onChange={(e) => setSelectedProposal(e.target.value)}
        >
          {proposals.data.map((p) => (
            <option key={p.id} value={p.id}>
              {p.chapterId === chapter.id ? "本章" : "其他章"} ·{" "}
              {statusLabel[p.status] || p.status} · {dateTime(p.createdAt)}
            </option>
          ))}
        </select>
      )}
      {proposal && (
        <div className="proposal-panel">
          <div className="row-between">
            <span className="section-label">
              {changes.length} 项变更 ·{" "}
              {statusLabel[proposal.status] || proposal.status}
            </span>
            {proposal.status !== "Applied" && (
              <button
                className="text-button"
                onClick={() =>
                  setDecisions(
                    Object.fromEntries(
                      changes.map((c, i) => [c.id || String(i), "Accept"]),
                    ),
                  )
                }
              >
                接受全部
              </button>
            )}
          </div>
          {changes.map((change, i) => {
            const key = change.id || String(i);
            return (
              <article className="memory-change" key={key}>
                <div className="row-between">
                  <span className="pill">{kindLabel(change.kind)}</span>
                  <span className="muted small">
                    {change.truthStatus || ""}
                  </span>
                </div>
                <h4>
                  {entityName(change.subject)} <span>{change.predicate}</span>{" "}
                  {entityName(change.object)}
                </h4>
                {change.characterId && (
                  <p className="small">
                    人物认知：{entityName(change.characterId)}
                  </p>
                )}
                <blockquote>
                  <Quote size={12} />
                  {change.evidence?.quote || change.quote || "尚无可定位证据"}
                </blockquote>
                <EvidencePreview
                  revisionId={proposal.sourceRevisionId}
                  quote={change.evidence?.quote || change.quote || ""}
                />
                {proposal.status !== "Applied" && (
                  <>
                    <div className="decision-buttons">
                      <button
                        className={
                          decisions[key] === "Accept" ? "accepted" : ""
                        }
                        onClick={() =>
                          setDecisions({ ...decisions, [key]: "Accept" })
                        }
                      >
                        <Check size={13} /> 接受
                      </button>
                      <button
                        className={
                          decisions[key] === "Reject" ? "rejected" : ""
                        }
                        onClick={() =>
                          setDecisions({ ...decisions, [key]: "Reject" })
                        }
                      >
                        <X size={13} /> 拒绝
                      </button>
                      <button
                        onClick={() => {
                          setEditID(key);
                          setEditText(JSON.stringify(change, null, 2));
                        }}
                      >
                        编辑
                      </button>
                    </div>
                    {editID === key && (
                      <div className="memory-edit">
                        <label>
                          对象或状态内容
                          <input
                            value={
                              (JSON.parse(editText || "{}") as MemoryChange)
                                .object || ""
                            }
                            onChange={(e) =>
                              setEditText(
                                JSON.stringify({
                                  ...JSON.parse(editText),
                                  object: e.target.value,
                                }),
                              )
                            }
                          />
                        </label>
                        <label>
                          人物知道、推测或误信
                          <select
                            value={
                              (JSON.parse(editText || "{}") as MemoryChange)
                                .truthStatus || "True"
                            }
                            onChange={(e) =>
                              setEditText(
                                JSON.stringify({
                                  ...JSON.parse(editText),
                                  truthStatus: e.target.value,
                                }),
                              )
                            }
                          >
                            <option value="True">已确认事实 / 人物已知</option>
                            <option value="False">人物误信</option>
                            <option value="Unknown">未知</option>
                          </select>
                        </label>
                        <label>
                          原文证据
                          <textarea
                            rows={3}
                            value={
                              (JSON.parse(editText || "{}") as MemoryChange)
                                .evidence?.quote || ""
                            }
                            onChange={(e) => {
                              const value = JSON.parse(editText);
                              setEditText(
                                JSON.stringify({
                                  ...value,
                                  evidence: {
                                    ...value.evidence,
                                    quote: e.target.value,
                                  },
                                }),
                              );
                            }}
                          />
                        </label>
                        <button
                          className="button"
                          onClick={() => {
                            let edited: MemoryChange;
                            try {
                              edited = JSON.parse(editText);
                            } catch {
                              notify(
                                "无法读取这项变更，请关闭编辑后重新打开。",
                              );
                              return;
                            }
                            setWorking(true);
                            api(
                              `/memory-proposals/${proposal.id}/decisions`,
                              "POST",
                              {
                                decisions: [
                                  {
                                    id: key,
                                    decision: "Accept",
                                    change: edited,
                                  },
                                ],
                              },
                            )
                              .then(() => {
                                setDecisions((old) => ({
                                  ...old,
                                  [key]: "Accept",
                                }));
                                setEditID("");
                                refresh();
                                notify("变更编辑已保存，证据仍由服务端校验。");
                              })
                              .catch((e) => notify(message(e)))
                              .finally(() => setWorking(false));
                          }}
                        >
                          保存这项编辑
                        </button>
                      </div>
                    )}
                  </>
                )}
              </article>
            );
          })}
          {!changes.length && (
            <p className="muted small">
              本次没有提出新记忆；仍可确认该章状态。
            </p>
          )}
          {proposal.status !== "Applied" && (
            <div className="memory-submit">
              <button
                className="button full"
                disabled={working}
                onClick={() => void decide()}
              >
                <Check size={14} /> 保存审核决定
              </button>
              <button
                className="button primary full"
                disabled={working}
                onClick={() => void apply()}
              >
                <CheckCheck size={14} /> 提交已审核记忆
              </button>
            </div>
          )}
        </div>
      )}
      <StoryMap projectId={detail.project.id} chapters={detail.chapters} />
      <details className="state-details">
        <summary>
          <Brain size={14} /> 当前已确认的记忆
        </summary>
        {state.isError ? (
          <div className="error-box">{message(state.error)}</div>
        ) : state.data ? (
          <div>
            {state.data.facts?.length ? (
              state.data.facts.map((f) => (
                <article className="fact-card" key={f.id}>
                  <span className="pill">{kindLabel(f.kind)}</span>
                  <p>
                    {entityName(f.subject)} · {f.predicate} ·{" "}
                    {entityName(f.object)}
                  </p>
                  {f.characterId && (
                    <small>
                      {entityName(f.characterId)} 的认知 ·{" "}
                      {f.truthStatus === "False"
                        ? "误信"
                        : f.truthStatus === "Unknown"
                          ? "推测 / 未知"
                          : "已知"}
                    </small>
                  )}
                  {f.evidence?.quote && (
                    <blockquote>{f.evidence.quote}</blockquote>
                  )}
                  <EvidencePreview
                    revisionId={String(
                      f.sourceRevisionId || f.evidence?.revisionId || "",
                    )}
                    quote={f.evidence?.quote || ""}
                  />
                </article>
              ))
            ) : (
              <p className="muted small">
                暂无已确认的事实；完成记忆审核后会显示在这里。
              </p>
            )}
          </div>
        ) : (
          <p className="muted">正在读取…</p>
        )}
      </details>
    </>
  );
}
function kindLabel(kind: string) {
  return (
    (
      {
        Location: "位置",
        Injury: "伤势",
        Ownership: "物品",
        Knowledge: "人物知识",
        Belief: "人物信念",
        Foreshadow: "伏笔",
        Foreshadowing: "伏笔",
        location: "位置",
        injury: "伤势",
        item: "物品",
        knowledge: "人物知识",
        foreshadow: "伏笔",
      } as Record<string, string>
    )[kind] || kind
  );
}

function EvidencePreview({
  revisionId,
  quote,
}: {
  revisionId: string;
  quote: string;
}) {
  const [open, setOpen] = useState(false);
  const source = useQuery({
    queryKey: ["evidence", revisionId],
    queryFn: () => api<{ plainText: string }>(`/revisions/${revisionId}`),
    enabled: open && !!revisionId,
  });
  if (!revisionId) return null;
  return (
    <div className="evidence-preview">
      <button className="text-button" onClick={() => setOpen(!open)}>
        <Link2 size={11} />
        {open ? "收起原文" : "查看来源正文"}
      </button>
      {open && (
        <div>
          {source.isError ? (
            <p className="small muted">{message(source.error)}</p>
          ) : source.data ? (
            <pre>
              {source.data.plainText.split(quote).map((part, i) => (
                <span key={i}>
                  {i > 0 && <mark>{quote}</mark>}
                  {part}
                </span>
              ))}
            </pre>
          ) : (
            <LoaderCircle size={13} className="spin" />
          )}
        </div>
      )}
    </div>
  );
}

function BudgetPanel({
  projectId,
  notify,
}: {
  projectId: string;
  notify: (s: string) => void;
}) {
  const qc = useQueryClient();
  const cost = useQuery({
    queryKey: ["cost", projectId],
    queryFn: () =>
      api<{
        knownEstimatedCost: number;
        unknownCalls: number;
        inputTokens: number;
        outputTokens: number;
        currency: string;
        preferences: {
          maxCost: number | null;
          currency: string;
          maxOutputTokens: number;
        };
      }>(`/projects/${projectId}/cost`),
    refetchInterval: 5000,
  });
  const [maxCost, setMaxCost] = useState(""),
    [currency, setCurrency] = useState(""),
    [tokens, setTokens] = useState("3000"),
    [editing, setEditing] = useState(false),
    [saving, setSaving] = useState(false);
  useEffect(() => {
    if (cost.data && !editing) {
      setMaxCost(
        cost.data.preferences?.maxCost == null
          ? ""
          : String(cost.data.preferences.maxCost),
      );
      setCurrency(cost.data.preferences?.currency || "");
      setTokens(String(cost.data.preferences?.maxOutputTokens || 3000));
    }
  }, [cost.data, editing]);
  return (
    <div className="budget-card">
      <div className="row-between">
        <span className="section-label">
          <Coins size={12} /> 本作品用量
        </span>
        <button className="text-button" onClick={() => setEditing(!editing)}>
          {editing ? "收起" : "预算设置"}
        </button>
      </div>
      {cost.isError ? (
        <p className="muted small">{message(cost.error)}</p>
      ) : (
        <>
          <div className="usage-counts">
            <div>
              <strong>{cost.data?.inputTokens?.toLocaleString() ?? "—"}</strong>
              <span>输入 Token</span>
            </div>
            <div>
              <strong>
                {cost.data?.outputTokens?.toLocaleString() ?? "—"}
              </strong>
              <span>输出 Token</span>
            </div>
          </div>
          <p className="small muted">
            已知估算 {cost.data?.knownEstimatedCost ?? 0}{" "}
            {cost.data?.currency || ""} · {cost.data?.unknownCalls ?? 0}{" "}
            次费用未知
          </p>
        </>
      )}
      {editing && (
        <form
          className="form-stack"
          onSubmit={(e) => {
            e.preventDefault();
            setSaving(true);
            api(`/projects/${projectId}/preferences`, "PUT", {
              maxCost: maxCost ? Number(maxCost) : null,
              currency,
              maxOutputTokens: Number(tokens),
            })
              .then(() => {
                setEditing(false);
                void qc.invalidateQueries({ queryKey: ["cost", projectId] });
                notify("预算设置已保存；价格未知时仍显示未知，不视为零费用。");
              })
              .catch((e) => notify(message(e)))
              .finally(() => setSaving(false));
          }}
        >
          <label>
            费用预算（留空为不设金额上限）
            <input
              type="number"
              min="0"
              step="0.01"
              value={maxCost}
              onChange={(e) => setMaxCost(e.target.value)}
              placeholder="未知价格不能折算金额"
            />
          </label>
          <label>
            币种
            <input
              value={currency}
              onChange={(e) => setCurrency(e.target.value)}
              placeholder="例如 CNY / USD"
              maxLength={12}
            />
          </label>
          <label>
            单次输出 Token 上限
            <input
              type="number"
              min="128"
              max="32768"
              value={tokens}
              onChange={(e) => setTokens(e.target.value)}
              required
            />
          </label>
          <button className="button" disabled={saving}>
            保存预算
          </button>
        </form>
      )}
    </div>
  );
}

type MapFact = {
  id: string;
  chapterId: string;
  chapterOrder: number;
  sourceRevisionId: string;
  kind: string;
  subject: string;
  predicate: string;
  object: string;
  characterId?: string;
  truthStatus?: string;
  storyTime?: number | null;
  evidence?: { quote: string };
};
type Relation = {
  id: string;
  source: string;
  target: string;
  predicate: string;
  kind: string;
  chapterOrder: number;
  sourceRevisionId: string;
  evidence?: { quote: string };
};
function StoryMap({
  projectId,
  chapters,
}: {
  projectId: string;
  chapters: Chapter[];
}) {
  const [view, setView] = useState("timeline");
  const map = useQuery({
    queryKey: ["map", projectId],
    queryFn: () =>
      api<{
        projectId: string;
        epoch: number;
        nodes: { id: string; name: string; kind: string }[];
        timeline: MapFact[];
        relations: Relation[];
      }>(`/projects/${projectId}/map`),
    refetchInterval: 5000,
  });
  const nodes = map.data?.nodes || [];
  const relations = map.data?.relations || [];
  const timeline = map.data?.timeline || [];
  const names = Object.fromEntries(nodes.map((n) => [n.id, n.name]));
  const positions = nodes.slice(0, 10).map((node, i) => ({
    ...node,
    x:
      140 +
      105 *
        Math.cos(
          (i / Math.max(1, Math.min(10, nodes.length))) * Math.PI * 2 -
            Math.PI / 2,
        ),
    y:
      130 +
      92 *
        Math.sin(
          (i / Math.max(1, Math.min(10, nodes.length))) * Math.PI * 2 -
            Math.PI / 2,
        ),
  }));
  return (
    <details className="story-map">
      <summary>
        <Link2 size={14} /> 故事时间线与人物关系
      </summary>
      <div className="segmented">
        <button
          className={view === "timeline" ? "active" : ""}
          onClick={() => setView("timeline")}
        >
          时间线
        </button>
        <button
          className={view === "relations" ? "active" : ""}
          onClick={() => setView("relations")}
        >
          关系图
        </button>
      </div>
      {map.isError ? (
        <p className="muted small">{message(map.error)}</p>
      ) : view === "timeline" ? (
        <div className="timeline-list">
          {timeline.length ? (
            timeline.map((f) => (
              <article key={f.id}>
                <span className="timeline-dot" />
                <span className="section-label">
                  第 {f.chapterOrder} 章 ·{" "}
                  {chapters.find((c) => c.id === f.chapterId)?.title || ""}
                </span>
                <p>
                  {names[f.subject] || f.subject} {f.predicate}{" "}
                  {names[f.object] || f.object}
                </p>
                <small>
                  {kindLabel(f.kind)} ·{" "}
                  {f.storyTime == null
                    ? "故事时间未确定"
                    : "故事第 " + f.storyTime + " 天"}
                  {f.characterId
                    ? " · " +
                      (names[f.characterId] || f.characterId) +
                      " 的认知"
                    : ""}
                </small>
                {f.evidence?.quote && (
                  <blockquote>{f.evidence.quote}</blockquote>
                )}
              </article>
            ))
          ) : (
            <p className="muted small">
              记忆确认后，来源明确的事件会出现在这里。
            </p>
          )}
        </div>
      ) : (
        <>
          {positions.length ? (
            <svg
              className="relations-map"
              viewBox="0 0 280 260"
              role="img"
              aria-label="已确认人物与物品关系图"
            >
              {relations.map((edge, i) => {
                const a = positions.find((n) => n.id === edge.source),
                  b = positions.find((n) => n.id === edge.target);
                return a && b ? (
                  <g key={edge.id || i}>
                    <title>
                      {a.name} {edge.predicate} {b.name}
                    </title>
                    <line x1={a.x} y1={a.y} x2={b.x} y2={b.y} />
                    <text x={(a.x + b.x) / 2} y={(a.y + b.y) / 2 - 5}>
                      {edge.predicate}
                    </text>
                  </g>
                ) : null;
              })}
              {positions.map((node) => (
                <g key={node.id}>
                  <circle cx={node.x} cy={node.y} r={21} />
                  <text className="node-label" x={node.x} y={node.y + 4}>
                    {node.name.length > 5
                      ? node.name.slice(0, 5) + "…"
                      : node.name}
                  </text>
                </g>
              ))}
            </svg>
          ) : (
            <p className="muted small">暂无已确认的实体关系。</p>
          )}
          {relations.map((r) => (
            <div className="relation-row" key={r.id}>
              <strong>{names[r.source] || r.source}</strong>
              <span>{r.predicate}</span>
              <strong>{names[r.target] || r.target}</strong>
              <small>第 {r.chapterOrder} 章</small>
            </div>
          ))}
        </>
      )}
    </details>
  );
}

function SearchPanel({
  detail,
  chapter,
  notify,
}: {
  detail: Detail;
  chapter: Chapter;
  notify: (s: string) => void;
}) {
  const [query, setQuery] = useState(""),
    [submitted, setSubmitted] = useState(""),
    [character, setCharacter] = useState(""),
    [scope, setScope] = useState("AuthorReview"),
    [source, setSource] = useState<{ text: string; id: string } | null>(null);
  const search = useQuery({
    queryKey: [
      "search",
      detail.project.id,
      submitted,
      character,
      scope,
      chapter.id,
    ],
    queryFn: () =>
      api<Citation[]>(
        `/projects/${detail.project.id}/search?q=${encodeURIComponent(submitted)}&beforeChapterId=${chapter.id}&characterId=${encodeURIComponent(character)}&purpose=${scope}`,
      ),
    enabled: !!submitted,
  });
  return (
    <>
      <div className="panel-section-heading">
        <div>
          <span className="eyebrow">FIND THE THREAD</span>
          <h3>回到故事的来处</h3>
        </div>
        <Search size={20} />
      </div>
      <p className="muted small">
        只检索本作品、当前有效版本和本章之前的正文。每条结果都能回看当时的来源。
      </p>
      <form
        className="search-form"
        onSubmit={(e) => {
          e.preventDefault();
          setSubmitted(query.trim());
        }}
      >
        <div className="search-box">
          <Search size={16} />
          <input
            aria-label="搜索故事历史"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="人物、物品或一句线索…"
          />
          <button
            className="icon-button"
            type="submit"
            aria-label="执行历史检索"
          >
            <ArrowRight size={16} />
          </button>
        </div>
        <label className="form-label">
          信息可见范围
          <select value={scope} onChange={(e) => setScope(e.target.value)}>
            <option value="AuthorReview">作者审稿视角</option>
            <option value="CharacterKnowledge">仅角色已经知道的信息</option>
          </select>
        </label>
        {scope === "CharacterKnowledge" && (
          <label className="form-label">
            人物
            <input
              value={character}
              onChange={(e) => setCharacter(e.target.value)}
              placeholder="输入人物稳定 ID"
              required
              list="character-ids"
            />
            <datalist id="character-ids">
              {detail.settings
                .filter((s) => s.kind === "character")
                .map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
            </datalist>
          </label>
        )}
      </form>
      {search.isFetching && (
        <div className="mini-empty">
          <LoaderCircle className="spin" />
          <p>正在查找来源…</p>
        </div>
      )}
      {search.isError && (
        <div className="error-box">{message(search.error)}</div>
      )}
      {search.data?.length === 0 && submitted && (
        <Empty
          Icon={Search}
          text="没有找到可用的证据。未知之处先留白，不补造答案。"
        />
      )}
      {search.data?.map((citation, i) => (
        <article className="citation-card" key={citation.id || i}>
          <span className="section-label">
            <Link2 size={12} />
            {detail.chapters.find((c) => c.id === citation.chapterId)?.title ||
              "来源章节"}
          </span>
          <blockquote>{citation.quote || citation.content}</blockquote>
          <div className="row-between">
            <small>版本 {citation.revisionId.slice(0, 8)}</small>
            <button
              className="text-button"
              onClick={() =>
                void api<{ plainText: string }>(
                  `/revisions/${citation.revisionId}`,
                )
                  .then((r) =>
                    setSource({ text: r.plainText, id: citation.revisionId }),
                  )
                  .catch((e) => notify(message(e)))
              }
            >
              查看原文
              <ArrowUpRight size={12} />
            </button>
          </div>
        </article>
      ))}
      {source && (
        <div className="source-preview">
          <div className="row-between">
            <span className="section-label">
              来源版本 {source.id.slice(0, 8)}
            </span>
            <button
              className="icon-button"
              aria-label="关闭来源"
              onClick={() => setSource(null)}
            >
              <X size={14} />
            </button>
          </div>
          <pre>{source.text}</pre>
        </div>
      )}
      <button
        className="text-button reindex-button"
        onClick={() =>
          void api(`/projects/${detail.project.id}/reindex`, "POST", {})
            .then(() => {
              notify("历史检索索引已重建");
              void search.refetch();
            })
            .catch((e) => notify(message(e)))
        }
      >
        <RefreshCw size={12} /> 从已确认正文重建索引
      </button>
      <p className="muted small">
        关键词与实体检索；尚未启用未经评估的向量模型。
      </p>
    </>
  );
}
