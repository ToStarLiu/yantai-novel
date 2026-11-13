import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Check,
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
import "./authorHome.css";
const WritingProfile = lazy(() => import("./WritingProfile"));
const Copilot = lazy(() => import("./Copilot"));
const WritingSkills = lazy(() => import("./WritingSkills"));

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
                    打开作品规划 <ArrowRight size={14} />
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
}: {
  styleRequest?: number;
  detail: Detail;
  onWrite: (id?: string) => void;
  onLibrary: () => void;
  onStudio: (tab: StudioTab) => void;
  onRefresh: () => void;
  notify: (text: string) => void;
  config?: { model: string; externalModel: boolean };
}) {
  const qc = useQueryClient(),
    pid = detail.project.id;
  const copilotPane = useRef<HTMLElement>(null);
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
      | "overview"
      | "plan"
      | "characters"
      | "world"
      | "chapters"
      | "skills"
      | "style"
    >("overview"),
    [editing, setEditing] = useState(false),
    [edit, setEdit] = useState<Brief>({ ...emptyBrief }),
    [editVersion, setEditVersion] = useState(0),
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
    [planError, setPlanError] = useState("");
  useEffect(() => {
    if (styleRequest) setTab("style");
  }, [styleRequest]);
  useEffect(() => {
    if (tab === "style") setProfileVisited(true);
  }, [tab]);
  const brief = query.data?.brief.data || emptyBrief,
    version = query.data?.brief.version || 0,
    chapters = detail.chapters.filter((c) => !c.deleted),
    canWrite = !!access.data && ["owner", "editor"].includes(access.data.role);
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
  const sections = [
    { id: "overview", label: "作品概览", Icon: BookOpen },
    { id: "plan", label: "全书规划", Icon: Waypoints },
    { id: "characters", label: "人物", Icon: Users },
    { id: "world", label: "世界与规则", Icon: Layers3 },
    { id: "chapters", label: "卷章目录", Icon: FilePlus2 },
    { id: "style", label: "文风与写法", Icon: Feather },
    { id: "skills", label: "写作 Skills", Icon: Sparkles },
  ] as const;
  const settingKind = tab === "characters" ? "character" : "world";
  return (
    <main className="book-home">
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
            className="button book-discuss"
            onClick={() => {
              copilotPane.current?.scrollIntoView({
                block: "start",
                behavior: "auto",
              });
              copilotPane.current
                ?.querySelector<HTMLTextAreaElement>("textarea")
                ?.focus({ preventScroll: true });
            }}
          >
            <Sparkles size={15} /> 和 AI 讨论
          </button>
          <button className="button" onClick={() => onStudio("canvas")}>
            <Layers3 size={14} /> 更多创作工具
          </button>
          <button
            className="button primary"
            onClick={() => (chapters.length ? onWrite() : setTab("chapters"))}
          >
            <Feather size={15} /> {chapters.length ? "继续写作" : "准备第一章"}
          </button>
        </div>
      </header>
      <nav className="book-tabs" aria-label="作品导航">
        {sections.map(({ id, label, Icon }) => (
          <button
            className={tab === id ? "active" : ""}
            key={id}
            onClick={() => {
              setTab(id);
              setSettingOpen(false);
            }}
          >
            <Icon size={15} />
            {label}
          </button>
        ))}
      </nav>
      <div className="book-home-layout">
        <section className="book-main">
          {(tab === "style" || profileVisited) && (
            <div hidden={tab !== "style"}>
              <Suspense fallback={<p className="muted">正在打开文风与写法…</p>}>
                <WritingProfile detail={detail} notify={notify} />
              </Suspense>
            </div>
          )}
          {tab === "skills" && (
            <Suspense fallback={<p className="muted">正在打开写作指南…</p>}>
              <WritingSkills projectId={pid} notify={notify} />
            </Suspense>
          )}
          {tab === "overview" && (
            <>
              <div className="book-next">
                <span className="step-badge">下一步</span>
                <h2>
                  {brief.plotPlan
                    ? "故事的方向已经有了，接下来让它生长。"
                    : "先找到这本书最打动人的核心。"}
                </h2>
                <p>
                  {brief.plotPlan
                    ? "完善人物与世界，再把计划拆成可写的章节。随时可以与右侧创作伙伴继续讨论。"
                    : "和创作伙伴讨论主角、冲突、结局与文风；喜欢的方案可以明确采用到全书规划。"}
                </p>
                <button
                  className="button primary"
                  onClick={() => setTab("plan")}
                >
                  <Sparkles size={15} />{" "}
                  {brief.plotPlan ? "查看并完善全书规划" : "开始全书规划"}
                </button>
              </div>
              <div className="book-stats">
                <div>
                  <strong>{chapters.length}</strong>
                  <span>章节</span>
                </div>
                <div>
                  <strong>
                    {
                      detail.settings.filter((s) => s.kind === "character")
                        .length
                    }
                  </strong>
                  <span>人物</span>
                </div>
                <div>
                  <strong>
                    {brief.targetWords
                      ? `${Math.round(brief.targetWords / 10000)}万`
                      : "未定"}
                  </strong>
                  <span>目标字数</span>
                </div>
                <div>
                  <strong>{brief.targetChapters || "未定"}</strong>
                  <span>计划章节</span>
                </div>
              </div>
              <div className="book-guidance">
                <h3>从想法到正文</h3>
                {[
                  {
                    label: "确定故事方向",
                    description: "主角的欲望、核心冲突、结局承诺",
                    done: !!brief.coreConflict,
                    click: () => setTab("plan"),
                  },
                  {
                    label: "认识人物与世界",
                    description: "动机、关系、故事发生的规则",
                    done: detail.settings.some((s) => s.kind === "character"),
                    click: () => setTab("characters"),
                  },
                  {
                    label: "安排卷章，开始写作",
                    description: "让每一章有目标，也有继续读下去的理由",
                    done: chapters.length > 0,
                    click: () => setTab("chapters"),
                  },
                ].map((item, i) => (
                  <button key={item.label} onClick={item.click}>
                    <span className={item.done ? "completed" : ""}>
                      {item.done ? <Check size={15} /> : i + 1}
                    </span>
                    <div>
                      <strong>{item.label}</strong>
                      <small>{item.description}</small>
                    </div>
                    <ArrowRight size={15} />
                  </button>
                ))}
              </div>
            </>
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
                    setEdit({ ...emptyBrief, ...brief });
                    setEditVersion(version);
                    setEditing(!editing);
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
                    setSetting({ name: "", content: "" });
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
                      onClick={() => setSettingOpen(false)}
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
                            setSetting(s);
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
                    <small>{c.finalRevisionId ? "已定稿" : "草稿"}</small>
                    <ArrowRight size={16} />
                  </button>
                ))}
              </div>
              {canWrite && (
                <form
                  className="form-stack book-new-chapter"
                  onSubmit={(e) => {
                    e.preventDefault();
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
                      onWrite(c.id);
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
        <aside className="book-copilot" ref={copilotPane}>
          <header>
            <Sparkles size={16} />
            <div>
              <strong>全书创作伙伴</strong>
              <span>先讨论方向，再采用方案</span>
            </div>
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
      </div>
    </main>
  );
}
