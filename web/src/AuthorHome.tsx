import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  AlertCircle,
  Check,
  X,
  Feather,
  FilePlus2,
  FolderOpen,
  Layers3,
  LoaderCircle,
  Plus,
  Save,
  Settings2,
  Sparkles,
  Users,
  Waypoints,
} from "lucide-react";
import {
  api,
  message,
  type Chapter,
  type Detail,
  type Project,
  type Setting,
} from "./api";
import type { StudioTab } from "./studioApi";
import {
  bookProgress,
  chapterHasText,
  parseRecentWriting,
} from "./bookProgress";
import {
  confirmWorkspaceNavigation,
  registerNavigationGuard,
} from "./navigationGuard";
import "./authorHome.css";
const WritingProfile = lazy(() => import("./WritingProfile"));
const Copilot = lazy(() => import("./Copilot"));

export type Brief = {
  plotPlan: string;
  genre: string;
  audience: string;
  targetWords: number;
  targetChapters: number;
  pov: string;
  platform: string;
  theme: string;
  sellingPoint: string;
  coreConflict: string;
  protagonistDesire: string;
  failureCost: string;
  endingDirection: string;
  worldRules: string[];
  relationships: string;
  restrictions: string;
  style: string;
  pace: string;
  volumes: {
    id: string;
    title: string;
    goal: string;
    summary: string;
    chapterCount: number;
  }[];
};
export const emptyBrief: Brief = {
  plotPlan: "",
  genre: "",
  audience: "",
  targetWords: 100000,
  targetChapters: 30,
  pov: "第三人称有限视角",
  platform: "",
  theme: "",
  sellingPoint: "",
  coreConflict: "",
  protagonistDesire: "",
  failureCost: "",
  endingDirection: "",
  worldRules: [],
  relationships: "",
  restrictions: "",
  style: "",
  pace: "",
  volumes: [],
};

export function LibraryHome({
  projects,
  onOpen,
  onCreate,
  onProvider,
  onImport,
  onResume,
  recentKey,
  loading,
  error,
}: {
  projects: Project[];
  onOpen: (p: Project) => void;
  onCreate: () => void;
  onProvider?: () => void;
  onImport: () => void;
  onResume: (projectId: string, chapterId: string) => void;
  recentKey: string;
  loading: boolean;
  error: unknown;
}) {
  const [search, setSearch] = useState("");
  let recent:
    { projectId: string; chapterId: string; chapterTitle: string } | undefined;
  try {
    recent = JSON.parse(localStorage.getItem(recentKey) || "null") || undefined;
  } catch {
    /* Ignore an outdated local preference. */
  }
  const recentProject = projects.find((p) => p.id === recent?.projectId);
  const filtered = projects.filter((p) =>
    (p.title + p.genre + p.description)
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  return (
    <main className="library-home">
      <section className="library-welcome">
        <div>
          <span className="eyebrow">A PLACE FOR YOUR STORIES</span>
          <h1>每个念头，都值得一本书。</h1>
          <p>先把世界和人物想清楚，再从容写下第一章。</p>
          <div className="row">
            <button className="button primary" onClick={onCreate}>
              <Plus size={17} /> 规划一本新书
            </button>
            <button className="button" onClick={onImport}>
              <FolderOpen size={16} /> 导入已有作品
            </button>
          </div>
        </div>
        <div className="library-art" aria-hidden="true">
          <BookOpen size={90} strokeWidth={0.8} />
          <Feather size={45} strokeWidth={1} />
          <span>构思 · 规划 · 写作</span>
        </div>
      </section>
      {recent && recentProject && (
        <section className="library-recent">
          <BookOpen size={21} />
          <div>
            <span>接着上次的思路</span>
            <strong>
              {recentProject.title} <small>· {recent.chapterTitle}</small>
            </strong>
          </div>
          <button
            className="button"
            onClick={() => onResume(recent!.projectId, recent!.chapterId)}
          >
            继续上次写作 <ArrowRight size={14} />
          </button>
        </section>
      )}
      <section className="library-books">
        <div className="row-between">
          <div>
            <span className="eyebrow">YOUR LIBRARY</span>
            <h2>
              我的书房 <small>{projects.length} 部作品</small>
            </h2>
          </div>
          <input
            aria-label="搜索书房作品"
            className="library-search"
            placeholder="找一本书…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        {error ? (
          <div className="error-box">{message(error)}</div>
        ) : loading ? (
          <p className="muted">
            <LoaderCircle className="spin" size={15} /> 正在整理书架…
          </p>
        ) : (
          <div className="book-grid">
            {filtered.map((p, index) => (
              <button
                className="book-card"
                key={p.id}
                onClick={() => onOpen(p)}
              >
                <span className={`book-cover cover-${index % 4}`}>
                  <span>{p.genre || "自由创作"}</span>
                  <strong>{p.title}</strong>
                  <Feather size={25} strokeWidth={1} />
                </span>
                <span className="book-card-info">
                  <strong>{p.title}</strong>
                  <small>
                    {p.description || "一个正在生长的故事，等你继续。"}
                  </small>
                  <span>
                    查看写作进度 <ArrowRight size={14} />
                  </span>
                </span>
              </button>
            ))}
            <button className="book-new" onClick={onCreate}>
              <Plus size={28} strokeWidth={1} />
              <strong>新的故事</strong>
              <small>从一个想法开始</small>
            </button>
          </div>
        )}
      </section>
      {onProvider && (
        <aside className="library-model">
          <Sparkles size={19} />
          <div>
            <strong>让 AI 成为你的创作伙伴</strong>
            <p>
              在开始之前连接自己的模型；之后可以一起规划、推敲人物和修改正文。
            </p>
          </div>
          <button className="button" onClick={onProvider}>
            <Settings2 size={14} /> 配置 AI 供应商
          </button>
        </aside>
      )}
    </main>
  );
}

export function NewBookWizard({
  onCreated,
}: {
  onCreated: (project: Project) => void;
}) {
  const [step, setStep] = useState(0),
    [title, setTitle] = useState(""),
    [idea, setIdea] = useState(""),
    [brief, setBrief] = useState<Brief>({ ...emptyBrief, genre: "悬疑" }),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [created, setCreated] = useState<Project>();
  const update = (key: keyof Brief, value: string | number) =>
    setBrief((b) => ({ ...b, [key]: value }));
  const save = async () => {
    setBusy(true);
    setError("");
    try {
      let p = created;
      if (!p) {
        p = await api<Project>("/projects", "POST", {
          title: title.trim(),
          description: idea,
          genre: brief.genre,
        });
        setCreated(p);
      }
      await api(`/projects/${p.id}/studio/brief`, "PUT", {
        expectedVersion: 0,
        data: brief,
      });
      onCreated(p);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form
      className="form-stack new-book-wizard"
      onSubmit={(e) => {
        e.preventDefault();
        if (step === 0) setStep(1);
        else void save();
      }}
    >
      <div className="wizard-progress">
        <span className={step === 0 ? "active" : "done"}>1 · 故事的种子</span>
        <i />
        <span className={step === 1 ? "active" : ""}>2 · 创作方向</span>
      </div>
      {step === 0 ? (
        <>
          <h3>你想讲一个怎样的故事？</h3>
          <p className="muted">
            不必一次想完整。留下一个念头，AI 会在下一步和你继续推敲。
          </p>
          <label>
            作品名
            <input
              autoFocus
              required
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="也可以先起一个暂定名"
              maxLength={200}
            />
          </label>
          <label>
            故事的第一个想法
            <textarea
              rows={5}
              value={idea}
              onChange={(e) => setIdea(e.target.value)}
              placeholder="例如：一位失去记忆的制印师，发现自己的作品出现在十年前的案卷里…"
            />
          </label>
          <div className="author-form-grid">
            <label>
              题材
              <input
                value={brief.genre}
                onChange={(e) => update("genre", e.target.value)}
                placeholder="悬疑、幻想、现实…"
              />
            </label>
            <label>
              希望写给谁看
              <input
                value={brief.audience}
                onChange={(e) => update("audience", e.target.value)}
                placeholder="例如：喜欢人物推理的成年读者"
              />
            </label>
          </div>
        </>
      ) : (
        <>
          <h3>给这个故事一点方向</h3>
          <p className="muted">
            这些都可以继续修改。保存后进入全书规划，无需先创建章节。
          </p>
          <label>
            核心冲突
            <textarea
              rows={3}
              value={brief.coreConflict}
              onChange={(e) => update("coreConflict", e.target.value)}
              placeholder="主角最想得到什么，什么在阻止他？"
            />
          </label>
          <label>
            故事主题
            <input
              value={brief.theme}
              onChange={(e) => update("theme", e.target.value)}
              placeholder="例如：记忆是否定义一个人"
            />
          </label>
          <div className="author-form-grid">
            <label>
              目标字数
              <input
                type="number"
                min={1000}
                max={10000000}
                value={brief.targetWords}
                onChange={(e) => update("targetWords", Number(e.target.value))}
              />
            </label>
            <label>
              预计章节
              <input
                type="number"
                min={1}
                max={10000}
                value={brief.targetChapters}
                onChange={(e) =>
                  update("targetChapters", Number(e.target.value))
                }
              />
            </label>
          </div>
          <label>
            想要的文风
            <input
              value={brief.style}
              onChange={(e) => update("style", e.target.value)}
              placeholder="冷静克制、轻快幽默、诗意细腻…"
            />
          </label>
        </>
      )}
      {error && <div className="error-box">{error}</div>}
      <div className="row-between">
        {step === 1 ? (
          <button
            className="button"
            type="button"
            onClick={() => setStep(0)}
            disabled={busy}
          >
            <ArrowLeft size={14} /> 上一步
          </button>
        ) : (
          <small className="muted">可以边聊边完善，不需要填满。</small>
        )}
        <button
          className="button primary"
          type="submit"
          disabled={busy || !title.trim()}
        >
          {busy ? (
            <LoaderCircle size={15} className="spin" />
          ) : step === 0 ? (
            <ArrowRight size={15} />
          ) : (
            <Sparkles size={15} />
          )}{" "}
          {step === 0 ? "下一步 · 创作方向" : "保存并进入全书规划"}
        </button>
      </div>
    </form>
  );
}

export function BookHome({
  detail,
  onWrite,
  onLibrary,
  onStudio,
  onRefresh,
  notify,
  config,
  styleRequest,
  recentKey,
}: {
  styleRequest?: number;
  recentKey?: string;
  detail: Detail;
  onWrite: (
    id?: string,
    section?: "memory" | "runs",
    navigationApproved?: boolean,
  ) => void;
  onLibrary: () => void;
  onStudio: (tab: StudioTab) => void;
  onRefresh: () => void;
  notify: (text: string) => void;
  config?: { model: string; externalModel: boolean };
}) {
  const qc = useQueryClient(),
    pid = detail.project.id;
  const copilotPane = useRef<HTMLElement>(null);
  const discussionOpener = useRef<HTMLButtonElement>(null);
  const [discussionOpen, setDiscussionOpen] = useState(false);
  const [discussionVisited, setDiscussionVisited] = useState(false);
  const [discussionRequest, setDiscussionRequest] = useState(0);
  const [recent] = useState(() => {
    try {
      if (!recentKey) return undefined;
      const scoped = parseRecentWriting(
        localStorage.getItem(`${recentKey}:${pid}`),
      );
      return scoped?.projectId === pid
        ? scoped
        : parseRecentWriting(localStorage.getItem(recentKey));
    } catch {
      return undefined;
    }
  });
  const query = useQuery({
    queryKey: ["book-brief", pid],
    queryFn: () =>
      api<{ brief: { version: number; data: Brief } }>(
        `/projects/${pid}/studio`,
      ),
  });
  const access = useQuery({
    queryKey: ["studio", pid, "access"],
    queryFn: () => api<{ role: string }>(`/projects/${pid}/access`),
  });
  const [profileVisited, setProfileVisited] = useState(false);
  const [tab, setTab] = useState<
      "overview" | "plan" | "characters" | "world" | "chapters" | "style"
    >("overview"),
    [editing, setEditing] = useState(false),
    [edit, setEdit] = useState<Brief>({ ...emptyBrief }),
    [editVersion, setEditVersion] = useState(0),
    [editBase, setEditBase] = useState<Brief>({ ...emptyBrief }),
    [busy, setBusy] = useState(false),
    [chapterTitle, setChapterTitle] = useState(""),
    [volume, setVolume] = useState("第一卷"),
    [setting, setSetting] = useState<{
      id?: string;
      kind?: string;
      name: string;
      content: string;
    }>({ name: "", content: "" }),
    [settingOpen, setSettingOpen] = useState(false),
    [settingBase, setSettingBase] = useState({ name: "", content: "" }),
    [planError, setPlanError] = useState("");
  useEffect(() => {
    if (styleRequest) setTab("style");
  }, [styleRequest]);
  useEffect(() => {
    if (tab === "style") setProfileVisited(true);
  }, [tab]);
  const brief = query.data?.brief.data || emptyBrief,
    version = query.data?.brief.version || 0,
    progress = bookProgress(pid, detail.chapters, recent),
    chapters = progress.chapters,
    canWrite = !!access.data && ["owner", "editor"].includes(access.data.role);
  const memoryNeedsAttention = ["MemoryPending", "RebuildRequired"].includes(
    detail.project.health,
  );
  const memoryAttention = useQuery({
    queryKey: [
      "book-memory-attention",
      pid,
      detail.project.health,
      detail.project.epoch,
      detail.project.stateVersionId,
    ],
    queryFn: async () => {
      if (detail.project.health === "RebuildRequired")
        return api<{ projectId: string; epoch: number; chapterIds: string[] }>(
          `/projects/${pid}/rebuild`,
        );
      const state = await api<{
        projectId: string;
        epoch: number;
        pendingChapterIds: string[];
      }>(`/projects/${pid}/state`);
      return {
        projectId: state.projectId,
        epoch: state.epoch,
        chapterIds: state.pendingChapterIds,
      };
    },
    enabled: memoryNeedsAttention && tab === "overview",
  });
  const memoryTarget =
    memoryAttention.data?.projectId === pid &&
    memoryAttention.data.epoch === detail.project.epoch
      ? memoryAttention.data.chapterIds
          .map((chapterId) =>
            chapters.find((chapter) => chapter.id === chapterId),
          )
          .find((chapter) => chapter?.finalRevisionId)
      : undefined;
  const unsaved = useRef(false);
  const planDirty =
    editing && JSON.stringify(edit) !== JSON.stringify(editBase);
  const settingDirty =
    settingOpen &&
    (setting.name !== settingBase.name ||
      setting.content !== settingBase.content);
  const chapterDirty = !!chapterTitle.trim();
  unsaved.current = planDirty || settingDirty || chapterDirty;
  useEffect(
    () =>
      registerNavigationGuard(
        () =>
          !unsaved.current ||
          window.confirm(
            "这本书还有未保存的设定、规划或章节表单，离开后会放弃这些修改。仍要离开吗？",
          ),
      ),
    [],
  );
  useEffect(() => {
    const leave = (event: BeforeUnloadEvent) => {
      if (!unsaved.current) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", leave);
    return () => window.removeEventListener("beforeunload", leave);
  }, []);
  const selectTab = (next: typeof tab) => {
    if (next === tab) return;
    if (
      settingDirty &&
      !window.confirm(
        "这条设定还有未保存的修改。切换页面会放弃这些修改，仍要切换吗？",
      )
    )
      return;
    setTab(next);
    setSettingOpen(false);
  };
  const discuss = () => {
    setDiscussionVisited(true);
    setDiscussionOpen(true);
    setDiscussionRequest((value) => value + 1);
  };
  useEffect(() => {
    if (!discussionOpen) return;
    const focus = () => {
      const input = copilotPane.current?.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="给创作伙伴的消息"]',
      );
      if (!input) return false;
      input.focus({ preventScroll: true });
      input.scrollIntoView({ block: "nearest", behavior: "auto" });
      return true;
    };
    if (focus()) return;
    const observer = new MutationObserver(() => {
      if (focus()) observer.disconnect();
    });
    if (copilotPane.current)
      observer.observe(copilotPane.current, { childList: true, subtree: true });
    const timeout = window.setTimeout(() => observer.disconnect(), 5000);
    return () => {
      observer.disconnect();
      window.clearTimeout(timeout);
    };
  }, [discussionOpen, discussionRequest]);
  const continueWriting = () =>
    progress.resume ? onWrite(progress.resume.id) : selectTab("chapters");
  const attention: {
    id: string;
    title: string;
    description: string;
    action: () => void;
    disabled?: boolean;
  }[] = [
    ...(memoryNeedsAttention
      ? [
          {
            id: "memory",
            title:
              detail.project.health === "RebuildRequired"
                ? "故事记忆需要复核"
                : "有已定稿章节的记忆待确认",
            description: memoryTarget
              ? `从第 ${memoryTarget.order} 章「${memoryTarget.title}」开始，按章顺序核对记忆。`
              : memoryAttention.isPending
                ? "正在定位待处理的定稿章节…"
                : memoryAttention.isError
                  ? "暂时无法读取待处理章节，点击重试。"
                  : "记忆清单已变化，点击刷新后再定位章节。",
            disabled: memoryAttention.isPending,
            action: () => {
              if (memoryTarget) {
                onWrite(memoryTarget.id, "memory");
                return;
              }
              void memoryAttention.refetch();
              onRefresh();
            },
          },
        ]
      : []),
    ...progress.review.map((chapter) => ({
      id: chapter.id,
      title: chapter.title,
      description: "前文变更后，本章被标记为待复核。",
      action: () => onWrite(chapter.id),
    })),
    ...(!chapters.length &&
    query.data &&
    !brief.coreConflict.trim() &&
    !brief.plotPlan.trim()
      ? [
          {
            id: "plan",
            title: "故事方向还没有确定",
            description: "可以先写下主角想要什么，以及挡在面前的冲突。",
            action: () => selectTab("plan"),
          },
        ]
      : []),
  ].slice(0, 3);
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["book-brief", pid] });
    void qc.invalidateQueries({ queryKey: ["studio", pid] });
    onRefresh();
  };
  const action = async (fn: () => Promise<void>) => {
    setBusy(true);
    setPlanError("");
    try {
      await fn();
      refresh();
    } catch (e) {
      setPlanError(message(e));
      notify(message(e));
    } finally {
      setBusy(false);
    }
  };
  const artificialChapter: Chapter = {
    id: "",
    projectId: pid,
    title: "全书设定与规划",
    volume: "",
    order: 0,
    blueprint: brief.plotPlan || "",
    blueprintVersion: version,
    currentRevisionId: "",
    finalRevisionId: "",
    needsReview: false,
    deleted: false,
  };
  const settingsTab = ["plan", "characters", "world", "style"].includes(tab);
  const sections = [
    { id: "overview", label: "作品概览", Icon: BookOpen },
    { id: "plan", label: "设定与规划", Icon: Waypoints },
    { id: "chapters", label: "卷章目录", Icon: FilePlus2 },
  ] as const;
  const settingSections = [
    { id: "plan", label: "全书规划" },
    { id: "characters", label: "人物" },
    { id: "world", label: "世界与规则" },
    { id: "style", label: "文风与写法" },
  ] as const;
  const settingKind = tab === "characters" ? "character" : "world";
  return (
    <main className={"book-home" + (discussionOpen ? " discussion-open" : "")}>
      <div className="book-breadcrumb">
        <button className="text-button" onClick={onLibrary}>
          <ArrowLeft size={14} /> 我的书房
        </button>
        <span>/</span>
        <span>{detail.project.title}</span>
      </div>
      <header className="book-heading">
        <div>
          <span className="eyebrow">
            {detail.project.genre || "自由创作"} · YOUR STORY
          </span>
          <h1>{detail.project.title}</h1>
          <p>
            {detail.project.description ||
              "把想法变成一个有方向、有血肉的故事。"}
          </p>
        </div>
        <div className="row">
          <button
            className={
              "button book-discuss" + (discussionOpen ? " active" : "")
            }
            ref={discussionOpener}
            aria-expanded={discussionOpen}
            aria-controls="book-discussion"
            onClick={discuss}
          >
            <Sparkles size={15} /> 和 AI 讨论
          </button>
          {tab !== "overview" && (
            <button className="button primary" onClick={continueWriting}>
              <Feather size={15} />{" "}
              {chapters.length ? "继续写作" : "准备第一章"}
            </button>
          )}
        </div>
      </header>
      <nav className="book-tabs" aria-label="作品导航">
        {sections.map(({ id, label, Icon }) => (
          <button
            className={
              (id === "plan" ? settingsTab : tab === id) ? "active" : ""
            }
            aria-current={
              (id === "plan" ? settingsTab : tab === id) ? "page" : undefined
            }
            key={id}
            onClick={() => selectTab(id)}
          >
            <Icon size={15} />
            {label}
          </button>
        ))}
      </nav>
      {settingsTab && (
        <nav className="book-setting-tabs" aria-label="设定与规划分类">
          {settingSections.map(({ id, label }) => (
            <button
              key={id}
              aria-current={tab === id ? "page" : undefined}
              className={tab === id ? "active" : ""}
              onClick={() => selectTab(id)}
            >
              {label}
            </button>
          ))}
        </nav>
      )}
      <div
        className={
          "book-home-layout" + (discussionOpen ? " discussion-open" : "")
        }
      >
        <section className="book-main">
          {(tab === "style" || profileVisited) && (
            <div hidden={tab !== "style"}>
              <Suspense fallback={<p className="muted">正在打开文风与写法…</p>}>
                <WritingProfile detail={detail} notify={notify} />
              </Suspense>
            </div>
          )}
          {tab === "overview" && (
            <div className="book-overview">
              <section className="book-writing-progress" aria-label="写作进度">
                <span className="step-badge">写作进度</span>
                <p>
                  <strong>{progress.written.length}</strong> / {chapters.length}{" "}
                  章已保存正文 <span>·</span>{" "}
                  <strong>{progress.final.length}</strong> 章当前稿已定稿{" "}
                  <span>·</span> <strong>{progress.review.length}</strong>{" "}
                  章待复核
                </p>
              </section>
              <section className="book-next" aria-label="下一步写作">
                <span className="step-badge">
                  {progress.resumeIsRecent ? "接着上次的思路" : "下一步"}
                </span>
                {progress.resume ? (
                  <>
                    <h2>{progress.resume.title}</h2>
                    <p>
                      {progress.resume.blueprint.trim()
                        ? progress.resume.blueprint.trim().slice(0, 140) +
                          (progress.resume.blueprint.trim().length > 140
                            ? "…"
                            : "")
                        : chapterHasText(progress.resume)
                          ? "正文已经保存在这里。接着写，或选一段和创作伙伴一起推敲。"
                          : "这一章已经准备好。写下开头，或和创作伙伴一起梳理本章要发生的事。"}
                    </p>
                    <small className="book-resume-location">
                      {progress.resume.volume || "正文"} · 第{" "}
                      {progress.resume.order} 章
                      {progress.resume.needsReview ? " · 待复核" : ""}
                    </small>
                  </>
                ) : (
                  <>
                    <h2>为故事准备第一章。</h2>
                    <p>
                      {brief.coreConflict || brief.plotPlan
                        ? "故事已经有了方向。给第一章起个名字，就能开始写作，也可以随时回来调整设定。"
                        : "从一个想写的场景开始。人物、世界和全书方向，都可以在“设定与规划”里慢慢完善。"}
                    </p>
                  </>
                )}
                <button className="button primary" onClick={continueWriting}>
                  <Feather size={15} />
                  {chapters.length ? "继续写作" : "准备第一章"}
                  <ArrowRight size={15} />
                </button>
              </section>
              <section className="book-attention" aria-label="待处理事项">
                <h3>待处理</h3>
                {attention.length ? (
                  <div className="book-attention-list">
                    {attention.map((item) => (
                      <button
                        key={item.id}
                        onClick={item.action}
                        disabled={item.disabled}
                      >
                        <AlertCircle size={17} />
                        <span>
                          <strong>{item.title}</strong>
                          <small>{item.description}</small>
                        </span>
                        <ArrowRight size={15} />
                      </button>
                    ))}
                  </div>
                ) : (
                  <p className="book-attention-clear">
                    <Check size={15} />
                    目前没有待复核标记。你可以按自己的节奏继续写作。
                  </p>
                )}
                {progress.review.length > 3 && (
                  <button
                    className="text-button"
                    onClick={() => selectTab("chapters")}
                  >
                    查看卷章目录中的全部标记 <ArrowRight size={13} />
                  </button>
                )}
              </section>
            </div>
          )}
          {tab === "plan" && (
            <>
              <div className="row-between">
                <div>
                  <span className="eyebrow">STORY FOUNDATION</span>
                  <h2>全书规划</h2>
                </div>
                <button
                  className="button"
                  disabled={!canWrite || busy || !query.data}
                  onClick={() => {
                    if (editing) {
                      if (
                        planDirty &&
                        !window.confirm(
                          "全书规划还有未保存的修改。收起编辑会放弃这些修改，仍要收起吗？",
                        )
                      )
                        return;
                      setEditing(false);
                    } else {
                      const initial = { ...emptyBrief, ...brief };
                      setEdit(initial);
                      setEditBase(initial);
                      setEditVersion(version);
                      setEditing(true);
                    }
                  }}
                >
                  {editing ? "收起编辑" : "编辑创作方向"}
                </button>
              </div>
              {query.isError && (
                <div className="error-box">{message(query.error)}</div>
              )}
              {editing ? (
                <form
                  className="form-stack book-plan-form"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void action(async () => {
                      await api(`/projects/${pid}/studio/brief`, "PUT", {
                        expectedVersion: editVersion,
                        data: edit,
                      });
                      setEditing(false);
                      notify("全书创作方向已保存。");
                    });
                  }}
                >
                  {(
                    [
                      ["theme", "故事主题"],
                      ["coreConflict", "核心冲突"],
                      ["protagonistDesire", "主角的欲望"],
                      ["failureCost", "失败的代价"],
                      ["endingDirection", "结局方向"],
                      ["audience", "目标读者"],
                      ["style", "文风"],
                    ] as const
                  ).map(([key, label]) => (
                    <label key={key}>
                      {label}
                      <textarea
                        rows={2}
                        value={edit[key]}
                        onChange={(e) =>
                          setEdit((b) => ({ ...b, [key]: e.target.value }))
                        }
                      />
                    </label>
                  ))}
                  <div className="author-form-grid">
                    <label>
                      目标字数
                      <input
                        type="number"
                        value={edit.targetWords}
                        min={1000}
                        onChange={(e) =>
                          setEdit((b) => ({
                            ...b,
                            targetWords: Number(e.target.value),
                          }))
                        }
                      />
                    </label>
                    <label>
                      预计章节
                      <input
                        type="number"
                        value={edit.targetChapters}
                        min={1}
                        onChange={(e) =>
                          setEdit((b) => ({
                            ...b,
                            targetChapters: Number(e.target.value),
                          }))
                        }
                      />
                    </label>
                  </div>
                  {editVersion !== version && (
                    <p className="warning-box">
                      全书规划刚刚有了更新。请收起后重新编辑，保留最新采用的方案。
                    </p>
                  )}
                  <button
                    className="button primary"
                    disabled={busy || editVersion !== version}
                  >
                    <Save size={14} /> 保存创作方向
                  </button>
                </form>
              ) : (
                <div className="book-foundation">
                  {[
                    ["核心冲突", brief.coreConflict],
                    ["主角的欲望", brief.protagonistDesire],
                    ["失败的代价", brief.failureCost],
                    [
                      "主题与结局",
                      [brief.theme, brief.endingDirection]
                        .filter(Boolean)
                        .join(" · "),
                    ],
                    [
                      "读者与文风",
                      [brief.audience, brief.style].filter(Boolean).join(" · "),
                    ],
                  ].map(([label, value]) => (
                    <div key={label}>
                      <h3>{label}</h3>
                      <p className={value ? "" : "muted"}>
                        {value || "还没有确定，可以和创作伙伴一起推敲。"}
                      </p>
                    </div>
                  ))}
                </div>
              )}
              <article className="book-plot-plan">
                <span className="eyebrow">YOUR ACCEPTED PLAN</span>
                <h3>已采用的故事方案</h3>
                {brief.plotPlan ? (
                  <div className="book-plan-text">{brief.plotPlan}</div>
                ) : (
                  <p className="muted">
                    右侧讨论产生的规划建议会先成为候选。点击“采用全书方案”后，才会保存在这里。
                  </p>
                )}
              </article>
              <button
                className="text-button"
                onClick={() => onStudio("planning")}
              >
                进一步安排卷目标与创作约束 <ArrowRight size={14} />
              </button>
            </>
          )}
          {(tab === "characters" || tab === "world") && (
            <>
              <div className="row-between">
                <div>
                  <span className="eyebrow">
                    {tab === "characters"
                      ? "PEOPLE WITH DESIRE"
                      : "RULES OF YOUR WORLD"}
                  </span>
                  <h2>
                    {tab === "characters" ? "认识你的人物" : "搭建故事的世界"}
                  </h2>
                </div>
                <button
                  className="button"
                  disabled={!canWrite}
                  onClick={() => {
                    if (
                      settingDirty &&
                      !window.confirm(
                        "这条设定还有未保存的修改，仍要重新添加吗？",
                      )
                    )
                      return;
                    setSetting({ name: "", content: "" });
                    setSettingBase({ name: "", content: "" });
                    setSettingOpen(true);
                  }}
                >
                  <Plus size={14} />{" "}
                  {tab === "characters" ? "添加人物" : "添加世界设定"}
                </button>
              </div>
              <p className="muted">
                {tab === "characters"
                  ? "他们想要什么，害怕什么，又会为谁改变？"
                  : "保存会影响故事的规则、地点、时代和限制。AI 会在写作时按需查阅。"}
              </p>
              {settingOpen && (
                <form
                  className="form-stack setting-inline"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void action(async () => {
                      await api(`/projects/${pid}/settings`, "POST", {
                        ...setting,
                        kind: setting.kind || settingKind,
                      });
                      setSettingOpen(false);
                      notify("设定已保存。");
                    });
                  }}
                >
                  <label>
                    名称
                    <input
                      required
                      value={setting.name}
                      onChange={(e) =>
                        setSetting((s) => ({ ...s, name: e.target.value }))
                      }
                    />
                  </label>
                  <label>
                    {tab === "characters"
                      ? "人物的欲望、性格与关系"
                      : "这个世界如何运作"}
                    <textarea
                      rows={6}
                      required
                      value={setting.content}
                      onChange={(e) =>
                        setSetting((s) => ({ ...s, content: e.target.value }))
                      }
                    />
                  </label>
                  <div className="row">
                    <button className="button primary" disabled={busy}>
                      <Save size={14} /> 保存设定
                    </button>
                    <button
                      className="text-button"
                      type="button"
                      onClick={() => {
                        if (
                          !settingDirty ||
                          window.confirm("放弃这条设定尚未保存的修改吗？")
                        )
                          setSettingOpen(false);
                      }}
                    >
                      取消
                    </button>
                  </div>
                </form>
              )}
              <div className="book-settings-grid">
                {detail.settings
                  .filter((s) =>
                    tab === "characters"
                      ? s.kind === "character"
                      : ["world", "rule"].includes(s.kind),
                  )
                  .map((s) => (
                    <article key={s.id}>
                      <div className="row-between">
                        <h3>{s.name}</h3>
                        <button
                          className="text-button"
                          disabled={!canWrite}
                          onClick={() => {
                            if (
                              settingDirty &&
                              !window.confirm(
                                "这条设定还有未保存的修改，仍要编辑另一条吗？",
                              )
                            )
                              return;
                            setSetting(s);
                            setSettingBase({
                              name: s.name,
                              content: s.content,
                            });
                            setSettingOpen(true);
                          }}
                        >
                          编辑
                        </button>
                      </div>
                      <p>{s.content}</p>
                    </article>
                  ))}
              </div>
              {!detail.settings.some((s) =>
                tab === "characters"
                  ? s.kind === "character"
                  : ["world", "rule"].includes(s.kind),
              ) &&
                !settingOpen && (
                  <div className="book-inline-empty">
                    <Users size={32} strokeWidth={1} />
                    <p>
                      {tab === "characters"
                        ? "先添加一个让你想了解的人。"
                        : "先确定一条这个世界不能违背的规则。"}
                    </p>
                  </div>
                )}
            </>
          )}
          {tab === "chapters" && (
            <>
              <span className="eyebrow">ONE CHAPTER AT A TIME</span>
              <h2>卷章目录</h2>
              <p className="muted">
                每章一个明确目标。规划可以随创作调整，正文版本始终保留。
              </p>
              <div className="book-chapters">
                {chapters.map((c) => (
                  <button key={c.id} onClick={() => onWrite(c.id)}>
                    <span>{String(c.order).padStart(2, "0")}</span>
                    <div>
                      <small>{c.volume || "正文"}</small>
                      <strong>{c.title}</strong>
                    </div>
                    <small>
                      {c.needsReview
                        ? "待复核"
                        : c.currentRevisionId &&
                            c.finalRevisionId === c.currentRevisionId
                          ? "已定稿"
                          : chapterHasText(c)
                            ? "创作中"
                            : "待起笔"}
                    </small>
                    <ArrowRight size={16} />
                  </button>
                ))}
              </div>
              {canWrite && (
                <form
                  className="form-stack book-new-chapter"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (planDirty) {
                      setTab("plan");
                      notify(
                        "全书规划还有未保存的修改。请先保存或收起编辑，再创建章节；章节标题已保留。",
                      );
                      return;
                    }
                    const ownUnsaved = unsaved.current;
                    let navigationApproved = false;
                    try {
                      // This form is being submitted; other mounted drafts must be checked before creating a chapter.
                      unsaved.current = planDirty || settingDirty;
                      navigationApproved = confirmWorkspaceNavigation();
                    } finally {
                      unsaved.current = ownUnsaved;
                    }
                    if (!navigationApproved) return;
                    void action(async () => {
                      const c = await api<Chapter>(
                        `/projects/${pid}/chapters`,
                        "POST",
                        { title: chapterTitle || "第一章", volume },
                      );
                      await qc.fetchQuery({
                        queryKey: ["project", pid],
                        queryFn: () => api<Detail>(`/projects/${pid}`),
                        staleTime: 0,
                      });
                      setChapterTitle("");
                      unsaved.current = planDirty || settingDirty;
                      onWrite(c.id, undefined, true);
                    });
                  }}
                >
                  <h3>{chapters.length ? "添加下一章" : "准备你的第一章"}</h3>
                  <div className="author-form-grid">
                    <label>
                      所属卷
                      <input
                        value={volume}
                        onChange={(e) => setVolume(e.target.value)}
                      />
                    </label>
                    <label>
                      章节标题
                      <input
                        required
                        value={chapterTitle}
                        onChange={(e) => setChapterTitle(e.target.value)}
                        placeholder="例如：第一章 雨夜的来客"
                      />
                    </label>
                  </div>
                  <button className="button primary" disabled={busy}>
                    <Feather size={15} /> 创建并进入写作
                  </button>
                </form>
              )}
            </>
          )}
          {planError && <div className="error-box">{planError}</div>}
        </section>
        {(discussionOpen || discussionVisited) && (
          <aside
            className="book-copilot"
            id="book-discussion"
            aria-label="全书创作伙伴"
            ref={copilotPane}
            hidden={!discussionOpen}
          >
            <header>
              <Sparkles size={16} />
              <div>
                <strong>全书创作伙伴</strong>
                <span>先讨论方向，再采用方案</span>
              </div>
              <button
                className="icon-button"
                aria-label="收起全书创作伙伴"
                onClick={() => {
                  setDiscussionOpen(false);
                  discussionOpener.current?.focus();
                }}
              >
                <X size={16} />
              </button>
            </header>
            <Suspense fallback={<p className="muted">正在打开创作伙伴…</p>}>
              <Copilot
                key={pid + ":book"}
                scope="book"
                briefVersion={version}
                chapter={artificialChapter}
                detail={detail}
                currentRevisionId=""
                currentText={brief.plotPlan || ""}
                selection=""
                config={config}
                flush={async () => ""}
                onApply={async () => {}}
                onSaved={refresh}
                notify={notify}
              />
            </Suspense>
          </aside>
        )}
      </div>
    </main>
  );
}
