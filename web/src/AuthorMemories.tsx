import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookOpen,
  Brain,
  Check,
  ChevronDown,
  Download,
  FileUp,
  Heart,
  LoaderCircle,
  MessageCircle,
  Pencil,
  Pin,
  Plus,
  Save,
  Search,
  SlidersHorizontal,
  Sparkles,
  Trash2,
  UserRound,
  X,
} from "lucide-react";
import { api, APIError, dateTime, message, type Project } from "./api";
import "./authorMemories.css";

export type AuthorMemoryScope = "author" | "book";
export type AuthorMemoryKind =
  "preference" | "voice" | "workflow" | "conversation";
export type AuthorMemoryStatus = "active" | "pending" | "disabled" | "rejected";
export type AuthorMemoryInput = {
  scope: AuthorMemoryScope;
  projectId: string;
  kind: AuthorMemoryKind;
  title: string;
  content: string;
  status?: AuthorMemoryStatus;
  pinned?: boolean;
  sourceSessionId?: string;
  sourceTurnId?: string;
  sourceQuote?: string;
};
export type AuthorMemory = AuthorMemoryInput & {
  id: string;
  ownerId: string;
  status: AuthorMemoryStatus;
  pinned: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
};
export type AuthorMemorySettings = {
  version: number;
  useMemory: boolean;
  suggestMemories: boolean;
};
export type AuthorMemoryList = {
  items: AuthorMemory[];
  version: number;
  settings: AuthorMemorySettings;
};
const kindLabels: Record<AuthorMemoryKind, string> = {
  preference: "创作偏好",
  voice: "文风与表达",
  workflow: "合作习惯",
  conversation: "对话记忆",
};
const statusLabels: Record<AuthorMemoryStatus, string> = {
  active: "已记住",
  pending: "等你确认",
  disabled: "已停用",
  rejected: "未采用",
};
const kinds = Object.keys(kindLabels) as AuthorMemoryKind[];
const emptyDraft = (): AuthorMemoryInput => ({
  scope: "author",
  projectId: "",
  kind: "preference",
  title: "",
  content: "",
  status: "active",
  pinned: false,
});
const memoryPath = (projectId?: string) =>
  "/author-memories" +
  (projectId ? `?projectId=${encodeURIComponent(projectId)}` : "");
const length = (text: string) => Array.from(text).length;
const textMatch = (item: AuthorMemory, query: string) =>
  `${item.title} ${item.content} ${kindLabels[item.kind]}`
    .toLocaleLowerCase()
    .includes(query.trim().toLocaleLowerCase());
const inputFromMemory = (item: AuthorMemory): AuthorMemoryInput => ({
  scope: item.scope,
  projectId: item.projectId || "",
  kind: item.kind,
  title: item.title,
  content: item.content,
  pinned: item.pinned,
  status: item.status,
});

function MemorySource({ item }: { item: Pick<AuthorMemory, "sourceQuote"> }) {
  return item.sourceQuote ? (
    <details className="author-memory-source">
      <summary>
        <MessageCircle size={12} />
        来自你的对话
        <ChevronDown size={12} />
      </summary>
      <blockquote>{item.sourceQuote}</blockquote>
      <small>保留的是你的原话，记忆内容可以由你修正。</small>
    </details>
  ) : (
    <span className="author-memory-source-label">由你主动记录或导入</span>
  );
}

export default function AuthorMemories({
  projectId,
  onClose,
  notify,
  initialDraft,
}: {
  projectId?: string;
  onClose: () => void;
  notify: (text: string) => void;
  initialDraft?: Partial<AuthorMemoryInput>;
}) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["author-memories", projectId || ""],
    queryFn: () => api<AuthorMemoryList>(memoryPath(projectId)),
  });
  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => api<Project[]>("/projects"),
  });
  const bookTitle = (id: string) =>
    projects.data?.find((project) => project.id === id)?.title || "指定作品";
  const [view, setView] = useState<"active" | "pending" | "other">("active");
  const [kind, setKind] = useState<"all" | AuthorMemoryKind>("all");
  const [scope, setScope] = useState<"all" | AuthorMemoryScope>("all");
  const [search, setSearch] = useState("");
  const [working, setWorking] = useState("");
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [forgetId, setForgetId] = useState("");
  const [editing, setEditing] = useState<AuthorMemory | null>(null);
  const [formOpen, setFormOpen] = useState(!!initialDraft);
  const [draft, setDraft] = useState<AuthorMemoryInput>(() => ({
    ...emptyDraft(),
    ...initialDraft,
  }));
  const [draftDirty, setDraftDirty] = useState(!!initialDraft);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settings, setSettings] = useState<AuthorMemorySettings | null>(null);
  const [settingsDirty, setSettingsDirty] = useState(false);
  const [settingsConflict, setSettingsConflict] = useState(false);
  const [importItems, setImportItems] = useState<AuthorMemoryInput[] | null>(
    null,
  );
  const [importName, setImportName] = useState("");
  const [remapBook, setRemapBook] = useState(false);
  const panel = useRef<HTMLElement>(null);
  const formElement = useRef<HTMLFormElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const initialFocus = useRef<HTMLButtonElement>(null);
  const latestClose = useRef(onClose);
  const busyRef = useRef(false);
  const dirtyRef = useRef(false);
  latestClose.current = onClose;
  busyRef.current = !!working;
  dirtyRef.current = draftDirty || settingsDirty;
  const askClose = () => {
    if (busyRef.current) return;
    if (
      dirtyRef.current &&
      !window.confirm(
        "还有未保存的记忆修改，关闭后将放弃这些修改。仍要关闭吗？",
      )
    )
      return;
    latestClose.current();
  };
  const closeRef = useRef(askClose);
  closeRef.current = askClose;
  useEffect(() => {
    if (query.data && !settingsDirty) setSettings(query.data.settings);
  }, [query.data, settingsDirty]);
  useEffect(() => {
    const trigger = document.activeElement;
    initialFocus.current?.focus();
    const focusInside = (event: FocusEvent) => {
      if (panel.current && !panel.current.contains(event.target as Node))
        initialFocus.current?.focus({ preventScroll: true });
    };
    const key = (event: KeyboardEvent) => {
      if (event.isComposing) return;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        closeRef.current();
      }
      if (event.key === "Tab") {
        const controls = [
          ...(panel.current?.querySelectorAll<HTMLElement>(
            "button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, a[href]",
          ) || []),
        ].filter((x) => x.tabIndex >= 0 && x.getClientRects().length > 0);
        const first = controls[0],
          last = controls[controls.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
    };
    const unload = (event: BeforeUnloadEvent) => {
      if (dirtyRef.current) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    document.addEventListener("focusin", focusInside);
    document.addEventListener("keydown", key);
    window.addEventListener("beforeunload", unload);
    return () => {
      document.removeEventListener("focusin", focusInside);
      document.removeEventListener("keydown", key);
      window.removeEventListener("beforeunload", unload);
      if (trigger instanceof HTMLElement && trigger.isConnected)
        trigger.focus({ preventScroll: true });
    };
  }, []);
  useEffect(() => {
    if (formOpen) formElement.current?.scrollIntoView({ block: "nearest" });
  }, [formOpen, editing?.id]);
  const items = query.data?.items || [];
  const pending = items.filter((x) => x.status === "pending");
  const active = items.filter((x) => x.status === "active");
  const shown = useMemo(
    () =>
      items.filter(
        (item) =>
          (view === "other"
            ? item.status === "disabled" || item.status === "rejected"
            : item.status === view) &&
          (kind === "all" || item.kind === kind) &&
          (scope === "all" || item.scope === scope) &&
          textMatch(item, search),
      ),
    [query.data, view, kind, scope, search],
  );
  const refresh = async () => {
    await qc.invalidateQueries({ queryKey: ["author-memories"] });
    void qc.invalidateQueries({ queryKey: ["author-memory-settings"] });
    void qc.invalidateQueries({ queryKey: ["copilot-sessions"] });
    void qc.invalidateQueries({ queryKey: ["copilot-session"] });
    void qc.invalidateQueries({ queryKey: ["copilot-artifact-run"] });
  };
  const action = async (key: string, fn: () => Promise<void>) => {
    if (working) return;
    setWorking(key);
    setError("");
    try {
      await fn();
    } catch (cause) {
      setError(message(cause));
    } finally {
      setWorking("");
    }
  };
  const begin = (item?: AuthorMemory) => {
    if (
      draftDirty &&
      !window.confirm("开始另一项修改将放弃当前未保存的内容，是否继续？")
    )
      return;
    setEditing(item || null);
    setDraft(item ? inputFromMemory(item) : emptyDraft());
    setFormOpen(true);
    setDraftDirty(false);
    setConflict(false);
    setError("");
    setForgetId("");
  };
  const patch = (changes: Partial<AuthorMemoryInput>) => {
    setDraft((old) => ({ ...old, ...changes }));
    setDraftDirty(true);
  };
  const updateStatus = (item: AuthorMemory, status: AuthorMemoryStatus) =>
    action(item.id, async () => {
      await api(`/author-memories/${item.id}`, "PUT", {
        expectedVersion: item.version,
        status,
      });
      await refresh();
      notify(
        status === "active"
          ? "这条记忆已启用，后续相关对话会参考。"
          : status === "rejected"
            ? "未采用这条建议。"
            : "这条记忆已停用。",
      );
    });
  const save = (activate = false) =>
    action("save", async () => {
      if (!draft.title.trim() || !draft.content.trim())
        throw new Error("写一个便于辨认的标题，再写下希望记住的内容。");
      if (length(draft.title) > 80 || length(draft.content) > 1200)
        throw new Error(
          "标题最多80字，记忆内容最多1200字。可以分成几条具体记忆。",
        );
      const body = {
        ...draft,
        title: draft.title.trim(),
        content: draft.content.trim(),
        projectId: draft.scope === "author" ? "" : draft.projectId,
        status: activate ? "active" : draft.status || "active",
      };
      try {
        if (editing)
          await api(`/author-memories/${editing.id}`, "PUT", {
            scope: body.scope,
            projectId: body.projectId,
            kind: body.kind,
            title: body.title,
            content: body.content,
            status: body.status,
            pinned: body.pinned,
            expectedVersion: editing.version,
          });
        else await api("/author-memories", "POST", body);
      } catch (cause) {
        if (editing && cause instanceof APIError && cause.status === 409)
          setConflict(true);
        throw cause;
      }
      setDraftDirty(false);
      setFormOpen(false);
      setEditing(null);
      setConflict(false);
      await refresh();
      notify("记忆已保存。你仍可随时修改或忘记。");
    });
  const saveSettings = () =>
    action("settings", async () => {
      if (!settings) return;
      try {
        await api("/author-memory-settings", "PUT", {
          expectedVersion: settings.version,
          useMemory: settings.useMemory,
          suggestMemories: settings.suggestMemories,
        });
      } catch (cause) {
        if (cause instanceof APIError && cause.status === 409)
          setSettingsConflict(true);
        throw cause;
      }
      setSettingsDirty(false);
      setSettingsConflict(false);
      await refresh();
      notify("记忆使用方式已更新。");
    });
  const loadImport = async (file: File) =>
    action("preview-import", async () => {
      setImportItems(null);
      if (file.size > 1024 * 1024)
        throw new Error("请选择1 MB以内的作者记忆JSON文件。");
      let value: unknown;
      try {
        value = JSON.parse(await file.text());
      } catch {
        throw new Error("这个文件不是有效的JSON记忆文件。");
      }
      if (!value || typeof value !== "object")
        throw new Error("请选择由作者记忆导出的JSON文件。");
      const source = value as { formatVersion?: number; items?: unknown[] };
      if (
        source.formatVersion !== 1 ||
        !Array.isArray(source.items) ||
        source.items.length > 200
      )
        throw new Error("请选择格式版本为1、最多200条的作者记忆导出文件。");
      const parsed: AuthorMemoryInput[] = source.items.map((raw, index) => {
        const item = raw as Partial<AuthorMemoryInput>;
        if (
          !item ||
          !["author", "book"].includes(item.scope || "") ||
          !kinds.includes(item.kind as AuthorMemoryKind) ||
          typeof item.title !== "string" ||
          !item.title.trim() ||
          length(item.title) > 80 ||
          typeof item.content !== "string" ||
          !item.content.trim() ||
          length(item.content) > 1200 ||
          (item.status !== undefined &&
            !["active", "pending", "disabled", "rejected"].includes(
              item.status,
            ))
        )
          throw new Error(`第${index + 1}条记忆的范围、类别或内容无效。`);
        return {
          scope: item.scope!,
          projectId:
            item.scope === "book" && typeof item.projectId === "string"
              ? item.projectId
              : "",
          kind: item.kind!,
          title: item.title.trim(),
          content: item.content.trim(),
          pinned: !!item.pinned,
          status: item.status || "active",
        };
      });
      setImportName(file.name);
      setImportItems(parsed);
      setRemapBook(false);
    });
  return createPortal(
    <div
      className="author-memory-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) askClose();
      }}
    >
      <section
        className="author-memory-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="作者记忆"
        ref={panel}
      >
        <header className="author-memory-heading">
          <div>
            <span className="author-memory-eyebrow">
              YOUR WRITING COMPANION
            </span>
            <h2>
              <Brain size={23} strokeWidth={1.5} />
              作者记忆
            </h2>
            <p>把值得延续的偏好和想法留下，让每次创作少一点重复说明。</p>
          </div>
          <button
            className="icon-button"
            ref={initialFocus}
            aria-label="关闭作者记忆"
            onClick={askClose}
            disabled={!!working}
          >
            <X size={20} />
          </button>
        </header>
        <div className="author-memory-body">
          <div className="author-memory-principle">
            <UserRound size={20} />
            <p>
              <strong>这是只属于你的创作记忆。</strong>
              跨作品使用的偏好与仅用于本书的要求分开保存，不会分享给其他作者，也不会自动变成故事事实。
            </p>
          </div>
          <div className="author-memory-settings-bar">
            <span className={settings?.useMemory === false ? "paused" : ""}>
              <span className="author-memory-dot" />
              {settings?.useMemory === false
                ? "已暂停使用记忆"
                : "在相关对话中参考记忆"}
            </span>
            <button
              className="text-button"
              onClick={() => setSettingsOpen(!settingsOpen)}
              aria-expanded={settingsOpen}
            >
              <SlidersHorizontal size={14} />
              记忆设置
              <ChevronDown size={12} />
            </button>
          </div>
          {settingsOpen && settings && (
            <section
              className="author-memory-settings"
              aria-label="作者记忆设置"
            >
              <label>
                <input
                  type="checkbox"
                  aria-label="在创作对话中使用作者记忆"
                  checked={settings.useMemory}
                  onChange={(e) => {
                    setSettings({ ...settings, useMemory: e.target.checked });
                    setSettingsDirty(true);
                  }}
                />
                <span>
                  <strong>在创作对话中使用记忆</strong>
                  <small>
                    根据当前任务选取相关记忆。暂停后保留条目，后续新对话轮次不再加载。
                  </small>
                </span>
              </label>
              <label>
                <input
                  type="checkbox"
                  aria-label="允许助手提出记忆建议"
                  checked={settings.suggestMemories}
                  onChange={(e) => {
                    setSettings({
                      ...settings,
                      suggestMemories: e.target.checked,
                    });
                    setSettingsDirty(true);
                  }}
                />
                <span>
                  <strong>让助手提出值得记住的内容</strong>
                  <small>
                    从你的明确偏好和纠正中提出建议，确认后才用于后续对话。
                  </small>
                </span>
              </label>
              {settingsConflict && (
                <p className="author-memory-conflict">
                  设置已在别处更新，你的选择仍保留。核对最新设置后再修改。
                  <button
                    onClick={() => {
                      setSettingsDirty(false);
                      setSettingsConflict(false);
                      void query.refetch();
                    }}
                  >
                    载入最新设置（替换当前选择）
                  </button>
                </p>
              )}
              <button
                className="button"
                disabled={!!working || !settingsDirty || settingsConflict}
                onClick={saveSettings}
              >
                {working === "settings" ? (
                  <LoaderCircle size={14} className="spin" />
                ) : (
                  <Save size={14} />
                )}
                保存记忆设置
              </button>
            </section>
          )}
          <div className="author-memory-toolbar">
            <div className="author-memory-view-tabs" aria-label="记忆状态">
              <button
                className={view === "active" ? "active" : ""}
                onClick={() => setView("active")}
                aria-pressed={view === "active"}
              >
                已记住 <span>{active.length}</span>
              </button>
              <button
                className={view === "pending" ? "active" : ""}
                onClick={() => setView("pending")}
                aria-pressed={view === "pending"}
              >
                待确认 <span>{pending.length}</span>
              </button>
              <button
                className={view === "other" ? "active" : ""}
                onClick={() => setView("other")}
                aria-pressed={view === "other"}
              >
                已停用与未采用
              </button>
            </div>
            <button
              className="button primary"
              onClick={() => begin()}
              disabled={!!working}
            >
              <Plus size={15} />
              记下一条
            </button>
          </div>
          <div className="author-memory-filters">
            <label className="author-memory-search">
              <Search size={15} />
              <input
                aria-label="搜索作者记忆"
                placeholder="搜索偏好、文风或想法…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </label>
            <select
              aria-label="筛选记忆类别"
              value={kind}
              onChange={(e) => setKind(e.target.value as typeof kind)}
            >
              <option value="all">全部类别</option>
              {kinds.map((k) => (
                <option value={k} key={k}>
                  {kindLabels[k]}
                </option>
              ))}
            </select>
            <select
              aria-label="筛选记忆范围"
              value={scope}
              onChange={(e) => setScope(e.target.value as typeof scope)}
            >
              <option value="all">
                {projectId ? "作者通用与本书" : "全部适用范围"}
              </option>
              <option value="author">作者通用</option>
              <option value="book">
                {projectId ? "仅用于本书" : "仅用于指定作品"}
              </option>
            </select>
          </div>
          {error && (
            <div className="author-memory-error" role="alert">
              {error}
            </div>
          )}
          {query.isError && (
            <div className="author-memory-error" role="alert">
              {message(query.error)}
              <button
                className="text-button"
                onClick={() => void query.refetch()}
              >
                重新读取
              </button>
            </div>
          )}
          {formOpen && (
            <form
              className="author-memory-form"
              ref={formElement}
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <div className="author-memory-form-heading">
                <h3>{editing ? "修改这条记忆" : "你希望创作伙伴记住什么？"}</h3>
                <button
                  type="button"
                  className="icon-button"
                  aria-label="收起记忆编辑"
                  disabled={!!working}
                  onClick={() => {
                    if (
                      !draftDirty ||
                      window.confirm("放弃当前尚未保存的记忆修改？")
                    ) {
                      setFormOpen(false);
                      setDraftDirty(false);
                    }
                  }}
                >
                  <X size={16} />
                </button>
              </div>
              <p className="author-memory-form-hint">
                写具体、可执行的偏好。比如“改稿时先说明问题，不直接重写全文”，比“帮我写好”更有帮助。
              </p>
              <div className="author-memory-form-row">
                <label>
                  适用范围
                  <select
                    aria-label="记忆适用范围"
                    value={draft.scope}
                    onChange={(e) =>
                      patch({
                        scope: e.target.value as AuthorMemoryScope,
                        projectId:
                          e.target.value === "book"
                            ? projectId || editing?.projectId || ""
                            : "",
                      })
                    }
                  >
                    <option value="author">我的全部作品</option>
                    <option
                      value="book"
                      disabled={!projectId && !editing?.projectId}
                    >
                      仅用于{projectId ? "当前作品" : "原作品"}
                    </option>
                  </select>
                </label>
                <label>
                  类别
                  <select
                    aria-label="记忆类别"
                    value={draft.kind}
                    onChange={(e) =>
                      patch({ kind: e.target.value as AuthorMemoryKind })
                    }
                  >
                    {kinds.map((k) => (
                      <option value={k} key={k}>
                        {kindLabels[k]}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <label>
                便于辨认的标题
                <input
                  aria-label="记忆标题"
                  maxLength={80}
                  value={draft.title}
                  placeholder="例如：对白保留潜台词"
                  onChange={(e) => patch({ title: e.target.value })}
                />
              </label>
              <label>
                {draft.kind === "voice" ? "想保留的文风特点" : "希望记住的内容"}
                <textarea
                  aria-label="记忆内容"
                  rows={4}
                  maxLength={1200}
                  value={draft.content}
                  placeholder={
                    draft.kind === "voice"
                      ? "例如：叙述克制，情绪藏在动作里。长短句随场景变化，不机械点题。可附一小段你自己的样例。"
                      : "例如：讨论情节时先给两个方向；我选定后再展开，不要直接写成长篇正文。"
                  }
                  onChange={(e) => patch({ content: e.target.value })}
                />
                <small className="author-memory-character-count">
                  {length(draft.content)} / 1200
                </small>
              </label>
              <label className="author-memory-pin">
                <input
                  type="checkbox"
                  aria-label="优先参考这条记忆"
                  checked={!!draft.pinned}
                  onChange={(e) => patch({ pinned: e.target.checked })}
                />
                <span>
                  <Pin size={13} />
                  优先参考这条记忆
                </span>
              </label>
              {(editing?.sourceQuote || draft.sourceQuote) && (
                <MemorySource item={editing || draft} />
              )}
              {conflict && (
                <div className="author-memory-conflict">
                  这条记忆已更新，你的草稿仍保留。
                  <button type="button" onClick={() => void query.refetch()}>
                    刷新服务器版本
                  </button>
                  <button
                    type="button"
                    disabled={!!working}
                    onClick={() =>
                      void action("reload-memory", async () => {
                        const result = await query.refetch();
                        if (result.error) throw result.error;
                        const current = result.data?.items.find(
                          (x) => x.id === editing?.id,
                        );
                        if (!current)
                          throw new Error(
                            "这条记忆已被移除。当前草稿仍保留，可复制内容后重新记下一条。",
                          );
                        setEditing(current);
                        setDraft(inputFromMemory(current));
                        setDraftDirty(false);
                        setConflict(false);
                        setError("");
                      })
                    }
                  >
                    重新载入最新版本（替换草稿）
                  </button>
                </div>
              )}
              <div className="author-memory-form-actions">
                <button
                  className="button primary"
                  type="submit"
                  disabled={!!working || conflict}
                >
                  {working === "save" ? (
                    <LoaderCircle size={15} className="spin" />
                  ) : (
                    <Save size={15} />
                  )}
                  保存记忆
                </button>
                {editing?.status === "pending" && (
                  <button
                    className="button"
                    type="button"
                    disabled={!!working || conflict}
                    onClick={() => void save(true)}
                  >
                    <Check size={15} />
                    保存并记住
                  </button>
                )}
                <span>你的当前要求始终可以覆盖长期偏好。</span>
              </div>
            </form>
          )}
          {query.isLoading ? (
            <div className="author-memory-empty">
              <LoaderCircle className="spin" size={26} />
              <p>正在整理你的记忆…</p>
            </div>
          ) : shown.length ? (
            <div className="author-memory-list">
              {shown.map((item) => (
                <article
                  className={`author-memory-card ${item.status}`}
                  key={item.id}
                >
                  <header>
                    <span className="author-memory-kind-icon">
                      {item.kind === "voice" ? (
                        <Heart size={17} />
                      ) : item.kind === "conversation" ? (
                        <MessageCircle size={17} />
                      ) : (
                        <Brain size={17} />
                      )}
                    </span>
                    <div>
                      <h3>
                        {item.title}
                        {item.pinned && <Pin size={12} aria-label="优先参考" />}
                      </h3>
                      <div className="author-memory-badges">
                        <span>{kindLabels[item.kind]}</span>
                        <span>
                          {item.scope === "author" ? (
                            <UserRound size={11} />
                          ) : (
                            <BookOpen size={11} />
                          )}
                          {item.scope === "author"
                            ? "作者通用"
                            : item.projectId === projectId
                              ? "仅用于本书"
                              : `仅用于《${bookTitle(item.projectId)}》`}
                        </span>
                        <span>{statusLabels[item.status]}</span>
                      </div>
                    </div>
                  </header>
                  <p className="author-memory-content">{item.content}</p>
                  <MemorySource item={item} />
                  {item.status === "pending" && (
                    <p className="author-memory-pending-note">
                      <Sparkles size={12} />
                      先核对内容和适用范围。确认后才会成为后续对话的参考。
                    </p>
                  )}
                  <footer>
                    {item.status === "pending" ? (
                      <>
                        <button
                          className="button primary"
                          disabled={!!working}
                          onClick={() => updateStatus(item, "active")}
                        >
                          <Check size={13} />
                          记住这条
                        </button>
                        <button
                          className="text-button"
                          disabled={!!working}
                          onClick={() => begin(item)}
                        >
                          <Pencil size={13} />
                          修改后记住
                        </button>
                        <button
                          className="text-button"
                          disabled={!!working}
                          onClick={() => updateStatus(item, "rejected")}
                        >
                          不记住
                        </button>
                      </>
                    ) : (
                      <>
                        <button
                          className="text-button"
                          disabled={!!working}
                          onClick={() => begin(item)}
                        >
                          <Pencil size={13} />
                          编辑
                        </button>
                        <button
                          className="text-button"
                          disabled={!!working}
                          onClick={() =>
                            updateStatus(
                              item,
                              item.status === "active" ? "disabled" : "active",
                            )
                          }
                        >
                          {item.status === "active" ? "暂时停用" : "重新启用"}
                        </button>
                      </>
                    )}
                    <button
                      className="text-button author-memory-forget"
                      disabled={!!working}
                      onClick={() =>
                        setForgetId(forgetId === item.id ? "" : item.id)
                      }
                    >
                      <Trash2 size={13} />
                      忘记
                    </button>
                    <small>{dateTime(item.updatedAt)}</small>
                  </footer>
                  {forgetId === item.id && (
                    <div className="author-memory-forget-confirm">
                      <p>
                        忘记后将删除这条长期记忆，后续对话不再加载。原始聊天和已生成稿件仍保留在各自的记录里。
                      </p>
                      <button
                        className="button"
                        disabled={!!working}
                        onClick={() => setForgetId("")}
                      >
                        保留这条
                      </button>
                      <button
                        className="button danger"
                        disabled={!!working}
                        onClick={() =>
                          action(item.id, async () => {
                            await api(
                              `/author-memories/${item.id}?expectedVersion=${item.version}`,
                              "DELETE",
                            );
                            setForgetId("");
                            await refresh();
                            notify("这条长期记忆已忘记。");
                          })
                        }
                      >
                        确认忘记
                      </button>
                    </div>
                  )}
                </article>
              ))}
            </div>
          ) : (
            <div className="author-memory-empty">
              <Brain size={34} strokeWidth={1.3} />
              <h3>
                {search || kind !== "all" || scope !== "all"
                  ? "没有找到符合条件的记忆"
                  : view === "pending"
                    ? "暂时没有待确认的记忆"
                    : view === "other"
                      ? "没有停用或未采用的条目"
                      : "从一句“我喜欢这样写”开始"}
              </h3>
              <p>
                {view === "pending"
                  ? "你在对话中表达的偏好与纠正，可以由助手整理成建议，再由你决定是否记住。"
                  : "记录你偏爱的表达、合作方式或值得延续的想法。故事人物和情节事实仍在作品的故事记忆中管理。"}
              </p>
              {view === "active" && (
                <button className="button" onClick={() => begin()}>
                  <Plus size={14} />
                  记下第一条偏好
                </button>
              )}
            </div>
          )}
          <div className="author-memory-transfer">
            <div>
              <strong>记忆由你掌握</strong>
              <p>
                只导出你自己的条目。文件可能包含私人偏好与对话摘录，请自行妥善保存。
              </p>
            </div>
            <div>
              <a className="button" href="/api/author-memories/export" download>
                <Download size={14} />
                导出记忆
              </a>
              <button
                className="button"
                disabled={!!working}
                onClick={() => fileInput.current?.click()}
              >
                <FileUp size={14} />
                导入记忆
              </button>
              <input
                hidden
                ref={fileInput}
                type="file"
                accept=".json,application/json"
                aria-label="选择作者记忆文件"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) void loadImport(file);
                  e.target.value = "";
                }}
              />
            </div>
          </div>
          {importItems && (
            <section
              className="author-memory-import"
              aria-label="作者记忆导入预览"
            >
              <header>
                <h3>导入预览 · {importItems.length} 条</h3>
                <button
                  className="icon-button"
                  aria-label="取消记忆导入"
                  onClick={() => setImportItems(null)}
                  disabled={!!working}
                >
                  <X size={16} />
                </button>
              </header>
              <p>
                {importName} ·
                尚未保存。文件内容是导入声明，不作为已核验的对话证据。
              </p>
              <ul>
                {importItems.map((item, index) => (
                  <li key={index}>
                    <span>
                      {item.scope === "author" ? "作者通用" : "指定作品"} ·{" "}
                      {kindLabels[item.kind]}
                    </span>
                    <strong>{item.title}</strong>
                    <p>{item.content}</p>
                  </li>
                ))}
              </ul>
              {projectId &&
                importItems.some((item) => item.scope === "book") && (
                  <label className="author-memory-pin">
                    <input
                      type="checkbox"
                      aria-label="将文件中的作品记忆用于当前作品"
                      checked={remapBook}
                      onChange={(e) => setRemapBook(e.target.checked)}
                    />
                    将文件中的作品记忆用于当前作品
                  </label>
                )}
              <button
                className="button primary"
                disabled={!!working || !importItems.length}
                onClick={() =>
                  action("import", async () => {
                    const body = importItems.map((item) =>
                      item.scope === "book" && remapBook && projectId
                        ? { ...item, projectId }
                        : item,
                    );
                    const result = await api<{
                      imported: number;
                      skipped: number;
                    }>("/author-memories/import", "POST", { items: body });
                    setImportItems(null);
                    await refresh();
                    notify(
                      `已导入${result.imported}条记忆${result.skipped ? `，跳过${result.skipped}条重复内容` : ""}。`,
                    );
                  })
                }
              >
                {working === "import" ? (
                  <LoaderCircle size={14} className="spin" />
                ) : (
                  <Check size={14} />
                )}
                确认导入记忆
              </button>
            </section>
          )}
        </div>
        <footer className="author-memory-footer">
          <Brain size={13} />
          只保留有用的记忆；你的想法改变了，也可以一起改变。
        </footer>
      </section>
    </div>,
    document.body,
  );
}

export function AuthorMemoryProposalCard({
  memoryId,
  projectId,
  onChanged,
  disabled = false,
}: {
  memoryId: string;
  projectId?: string;
  onChanged?: () => void;
  disabled?: boolean;
}) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["author-memories", projectId || ""],
    queryFn: () => api<AuthorMemoryList>(memoryPath(projectId)),
  });
  const item = query.data?.items.find((memory) => memory.id === memoryId);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");
  if (query.isLoading)
    return (
      <div className="author-memory-proposal">
        <LoaderCircle size={14} className="spin" />
        正在读取记忆建议…
      </div>
    );
  if (query.isError)
    return (
      <div className="author-memory-proposal">
        <p>暂时无法读取这条记忆建议。</p>
        <button className="text-button" onClick={() => void query.refetch()}>
          重新读取
        </button>
      </div>
    );
  if (!item)
    return (
      <div className="author-memory-proposal author-memory-proposal-done">
        <Brain size={15} />
        这条记忆已移除，不会再用于后续对话。
      </div>
    );
  const decide = async (status: AuthorMemoryStatus) => {
    if (disabled || working) return;
    setWorking(true);
    setError("");
    try {
      if (editing && (!title.trim() || !content.trim()))
        throw new Error("标题和内容不能为空。");
      await api(`/author-memories/${item.id}`, "PUT", {
        expectedVersion: item.version,
        status,
        ...(editing && status === "active"
          ? { title: title.trim(), content: content.trim() }
          : {}),
      });
      setEditing(false);
      await qc.invalidateQueries({ queryKey: ["author-memories"] });
      void qc.invalidateQueries({ queryKey: ["copilot-artifact-run"] });
      onChanged?.();
    } catch (cause) {
      setError(message(cause));
      if (cause instanceof APIError && cause.status === 409)
        void query.refetch();
    } finally {
      setWorking(false);
    }
  };
  return (
    <article
      className={`author-memory-proposal ${item.status}`}
      data-testid="author-memory-proposal"
      data-memory-id={item.id}
    >
      <header>
        <Brain size={17} />
        <strong>
          {item.status === "pending"
            ? "这件事，值得以后记住吗？"
            : statusLabels[item.status]}
        </strong>
      </header>
      <div className="author-memory-badges">
        <span>{kindLabels[item.kind]}</span>
        <span>
          {item.scope === "author" ? "作者通用 · 跨作品" : "仅用于本书"}
        </span>
      </div>
      {editing ? (
        <div className="author-memory-proposal-editor">
          <input
            aria-label="修改记忆建议标题"
            value={title}
            maxLength={80}
            onChange={(e) => setTitle(e.target.value)}
          />
          <textarea
            aria-label="修改记忆建议内容"
            value={content}
            rows={3}
            maxLength={1200}
            onChange={(e) => setContent(e.target.value)}
          />
        </div>
      ) : (
        <>
          <h4>{item.title}</h4>
          <p>{item.content}</p>
        </>
      )}
      <MemorySource item={item} />
      {error && (
        <p className="author-memory-error" role="alert">
          {error}
        </p>
      )}
      {disabled && item.status === "pending" && (
        <small>本轮结束后可以确认；确认后的偏好从下一轮使用。</small>
      )}
      {item.status === "pending" ? (
        <footer>
          <button
            className="button primary"
            disabled={working || disabled}
            onClick={() => void decide("active")}
          >
            {working ? (
              <LoaderCircle size={13} className="spin" />
            ) : (
              <Check size={13} />
            )}
            {editing ? "保存并记住" : "记住这条"}
          </button>
          <button
            className="text-button"
            disabled={working || disabled}
            onClick={() => {
              if (!editing) {
                setTitle(item.title);
                setContent(item.content);
              }
              setEditing(!editing);
            }}
          >
            {editing ? "取消修改" : "先修改"}
          </button>
          <button
            className="text-button"
            disabled={working || disabled}
            onClick={() => void decide("rejected")}
          >
            不记住
          </button>
        </footer>
      ) : (
        <small className="author-memory-proposal-result">
          {item.status === "active"
            ? "后续相关对话会参考；当前要求仍然优先。"
            : "不会作为后续对话的记忆参考。"}
        </small>
      )}
    </article>
  );
}
