import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDown,
  ArrowUp,
  Brain,
  Check,
  ChevronDown,
  Copy,
  FileText,
  GitCompareArrows,
  History,
  LoaderCircle,
  MessageCircle,
  Plus,
  Quote,
  Scissors,
  Sparkles,
  Square,
  Wrench,
  X,
} from "lucide-react";
import {
  api,
  dateTime,
  id,
  message,
  type Chapter,
  type Detail,
  type Run,
  type Revision,
} from "./api";
import {
  applyHunks,
  candidateDocumentText,
  canApplyCandidate,
  paragraphDiff,
  type Hunk,
} from "./revisions";
import {
  copilotBusy,
  copilotStatus,
  copilotTimeline,
  mergeCopilotEvents,
  toolNames,
  type CopilotArtifact,
  type CopilotDetail,
  type CopilotEvent,
  type CopilotSession,
  type CopilotTurn,
  type ToolData,
} from "./copilotEvents";
import type { WritingSkillList } from "./WritingSkills";
import {
  AuthorMemoryProposalCard,
  type AuthorMemoryList,
} from "./AuthorMemories";
import { openAuthorMemories } from "./authorMemoryEvents";
import "./copilot.css";

export type CandidateTarget = {
  runId: string;
  title: string;
  selection?: string;
};
type Props = {
  composeRequest?: { id: number; text: string };
  candidateRequest?: { id: number; text: string; target: CandidateTarget };
  onRefine?: (text: string, target?: CandidateTarget) => void;
  onBusyChange?: (busy: boolean) => void;
  scope?: "book" | "chapter";
  briefVersion?: number;
  artifactReady?: boolean;
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
};

export default function Copilot(props: Props) {
  const pid = props.detail.project.id;
  const book = props.scope === "book",
    sessionKey = `novel-copilot:${pid}${book ? ":book" : ""}`;
  const qc = useQueryClient();
  const [sessionID, setSessionID] = useState(
    () => localStorage.getItem(sessionKey) || "",
  );
  const [draft, setDraft] = useState("");
  const [candidateTarget, setCandidateTarget] =
    useState<CandidateTarget | null>(null);
  const [working, setWorking] = useState(false);
  const [showSessions, setShowSessions] = useState(false);
  const [includeSelection, setIncludeSelection] = useState(true);
  const [ignoreMemory, setIgnoreMemory] = useState(false);
  const [pointOfView, setPointOfView] = useState(
    () => localStorage.getItem(`novel-copilot-pov:${pid}`) || "",
  );
  useEffect(() => {
    if (pointOfView)
      localStorage.setItem(`novel-copilot-pov:${pid}`, pointOfView);
    else localStorage.removeItem(`novel-copilot-pov:${pid}`);
  }, [pid, pointOfView]);
  useEffect(() => setIncludeSelection(true), [props.selection]);
  const [events, setEvents] = useState<Record<string, CopilotEvent[]>>({});
  const [streamError, setStreamError] = useState(false);
  const [following, setFollowing] = useState(true);
  const log = useRef<HTMLDivElement>(null);
  const end = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const lists = useQuery({
    queryKey: ["copilot-sessions", pid, props.scope || "chapter"],
    queryFn: () =>
      api<CopilotSession[]>(
        `/projects/${pid}/copilot/sessions?scope=${props.scope || "chapter"}`,
      ),
  });
  const authorMemories = useQuery({
    queryKey: ["author-memories", pid],
    queryFn: () =>
      api<AuthorMemoryList>(
        `/author-memories?projectId=${encodeURIComponent(pid)}`,
      ),
  });
  const writingSkills = useQuery({
    queryKey: ["writing-skills", pid],
    queryFn: () => api<WritingSkillList>(`/projects/${pid}/writing-skills`),
  });
  const conversation = useQuery({
    queryKey: ["copilot-session", sessionID],
    queryFn: () => api<CopilotDetail>(`/copilot/sessions/${sessionID}`),
    enabled: !!sessionID,
    refetchInterval: (query) =>
      query.state.data?.turns.some((turn) => copilotBusy(turn.status))
        ? 1500
        : false,
  });
  const turns = useMemo(
    () =>
      [...(conversation.data?.turns || [])].sort((a, b) =>
        a.createdAt.localeCompare(b.createdAt),
      ),
    [conversation.data?.turns],
  );
  const active = turns.find((turn) => copilotBusy(turn.status));
  useEffect(() => {
    props.onBusyChange?.(working || !!active);
  }, [working, !!active, props.onBusyChange]);
  useEffect(() => {
    if (!props.composeRequest) return;
    setCandidateTarget(null);
    setIncludeSelection(true);
    const requestText = props.composeRequest.text;
    setDraft((current) =>
      current.trim() ? `${current}\n\n${requestText}` : requestText,
    );
    const frame = requestAnimationFrame(() => input.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [props.composeRequest?.id]);
  useEffect(() => {
    if (!props.candidateRequest) return;
    const request = props.candidateRequest;
    setCandidateTarget(request.target);
    setDraft((current) =>
      current.trim() ? current + "\n\n" + request.text : request.text,
    );
    const frame = requestAnimationFrame(() => input.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, [props.candidateRequest?.id]);
  const unreadable = conversation.isError;
  useEffect(() => {
    if (!lists.data) return;
    if (
      !sessionID ||
      (conversation.isError &&
        !lists.data.some((session) => session.id === sessionID))
    )
      setSessionID(lists.data[0]?.id || "");
  }, [lists.data, sessionID, conversation.isError]);
  useEffect(() => {
    if (sessionID) localStorage.setItem(sessionKey, sessionID);
    else localStorage.removeItem(sessionKey);
    setEvents({});
    setCandidateTarget(null);
    setFollowing(true);
    setStreamError(false);
  }, [pid, sessionID, sessionKey]);
  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["copilot-session", sessionID] });
    void qc.invalidateQueries({ queryKey: ["copilot-sessions", pid] });
  }, [pid, qc, sessionID]);
  // Full persisted replay restores tool results, citations and candidate cards.
  useEffect(() => {
    let disposed = false;
    void Promise.all(
      turns.map(async (turn) => {
        let history: CopilotEvent[] = [];
        for (let page = 0; page < 100; page++) {
          const batch = await api<CopilotEvent[]>(
            `/copilot/turns/${turn.id}/events?format=json&after=${history.at(-1)?.id || 0}`,
          );
          history = mergeCopilotEvents(history, batch);
          if (batch.length < 200) break;
        }
        if (!disposed)
          setEvents((previous) => ({
            ...previous,
            [turn.id]: mergeCopilotEvents(previous[turn.id] || [], history),
          }));
      }),
    ).catch(() => {
      if (!disposed) setStreamError(true);
    });
    return () => {
      disposed = true;
    };
  }, [turns]);
  useEffect(() => {
    if (!active) return;
    const source = new EventSource(`/api/copilot/turns/${active.id}/events`, {
      withCredentials: true,
    });
    const receive = (raw: MessageEvent) => {
      try {
        const parsed = JSON.parse(raw.data) as Record<string, unknown>;
        const persisted: CopilotEvent = parsed.kind
          ? (parsed as unknown as CopilotEvent)
          : { id: Number(raw.lastEventId), kind: raw.type, data: parsed };
        setEvents((previous) => ({
          ...previous,
          [active.id]: mergeCopilotEvents(previous[active.id] || [], [
            persisted,
          ]),
        }));
        setStreamError(false);
        if (["completed", "status"].includes(persisted.kind)) refresh();
        if (persisted.kind === "tool-result")
          void qc.invalidateQueries({ queryKey: ["author-memories"] });
      } catch {
        setStreamError(true);
      }
    };
    for (const name of [
      "status",
      "delta",
      "tool-start",
      "tool-result",
      "usage",
      "completed",
      "compaction",
    ])
      source.addEventListener(name, receive as EventListener);
    source.onmessage = receive;
    source.onerror = () => {
      setStreamError(true);
      refresh();
    };
    return () => source.close();
  }, [active?.id, refresh]);
  useEffect(() => {
    if (following)
      end.current?.scrollIntoView({ block: "nearest", behavior: "auto" });
  }, [events, turns, following]);

  const createSession = async (clearDraft = true) => {
    const session = await api<CopilotSession>(
      `/projects/${pid}/copilot/sessions`,
      "POST",
      {
        title: `${props.chapter.title} · 灵感对话`,
        chapterId: props.chapter.id,
        scope: props.scope || "chapter",
      },
    );
    setSessionID(session.id);
    await qc.invalidateQueries({ queryKey: ["copilot-sessions", pid] });
    setShowSessions(false);
    if (clearDraft) setDraft("");
    return session.id;
  };
  const send = async (text = draft, mode = "chat") => {
    if (working || active || (!text.trim() && mode !== "compact")) return;
    setWorking(true);
    try {
      const revision = await props.flush();
      const selected = sessionID || (await createSession(false));
      const turn = await api<CopilotTurn>(
        `/copilot/sessions/${selected}/turns`,
        "POST",
        {
          intentId: id(),
          text: text.trim(),
          chapterId: props.chapter.id,
          inputRevisionId: revision,
          selection: candidateTarget
            ? ""
            : includeSelection
              ? props.selection
              : "",
          candidateRunId:
            mode === "compact" ? "" : candidateTarget?.runId || "",
          candidateSelection:
            mode === "compact" ? "" : candidateTarget?.selection || "",
          pointOfViewCharacterId: book ? "" : pointOfView,
          ignoreMemory,
          maxSteps: 8,
          mode,
          scope: props.scope || "chapter",
        },
      );
      setSessionID(selected);
      setDraft("");
      setIgnoreMemory(false);
      setCandidateTarget(null);
      setFollowing(true);
      qc.setQueryData<CopilotDetail>(["copilot-session", selected], (old) =>
        old ? { ...old, turns: [...old.turns, turn] } : old,
      );
      await qc.invalidateQueries({ queryKey: ["copilot-session", selected] });
    } catch (error) {
      props.notify(message(error));
    } finally {
      setWorking(false);
    }
  };
  const cancel = async () => {
    if (!active) return;
    try {
      await api(`/copilot/turns/${active.id}/cancel`, "POST", {});
      refresh();
      props.notify("停止请求已记录，已产生的内容会保留。");
    } catch (error) {
      props.notify(message(error));
    }
  };

  return (
    <section
      className={`copilot${book ? " copilot-book" : " copilot-chapter"}`}
      aria-label="小说 Copilot"
    >
      <div className="copilot-toolbar">
        <button
          className="copilot-session-title"
          onClick={() => setShowSessions(!showSessions)}
          aria-expanded={showSessions}
        >
          <MessageCircle size={14} />
          <span>{conversation.data?.session.title || "新的灵感对话"}</span>
          <ChevronDown size={12} />
        </button>
        <button
          className="icon-button"
          title="压缩对话上下文"
          aria-label="压缩对话上下文"
          disabled={!turns.length || !!active || working}
          onClick={() => void send("", "compact")}
        >
          <Scissors size={14} />
        </button>
        <button
          className="icon-button"
          title="新建对话"
          aria-label="新建对话"
          disabled={working}
          onClick={() => {
            setWorking(true);
            void createSession()
              .catch((error) => props.notify(message(error)))
              .finally(() => setWorking(false));
          }}
        >
          <Plus size={16} />
        </button>
      </div>
      {showSessions && (
        <div className="copilot-sessions" aria-label="对话历史">
          <span className="section-label">
            <History size={12} /> 我的作品对话
          </span>
          {lists.data?.map((session) => (
            <button
              className={sessionID === session.id ? "active" : ""}
              key={session.id}
              onClick={() => {
                setSessionID(session.id);
                setShowSessions(false);
              }}
            >
              <span>{session.title}</span>
              <small>{dateTime(session.updatedAt)}</small>
            </button>
          ))}
          {!lists.data?.length && <p>发出第一条消息，这段对话会留在作品中。</p>}
        </div>
      )}
      <div
        className={`copilot-context${book ? "" : " copilot-context-chapter"}`}
      >
        <span title={props.chapter.title}>
          <FileText size={12} />
          {props.chapter.title}
        </span>
        <span>
          <Brain size={12} />
          {props.detail.settings.length} 项设定 ·{" "}
          {book ? "全书创作方向" : "故事记忆"}
        </span>
        {!book && (
          <label className="copilot-pov">
            <span>记忆视角</span>
            <select
              aria-label="对话记忆视角"
              value={pointOfView}
              onChange={(event) => setPointOfView(event.target.value)}
              disabled={!!active || working}
            >
              <option value="">作者视角</option>
              {props.detail.settings
                .filter((setting) => setting.kind === "character")
                .map((setting) => (
                  <option key={setting.id} value={setting.id}>
                    {setting.name}
                  </option>
                ))}
            </select>
          </label>
        )}
      </div>
      <div className="copilot-author-memory" aria-label="本轮作者记忆设置">
        <button
          type="button"
          className="text-button"
          onClick={() => openAuthorMemories({ projectId: pid })}
        >
          <Brain size={12} />
          <span>
            {authorMemories.isError
              ? "记忆读取失败 · 管理"
              : ignoreMemory
                ? "本轮不使用作者记忆"
                : authorMemories.data?.settings.useMemory === false
                  ? "作者记忆已关闭"
                  : `作者记忆 · ${authorMemories.data?.items.filter((item) => item.status === "active").length || 0} 条可用`}
          </span>
        </button>
        <label title="不加载作者偏好，也不从这一轮提出新记忆；切换时会重建对话上下文。">
          <input
            type="checkbox"
            checked={ignoreMemory}
            disabled={working || !!active}
            onChange={(event) => setIgnoreMemory(event.target.checked)}
          />
          本轮不使用记忆
        </label>
      </div>
      {!!writingSkills.data?.items.some(
        (skill) =>
          skill.enabled &&
          (!skill.taskTypes.length || skill.taskTypes.includes("copilot")),
      ) && (
        <details className="copilot-skills" aria-label="下次对话写作 Skills">
          <summary>
            <Sparkles size={12} /> 后续对话的写作指南
          </summary>
          <p>
            {writingSkills.data.items
              .filter(
                (skill) =>
                  skill.enabled &&
                  (!skill.taskTypes.length ||
                    skill.taskTypes.includes("copilot")),
              )
              .map((skill) => `${skill.name} v${skill.version}`)
              .join(" · ")}
          </p>
          <small>按需阅读，人物与作品设定优先。</small>
        </details>
      )}
      <div
        className="copilot-log"
        ref={log}
        role="log"
        aria-label="灵感对话记录"
        onScroll={() => {
          const el = log.current;
          if (el)
            setFollowing(el.scrollHeight - el.scrollTop - el.clientHeight < 80);
        }}
      >
        {(lists.isError || unreadable) && (
          <div className="error-box">
            {message(lists.error || conversation.error)}
            <button className="text-button" onClick={refresh}>
              重新读取对话
            </button>
          </div>
        )}
        {conversation.isLoading && (
          <p className="copilot-muted">
            <LoaderCircle className="spin" size={13} /> 正在打开对话…
          </p>
        )}
        {!turns.length && !conversation.isLoading && (
          <div className="copilot-empty">
            <span className="copilot-empty-icon">
              <Sparkles size={24} />
            </span>
            <span className="eyebrow">THINK WITH YOUR STORY</span>
            <h3>
              把念头说出来，
              <br />
              我们一起往下推敲。
            </h3>
            <p>
              {book
                ? "从核心冲突、人物欲望到卷章节奏，一起找到整本书的方向。方案会先成为候选。"
                : "讨论情节、查人物记忆，或选一段正文一起改。建议会先成为候选。"}
            </p>
            <div className="copilot-starters">
              {(book
                ? [
                    "帮我规划这本书的主线",
                    "一起推敲主角的欲望和阻碍",
                    "这个故事适合怎样的结局？",
                  ]
                : [
                    "给我三个发展方向",
                    "检查本章人物的可知信息",
                    "润色选段，保留原来的语气",
                  ]
              ).map((text, index) => (
                <button
                  key={text}
                  disabled={
                    working || (!book && index === 2 && !props.selection)
                  }
                  onClick={() => {
                    setDraft(text);
                    input.current?.focus();
                  }}
                >
                  {text}
                  <ArrowUp size={11} />
                </button>
              ))}
            </div>
          </div>
        )}
        {turns.map((turn) => (
          <Turn
            key={turn.id}
            turn={turn}
            events={events[turn.id] || []}
            {...props}
            onRefine={(text, target) => {
              setCandidateTarget(target || null);
              setDraft((current) =>
                current.trim() ? current + "\n\n" + text : text,
              );
              input.current?.focus();
            }}
          />
        ))}
        {working && !active && (
          <p className="copilot-muted">
            <LoaderCircle className="spin" size={13} /> 正在保存对话…
          </p>
        )}
        <div ref={end} />
      </div>
      {!following && (
        <button
          className="copilot-jump"
          onClick={() => {
            setFollowing(true);
            end.current?.scrollIntoView({
              block: "nearest",
              behavior: "smooth",
            });
          }}
        >
          <ArrowDown size={12} /> 回到最新消息
        </button>
      )}
      {streamError && (
        <p className="copilot-connection">
          连接正在恢复，已保存的内容仍在。
          <button onClick={refresh}>刷新</button>
        </p>
      )}
      <form
        className="copilot-composer"
        onSubmit={(event) => {
          event.preventDefault();
          void send();
        }}
      >
        {candidateTarget && (
          <div className="copilot-candidate-target" aria-label="本轮修改的候选">
            <GitCompareArrows size={13} />
            <div>
              <strong>{candidateTarget.title || "继续打磨候选"}</strong>
              <span>
                {candidateTarget.selection
                  ? "只修改候选中的选段"
                  : "基于这份候选继续修改"}{" "}
                · {candidateTarget.runId.slice(0, 8)}
              </span>
              {candidateTarget.selection && (
                <blockquote>{candidateTarget.selection}</blockquote>
              )}
            </div>
            <button
              type="button"
              aria-label="取消本轮候选引用"
              disabled={working || !!active}
              onClick={() => setCandidateTarget(null)}
            >
              <X size={13} />
            </button>
          </div>
        )}
        {!candidateTarget && props.selection && includeSelection && (
          <div className="copilot-selection">
            <Quote size={12} />
            <span>
              {props.selection.slice(0, 90)}
              {props.selection.length > 90 ? "…" : ""}
            </span>
            <button
              type="button"
              aria-label="移除本次选段"
              onClick={() => setIncludeSelection(false)}
            >
              <X size={12} />
            </button>
          </div>
        )}
        {!candidateTarget && props.selection && !includeSelection && (
          <button
            className="text-button"
            type="button"
            onClick={() => setIncludeSelection(true)}
          >
            <Quote size={12} /> 引用正文选段
          </button>
        )}
        <textarea
          ref={input}
          aria-label="给创作伙伴的消息"
          placeholder={
            book
              ? "一起推敲主角、冲突、世界和结局…"
              : props.selection
                ? "想怎样推敲这段文字？"
                : "聊聊情节，或让助手查阅故事…"
          }
          rows={3}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (
              event.key === "Enter" &&
              !event.shiftKey &&
              !event.nativeEvent.isComposing
            ) {
              event.preventDefault();
              void send();
            }
          }}
        />
        <div className="copilot-composer-footer">
          <span>
            {active ? "可以停止，输出会保留" : "Enter 发送 · Shift Enter 换行"}
          </span>
          {active ? (
            <button
              type="button"
              className="copilot-send stopping"
              onClick={() => void cancel()}
              aria-label="停止回答"
            >
              <Square size={13} /> 停止
            </button>
          ) : (
            <button
              type="submit"
              className="copilot-send"
              disabled={working || !draft.trim() || unreadable}
              aria-label="发送消息"
            >
              {working ? (
                <LoaderCircle size={14} className="spin" />
              ) : (
                <ArrowUp size={15} />
              )}
            </button>
          )}
        </div>
      </form>
      <p className="copilot-model" title={props.config?.model || "读取配置中"}>
        {props.config?.externalModel ? "使用已配置模型" : "演示模型"} ·{" "}
        {props.config?.model || "读取配置中"} · 每次修改由你采用
      </p>
    </section>
  );
}

function Turn({
  turn,
  events,
  ...props
}: Props & { turn: CopilotTurn; events: CopilotEvent[] }) {
  const timeline = useMemo(() => copilotTimeline(events), [events]);
  const live = copilotBusy(turn.status);
  const hasText = timeline.some((item) => item.kind === "text");
  return (
    <article className="copilot-turn" data-testid="copilot-turn">
      {turn.input.mode === "compact" ? (
        <div className="copilot-compact-request">
          <Scissors size={12} /> 整理这段对话的上下文
        </div>
      ) : (
        <div className="copilot-user">
          <p>{turn.input.text}</p>
          {turn.input.pointOfViewCharacterId && (
            <small className="copilot-turn-pov">
              记忆视角：
              {props.detail.settings.find(
                (setting) => setting.id === turn.input.pointOfViewCharacterId,
              )?.name || "所选人物"}
            </small>
          )}
          {!!turn.input.writingSkills?.items?.length && (
            <details className="copilot-turn-skills">
              <summary>
                <Sparkles size={11} /> 本轮写作指南 ·{" "}
                {turn.input.writingSkills.items.length} 项
              </summary>
              <p>
                {turn.input.writingSkills.items
                  .map((skill) => `${skill.name} v${skill.version}`)
                  .join(" · ")}
              </p>
            </details>
          )}
          {(turn.input.ignoreMemory || turn.input.authorMemory) && (
            <details className="copilot-turn-memory">
              <summary>
                <Brain size={11} />
                {turn.input.ignoreMemory
                  ? "本轮未使用作者记忆"
                  : turn.input.authorMemory?.enabled
                    ? `本轮已加载 ${turn.input.authorMemory.items.length} 条作者记忆`
                    : "本轮作者记忆已关闭"}
              </summary>
              {!turn.input.ignoreMemory &&
                turn.input.authorMemory?.enabled &&
                (turn.input.authorMemory.items.length ? (
                  <ul>
                    {turn.input.authorMemory.items.map((memory) => (
                      <li key={memory.id}>
                        <strong>{memory.title}</strong>
                        <span>
                          {memory.scope === "book" ? "本书" : "作者通用"} · v
                          {memory.version}
                        </span>
                        <p>{memory.content}</p>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p>本轮没有匹配的已确认记忆。</p>
                ))}
              <small>这里是本轮加载的版本；后续修改从下一轮生效。</small>
            </details>
          )}
          <button
            className="copilot-remember"
            type="button"
            disabled={live}
            onClick={() =>
              openAuthorMemories({
                projectId: props.detail.project.id,
                initialDraft: {
                  scope: "author",
                  kind: "conversation",
                  title: "对话中的记忆点",
                  content: Array.from(turn.input.text).slice(0, 1200).join(""),
                  sourceSessionId: turn.sessionId,
                  sourceTurnId: turn.id,
                  sourceQuote: Array.from(turn.input.text)
                    .slice(0, 2000)
                    .join(""),
                },
              })
            }
          >
            <Brain size={11} />
            记住这句话
          </button>
          {turn.input.candidateRunId && (
            <div className="copilot-turn-candidate">
              <GitCompareArrows size={11} />
              <span>
                基于候选 {turn.input.candidateRunId.slice(0, 8)} 继续打磨
                {turn.input.candidateSelection ? " · 仅选段" : ""}
              </span>
            </div>
          )}
          {turn.input.selection && (
            <details>
              <summary>
                <Quote size={11} /> 引用了正文选段
              </summary>
              <blockquote>{turn.input.selection}</blockquote>
            </details>
          )}
        </div>
      )}
      <div className="copilot-assistant">
        <div className="copilot-speaker">
          <Sparkles size={13} />
          <strong>创作伙伴</strong>
          <span>{copilotStatus[turn.status] || turn.status}</span>
          {live && <LoaderCircle size={12} className="spin" />}
        </div>
        {timeline.map((item) =>
          item.kind === "text" ? (
            <div className="copilot-answer" key={item.id}>
              {item.text}
            </div>
          ) : item.kind === "tool" ? (
            <ToolCard
              key={item.id}
              data={item.data}
              pending={item.pending && live}
              {...props}
              artifactReady={turn.status === "Succeeded"}
            />
          ) : (
            <p className="copilot-compact-note" key={item.id}>
              <Scissors size={12} /> 上下文已整理，原对话仍保留。
            </p>
          ),
        )}
        {!hasText && turn.text && (
          <div className="copilot-answer">{turn.text}</div>
        )}
        {live && !timeline.length && !turn.text && (
          <p className="copilot-thinking">
            <span className="ink-pulse" /> 正在梳理你的故事…
          </p>
        )}
        {turn.status === "NeedsReconciliation" && (
          <div className="warning-box">
            本次调用的结果尚不能确认。已保留输出，请先核对模型服务记录；再次发送会创建新意图并可能再次计费。
          </div>
        )}
        {turn.error && <p className="error-box">{turn.error}</p>}
        {turn.status === "Cancelled" && (
          <p className="copilot-muted">已停止，以上内容和调用记录已保留。</p>
        )}
        {!!turn.text && !live && (
          <button
            className="copilot-copy"
            aria-label="复制这条回复"
            onClick={() =>
              void navigator.clipboard
                .writeText(turn.text)
                .then(() => props.notify("回复已复制"))
                .catch((error) => props.notify(message(error)))
            }
          >
            <Copy size={11} /> 复制
          </button>
        )}
      </div>
    </article>
  );
}

function ToolCard({
  data,
  pending,
  ...props
}: Props & { data: ToolData; pending: boolean }) {
  const content = data.result?.content;
  const error = data.error || data.result?.error;
  const citations = Array.isArray(content)
    ? content
    : content && typeof content === "object" && "citations" in content
      ? (content as { citations: unknown[] }).citations
      : [];
  return (
    <div
      className={
        "copilot-tool " + (error || data.result?.isError ? "tool-error" : "")
      }
    >
      <details>
        <summary>
          {pending ? (
            <LoaderCircle size={12} className="spin" />
          ) : (
            <Wrench size={12} />
          )}
          <span>{toolNames[data.name || ""] || data.name || "故事工具"}</span>
          <small>
            {pending
              ? "处理中"
              : error || data.result?.isError
                ? "未执行"
                : data.result
                  ? "已完成"
                  : "已结束"}
          </small>
          <ChevronDown size={11} />
        </summary>
        {data.args && <pre>{JSON.stringify(data.args, null, 2)}</pre>}
        {error && <p className="error-box">{error}</p>}
        {content !== undefined && (
          <pre>
            {typeof content === "string"
              ? content
              : JSON.stringify(content, null, 2)}
          </pre>
        )}
      </details>
      {citations.length > 0 && (
        <div className="copilot-citations">
          {citations.slice(0, 4).map((citation, index) => {
            const c = citation as {
              title?: string;
              quote?: string;
              content?: string;
              chapterId?: string;
              revisionId?: string;
            };
            return (
              <details key={index}>
                <summary>
                  <Quote size={11} />
                  {c.title || `故事来源 ${index + 1}`}
                </summary>
                <blockquote>{c.quote || c.content}</blockquote>
                {c.revisionId && (
                  <small>正文版本 {c.revisionId.slice(0, 8)}</small>
                )}
              </details>
            );
          })}
        </div>
      )}
      {data.result?.artifact && (
        <Artifact artifact={data.result.artifact} {...props} />
      )}
    </div>
  );
}

function Artifact(props: Props & { artifact: CopilotArtifact }) {
  if (props.artifact.kind === "author-memory-proposal")
    return (
      <AuthorMemoryProposalCard
        memoryId={props.artifact.memoryId || props.artifact.proposalId || ""}
        projectId={props.detail.project.id}
        disabled={!props.artifactReady}
        onChanged={props.onSaved}
      />
    );
  return props.artifact.kind === "book-plan-proposal" ? (
    <BookPlanArtifact {...props} />
  ) : (
    <ChapterArtifact {...props} />
  );
}
function BookPlanArtifact({
  artifact,
  ...props
}: Props & { artifact: CopilotArtifact }) {
  const [busy, setBusy] = useState(false),
    [adoptedHere, setAdopted] = useState(false);
  const access = useQuery({
    queryKey: ["studio", props.detail.project.id, "access"],
    queryFn: () =>
      api<{ role: string }>(`/projects/${props.detail.project.id}/access`),
  });
  const expectedVersion =
    artifact.expectedVersion ?? artifact.blueprintVersion ?? 0;
  const valid = props.briefVersion === expectedVersion;
  const adopted =
    adoptedHere || (!!artifact.text && props.currentText === artifact.text);
  return (
    <div
      className="copilot-artifact"
      data-testid="copilot-artifact"
      data-artifact-kind={artifact.kind}
    >
      <div className="row-between">
        <strong>{artifact.title || "全书规划候选"}</strong>
        <span className="pill">{adopted ? "已采用" : "规划候选"}</span>
      </div>
      <pre className="copilot-artifact-preview">{artifact.text}</pre>
      {!valid && !adopted && (
        <p className="copilot-stale">
          全书设定已更新，请基于当前版本继续讨论。
        </p>
      )}
      <button
        className="button primary"
        disabled={
          busy ||
          adopted ||
          !valid ||
          !props.artifactReady ||
          !artifact.proposalId ||
          !["owner", "editor"].includes(access.data?.role || "")
        }
        onClick={() => {
          setBusy(true);
          void api(`/copilot/book-plans/${artifact.proposalId}/apply`, "POST", {
            expectedVersion,
          })
            .then(() => {
              setAdopted(true);
              props.onSaved();
              props.notify("全书规划方案已采用，可以继续完善人物与卷章。");
            })
            .catch((e) => props.notify(message(e)))
            .finally(() => setBusy(false));
        }}
      >
        {busy ? (
          <LoaderCircle size={13} className="spin" />
        ) : (
          <Check size={13} />
        )}{" "}
        {adopted ? "已采用全书方案" : "采用全书方案"}
      </button>
    </div>
  );
}
function ChapterArtifact({
  artifact,
  ...props
}: Props & { artifact: CopilotArtifact }) {
  const [comparing, setComparing] = useState(false),
    [hunks, setHunks] = useState<Hunk[]>([]),
    [working, setWorking] = useState(false),
    [adoptedHere, setAdopted] = useState(false);
  const [candidateSelection, setCandidateSelection] = useState("");
  const [compareBase, setCompareBase] = useState<"source" | "parent">("source");
  const isBlueprint = artifact.kind === "blueprint-proposal";
  const currentVersion = useQuery({
    queryKey: ["revision", props.currentRevisionId],
    queryFn: () => api<Revision>(`/revisions/${props.currentRevisionId}`),
    enabled:
      !!artifact.runId &&
      props.currentRevisionId !== artifact.sourceRevisionId &&
      artifact.kind !== "diff",
  });
  const access = useQuery({
    queryKey: ["studio", props.detail.project.id, "access"],
    queryFn: () =>
      api<{ role: string }>(`/projects/${props.detail.project.id}/access`),
  });
  const canWrite =
    !!access.data && ["owner", "editor"].includes(access.data.role);
  const adopted =
    adoptedHere ||
    (!isBlueprint && currentVersion.data?.runId === artifact.runId);
  const run = useQuery({
    queryKey: ["copilot-artifact-run", artifact.runId],
    queryFn: () => api<Run>(`/runs/${artifact.runId}`),
    enabled: !!artifact.runId,
  });
  const parentID =
    artifact.parentCandidateRunId ||
    String(run.data?.input.candidateRunId || "");
  const parent = useQuery({
    queryKey: ["copilot-artifact-run", parentID],
    queryFn: () => api<Run>(`/runs/${parentID}`),
    enabled: !!parentID,
  });
  const parentText = parent.data
    ? candidateDocumentText(parent.data)
    : undefined;
  const valid =
    run.isSuccess &&
    run.data.status === "Succeeded" &&
    artifact.chapterId === props.chapter.id &&
    canApplyCandidate(
      artifact.sourceRevisionId,
      props.currentRevisionId,
      !!run.data?.stale,
    ) &&
    (!isBlueprint ||
      artifact.blueprintVersion === props.chapter.blueprintVersion);
  const compare = () => {
    setCompareBase("source");
    setHunks(
      paragraphDiff(
        isBlueprint ? props.chapter.blueprint : props.currentText,
        artifact.text,
      ),
    );
    setComparing(true);
  };
  const adopt = async () => {
    if (
      !artifact.runId ||
      !valid ||
      !canWrite ||
      (comparing && compareBase !== "source")
    )
      return;
    setWorking(true);
    try {
      const rid = await props.flush();
      if (rid !== artifact.sourceRevisionId)
        throw new Error("正文已变化，请根据新版本重新生成候选。");
      if (isBlueprint) {
        await api(`/copilot/blueprints/${artifact.runId}/apply`, "POST", {
          expectedVersion: artifact.blueprintVersion,
        });
        props.onSaved();
        props.notify("章纲建议已采用，正文保持原样。");
      } else {
        await props.onApply(
          comparing ? applyHunks(hunks) : artifact.text,
          artifact.runId,
        );
      }
      setAdopted(true);
      setComparing(false);
    } catch (error) {
      props.notify(message(error));
    } finally {
      setWorking(false);
    }
  };
  const changed = hunks.filter((h) => h.changed);
  return (
    <div
      className="copilot-artifact"
      data-testid="copilot-artifact"
      data-artifact-run={artifact.runId}
      data-artifact-kind={artifact.kind}
    >
      <div className="row-between">
        <span className="section-label">
          {isBlueprint ? (
            <FileText size={12} />
          ) : (
            <GitCompareArrows size={12} />
          )}{" "}
          {artifact.title || (isBlueprint ? "章纲建议" : "改写候选")}
        </span>
        <span className="pill">{adopted ? "已采用" : "尚未采用"}</span>
      </div>
      {parentID && (
        <div className="copilot-candidate-lineage">
          <GitCompareArrows size={11} />
          <span>延续候选 {parentID.slice(0, 8)} · 上一稿保留</span>
        </div>
      )}
      <pre
        className="copilot-artifact-preview"
        onMouseUp={(event) => {
          const selected = window.getSelection();
          if (
            !selected ||
            !selected.rangeCount ||
            !event.currentTarget.contains(selected.anchorNode) ||
            !event.currentTarget.contains(selected.focusNode)
          )
            return;
          const text = selected.toString();
          setCandidateSelection(
            text.trim() && artifact.text.split(text).length === 2 ? text : "",
          );
        }}
      >
        {artifact.text}
      </pre>
      {!isBlueprint && candidateSelection && (
        <div className="copilot-candidate-selection">
          <Quote size={12} />
          <span>
            {candidateSelection.slice(0, 120)}
            {candidateSelection.length > 120 ? "…" : ""}
          </span>
          <button
            type="button"
            className="text-button"
            disabled={!valid || !props.artifactReady}
            onClick={() =>
              props.onRefine?.(
                "请只打磨候选中引用的这一段，保留其他段落、事实和人物口吻。",
                {
                  runId: artifact.runId!,
                  title: artifact.title,
                  selection: candidateSelection,
                },
              )
            }
          >
            只打磨这段
          </button>
        </div>
      )}
      {!valid && !adopted && artifact.kind !== "diff" && !run.isLoading && (
        <p className="copilot-stale">
          输入版本已变化，这份候选保留为历史。请基于当前正文继续讨论。
        </p>
      )}
      {!adopted && artifact.kind !== "diff" && (
        <div className="row">
          <button className="button" onClick={compare} disabled={!valid}>
            查看 Diff
          </button>
          {parentID && (
            <button
              className="button"
              disabled={!parentText}
              onClick={() => {
                setHunks(paragraphDiff(parentText || "", artifact.text));
                setCompareBase("parent");
                setComparing(true);
              }}
            >
              与上一稿比较
            </button>
          )}
          <button
            className="button primary"
            disabled={
              !valid ||
              !canWrite ||
              working ||
              !artifact.runId ||
              (comparing && compareBase === "parent") ||
              (!isBlueprint &&
                comparing &&
                !hunks.some((h) => h.changed && h.accepted))
            }
            onClick={() => void adopt()}
          >
            {working ? (
              <LoaderCircle size={12} className="spin" />
            ) : (
              <Check size={12} />
            )}{" "}
            {isBlueprint
              ? "采用章纲建议"
              : comparing
                ? "保存选中的修订"
                : "采用完整候选"}
          </button>
        </div>
      )}
      {artifact.kind === "diff" && (
        <button
          className="button"
          onClick={() => {
            setCompareBase(
              artifact.comparisonBase === "parent" ? "parent" : "source",
            );
            setHunks(
              artifact.diff || paragraphDiff(props.currentText, artifact.text),
            );
            setComparing(!comparing);
          }}
        >
          查看差异
        </button>
      )}
      {!isBlueprint &&
        artifact.kind === "revision-candidate" &&
        props.onRefine && (
          <div className="copilot-feedback" aria-label="继续打磨候选">
            <button
              type="button"
              disabled={!props.artifactReady || !valid}
              onClick={() =>
                props.onRefine?.(
                  "请继续打磨这份候选，保留已满意的表达，只修改我接下来指出的地方。",
                  { runId: artifact.runId!, title: artifact.title },
                )
              }
            >
              继续打磨这稿
            </button>
            <span>还差一点？</span>
            {["更含蓄", "少解释", "节奏更紧", "保留原声"].map(
              (label, index) => (
                <button
                  key={label}
                  type="button"
                  disabled={!props.artifactReady || !valid}
                  onClick={() =>
                    props.onRefine?.(
                      [
                        "请继续打磨这份改写候选：用动作和对话表现情绪，减少直接说明心理；保持事实和人物语气。",
                        "请继续打磨这份改写候选：删去重复解释和段尾总结，让读者自己体会；保持事实。",
                        "请继续打磨这份改写候选：压紧重复铺陈，让动作推进冲突；保留关键线索和人物意图。",
                        "请继续打磨这份改写候选：对照原文保留作者措辞、人物口吻和句子节奏，只修改必要部分。",
                      ][index],
                      { runId: artifact.runId!, title: artifact.title },
                    )
                  }
                >
                  {label}
                </button>
              ),
            )}
            <small>先放入输入框，由你补充并发送。</small>
          </div>
        )}
      {comparing && (
        <div className="copilot-diff" aria-label="候选差异">
          <div className="row-between">
            <small>
              {compareBase === "parent" ? "对比上一稿（只读）" : "对比当前原稿"}{" "}
              · {changed.length} 处变化
            </small>
            {!isBlueprint &&
              artifact.kind !== "diff" &&
              compareBase === "source" && (
                <button
                  className="text-button"
                  onClick={() =>
                    setHunks(hunks.map((h) => ({ ...h, accepted: true })))
                  }
                >
                  全选修订
                </button>
              )}
            <button className="text-button" onClick={() => setComparing(false)}>
              收起差异
            </button>
          </div>
          {hunks.map((h) => (
            <label
              key={h.id}
              className={"copilot-hunk " + (h.changed ? "changed" : "")}
            >
              {h.changed &&
                !isBlueprint &&
                artifact.kind !== "diff" &&
                compareBase === "source" && (
                  <input
                    type="checkbox"
                    checked={h.accepted}
                    onChange={(e) =>
                      setHunks(
                        hunks.map((row) =>
                          row.id === h.id
                            ? { ...row, accepted: e.target.checked }
                            : row,
                        ),
                      )
                    }
                  />
                )}
              <span>
                {h.changed && h.before && <del>{h.before}</del>}
                {h.changed ? <ins>{h.after || "（删除这段）"}</ins> : h.after}
              </span>
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
