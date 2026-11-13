import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useState,
  useRef,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookOpen,
  Plus,
  ChevronDown,
  ChevronRight,
  Feather,
  Search,
  Settings,
  Download,
  Upload,
  LogOut,
  PanelRightClose,
  PanelRightOpen,
  History,
  Check,
  LoaderCircle,
  AlertCircle,
  CheckCheck,
  ArrowUp,
  ArrowDown,
  Trash2,
  X,
  Library,
  Leaf,
  Clock3,
  Save,
  PencilLine,
  FilePlus2,
  MoreHorizontal,
  BookMarked,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Maximize2,
  Minimize2,
  Users,
  Waypoints,
  NotebookPen,
  GripVertical,
} from "lucide-react";
import {
  api,
  setCSRF,
  type Session,
  type Project,
  type Detail,
  type Chapter,
  type Revision,
  type Doc,
  type Impact,
  documentFromText,
  dateTime,
  message,
  statusLabel,
  plainText,
} from "./api";
import { useDraft } from "./useDraft";
import { BookHome, LibraryHome, NewBookWizard } from "./AuthorHome";
import ProviderSettings from "./ProviderSettings";
import WorkspaceCommandPalette, {
  type WorkspaceCommand,
} from "./WorkspaceCommandPalette";
import type { StudioTab } from "./studioApi";
import "./workspaceLayout.css";
const LazyWritingSkills = lazy(() => import("./WritingSkills"));
const LazyEditor = lazy(() => import("./Editor"));
const LazyAssistant = lazy(() => import("./Assistant"));
const LazyStudio = lazy(() => import("./Studio"));
const Editor = (props: React.ComponentProps<typeof LazyEditor>) => (
  <Suspense fallback={<p className="muted small">正在打开正文编辑器…</p>}>
    <LazyEditor {...props} />
  </Suspense>
);
const Assistant = (props: React.ComponentProps<typeof LazyAssistant>) => (
  <Suspense
    fallback={
      <aside className="assistant-pane">
        <div className="mini-empty">正在打开创作伙伴…</div>
      </aside>
    }
  >
    <LazyAssistant {...props} />
  </Suspense>
);

type ModalState = {
  kind: "project" | "import" | "history" | "settings" | "impacts";
  project?: Project;
} | null;
type InstallPrompt = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: string }>;
};
function Button({
  children,
  onClick,
  disabled = false,
  primary = false,
  className = "",
  title,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  primary?: boolean;
  className?: string;
  title?: string;
}) {
  return (
    <button
      className={`${primary ? "button primary" : "button"} ${className}`}
      onClick={onClick}
      disabled={disabled}
      title={title}
    >
      {children}
    </button>
  );
}
export { Button };

function storedLayoutNumber(key: string, fallback: number) {
  try {
    const raw = localStorage.getItem(key);
    const value = raw === null ? NaN : Number(raw);
    return Number.isFinite(value) ? value : fallback;
  } catch {
    return fallback;
  }
}
const boundWidth = (value: number, min: number, max: number) =>
  Math.round(Math.min(Math.max(value, min), Math.max(min, max)));

function WorkspaceDivider({
  label,
  value,
  min,
  max,
  reverse = false,
  onChange,
  className = "",
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  reverse?: boolean;
  onChange: (value: number) => void;
  className?: string;
}) {
  const drag = useRef<{ x: number; value: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const direction = reverse ? -1 : 1;
  return (
    <div
      role="separator"
      tabIndex={0}
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={value}
      aria-valuetext={value + " 像素"}
      title={label + "：拖动或使用左右方向键"}
      className={
        "workspace-divider " + className + (dragging ? " dragging" : "")
      }
      onPointerDown={(e) => {
        if (e.button !== 0) return;
        e.preventDefault();
        drag.current = { x: e.clientX, value };
        e.currentTarget.setPointerCapture(e.pointerId);
        setDragging(true);
      }}
      onPointerMove={(e) => {
        if (drag.current)
          onChange(
            boundWidth(
              drag.current.value + (e.clientX - drag.current.x) * direction,
              min,
              max,
            ),
          );
      }}
      onPointerUp={(e) => {
        drag.current = null;
        setDragging(false);
        if (e.currentTarget.hasPointerCapture(e.pointerId))
          e.currentTarget.releasePointerCapture(e.pointerId);
      }}
      onPointerCancel={() => {
        drag.current = null;
        setDragging(false);
      }}
      onLostPointerCapture={() => {
        drag.current = null;
        setDragging(false);
      }}
      onKeyDown={(e) => {
        let next: number | undefined;
        if (e.key === "ArrowLeft") next = value - 16 * direction;
        if (e.key === "ArrowRight") next = value + 16 * direction;
        if (e.key === "Home") next = min;
        if (e.key === "End") next = max;
        if (next !== undefined) {
          e.preventDefault();
          onChange(boundWidth(next, min, max));
        }
      }}
    >
      <GripVertical size={11} />
    </div>
  );
}

export default function App() {
  const qc = useQueryClient();
  const [projectId, setProjectId] = useState(
    window.location.hash.split("/")[1] ||
      localStorage.getItem("novel-project") ||
      "",
  );
  const [chapterId, setChapterId] = useState("");
  const [modal, setModal] = useState<ModalState>(null);
  const [notice, setNotice] = useState("");
  const [assistantOpen, setAssistantOpen] = useState(
    () => window.innerWidth >= 1160,
  );
  const [mobileNav, setMobileNav] = useState(false);
  const [directoryOpen, setDirectoryOpen] = useState(true);
  const [chapterSearch, setChapterSearch] = useState("");
  const [collapsedVolumes, setCollapsedVolumes] = useState<Set<string>>(
    () => new Set(),
  );
  const [focusMode, setFocusMode] = useState(false);
  const [viewportWidth, setViewportWidth] = useState(window.innerWidth);
  const [preferredDirectoryWidth, setPreferredDirectoryWidth] = useState(() =>
    boundWidth(storedLayoutNumber("yantai-directory-width", 232), 190, 330),
  );
  const [preferredAssistantWidth, setPreferredAssistantWidth] = useState(() =>
    boundWidth(storedLayoutNumber("yantai-assistant-width", 360), 300, 540),
  );
  const desktopAssistant = viewportWidth >= 1160;
  const directoryMax = Math.max(
    190,
    Math.min(
      330,
      viewportWidth -
        64 -
        460 -
        10 -
        (assistantOpen && desktopAssistant ? preferredAssistantWidth : 0),
    ),
  );
  const directoryWidth = boundWidth(preferredDirectoryWidth, 190, directoryMax);
  const assistantMax = Math.max(
    300,
    Math.min(
      540,
      viewportWidth - 64 - (directoryOpen ? directoryWidth + 5 : 0) - 460 - 5,
    ),
  );
  const assistantWidth = boundWidth(preferredAssistantWidth, 300, assistantMax);
  useEffect(() => {
    const resize = () => setViewportWidth(window.innerWidth);
    window.addEventListener("resize", resize);
    return () => window.removeEventListener("resize", resize);
  }, []);
  useEffect(() => {
    try {
      localStorage.setItem(
        "yantai-directory-width",
        String(preferredDirectoryWidth),
      );
    } catch {
      /* Layout works without storage. */
    }
  }, [preferredDirectoryWidth]);
  useEffect(() => {
    try {
      localStorage.setItem(
        "yantai-assistant-width",
        String(preferredAssistantWidth),
      );
    } catch {
      /* Layout works without storage. */
    }
  }, [preferredAssistantWidth]);
  useEffect(() => {
    setChapterSearch("");
    setCollapsedVolumes(new Set());
  }, [projectId]);
  useEffect(() => {
    if (!focusMode && !mobileNav) return;
    const escape = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setFocusMode(false);
        setMobileNav(false);
      }
    };
    window.addEventListener("keydown", escape);
    return () => window.removeEventListener("keydown", escape);
  }, [focusMode, mobileNav]);
  const [nonce, setNonce] = useState(0);
  const [studioOpen, setStudioOpen] = useState(false);
  const [studioTab, setStudioTab] = useState<StudioTab>("canvas");
  const initialRoute = window.location.hash.split("/");
  const [surface, setSurface] = useState<"library" | "book" | "write">(
    initialRoute[0] === "#write"
      ? "write"
      : initialRoute[0] === "#book"
        ? "book"
        : "library",
  );
  const [providerOpen, setProviderOpen] = useState(false);
  const [skillsOpen, setSkillsOpen] = useState(false);
  const [commandsOpen, setCommandsOpen] = useState(false);
  const draftFlush = useRef<(() => Promise<string>) | null>(null);
  const registerFlush = useCallback((flush: (() => Promise<string>) | null) => {
    draftFlush.current = flush;
  }, []);
  const [installPrompt, setInstallPrompt] = useState<InstallPrompt | null>(
    null,
  );
  useEffect(() => {
    const remember = (event: Event) => {
      event.preventDefault();
      setInstallPrompt(event as InstallPrompt);
    };
    window.addEventListener("beforeinstallprompt", remember);
    return () => window.removeEventListener("beforeinstallprompt", remember);
  }, []);
  const session = useQuery({
    queryKey: ["session"],
    queryFn: () => api<Session>("/session"),
    retry: false,
  });
  useEffect(() => {
    if (session.data) setCSRF(session.data.csrfToken || "");
  }, [session.data]);
  useEffect(() => {
    if (!session.data?.authenticated || !projectId || !chapterId) return;
    const heartbeat = () =>
      void api(`/projects/${projectId}/presence`, "POST", { chapterId }).catch(
        () => {},
      );
    heartbeat();
    const timer = setInterval(heartbeat, 20000);
    const stream = new EventSource(`/api/projects/${projectId}/changes`);
    stream.addEventListener("change", () => {
      void qc.invalidateQueries({ queryKey: ["project", projectId] });
      void qc.invalidateQueries({ queryKey: ["chapter", chapterId] });
      void qc.invalidateQueries({ queryKey: ["studio", projectId] });
    });
    return () => {
      clearInterval(timer);
      stream.close();
    };
  }, [session.data?.authenticated, projectId, chapterId, qc]);
  const projects = useQuery({
    queryKey: ["projects"],
    queryFn: () => api<Project[]>("/projects"),
    enabled: !!session.data?.authenticated,
  });
  const detail = useQuery({
    queryKey: ["project", projectId],
    queryFn: () => api<Detail>("/projects/" + projectId),
    enabled: !!projectId && !!session.data?.authenticated,
  });
  const chapter = useQuery({
    queryKey: ["chapter", chapterId],
    queryFn: () => api<Chapter>("/chapters/" + chapterId),
    enabled: !!chapterId && !!session.data?.authenticated,
  });
  const recentKey = `novel-last-writing:${session.data?.user?.id || ""}`;
  useEffect(() => {
    const current = chapter.data;
    if (
      surface === "write" &&
      session.data?.user?.id &&
      current?.projectId === projectId
    )
      localStorage.setItem(
        recentKey,
        JSON.stringify({
          projectId,
          chapterId: current.id,
          chapterTitle: current.title,
        }),
      );
  }, [surface, projectId, chapter.data, recentKey, session.data?.user?.id]);
  const access = useQuery({
    queryKey: ["studio", projectId, "access"],
    queryFn: () => api<{ role: string }>(`/projects/${projectId}/access`),
    enabled: !!projectId && !!session.data?.authenticated,
  });
  const config = useQuery({
    queryKey: ["config"],
    queryFn: () =>
      api<{
        model: string;
        provider: string;
        externalModel: boolean;
        maxOutputTokens: number;
      }>("/config"),
    enabled: !!session.data?.authenticated,
  });
  useEffect(() => {
    if (projects.data?.length && !projectId) setProjectId(projects.data[0].id);
  }, [projects.data, projectId]);
  useEffect(() => {
    localStorage.setItem("novel-project", projectId);
    setChapterId(
      window.location.hash.split("/")[1] === projectId
        ? window.location.hash.split("/")[2] || ""
        : "",
    );
  }, [projectId]);
  useEffect(() => {
    if (detail.data) {
      const chapters = detail.data.chapters.filter((c) => !c.deleted);
      if (!chapters.some((c) => c.id === chapterId))
        setChapterId(chapters[0]?.id || "");
    }
  }, [detail.data, chapterId]);
  useEffect(() => {
    if (notice) {
      const timer = setTimeout(() => setNotice(""), 6500);
      return () => clearTimeout(timer);
    }
  }, [notice]);
  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["project", projectId] });
    void qc.invalidateQueries({ queryKey: ["chapter", chapterId] });
    void qc.invalidateQueries({ queryKey: ["projects"] });
  }, [qc, projectId, chapterId]);
  const notify = useCallback((text: string) => setNotice(text), []);
  const navigate = async (
    next: "library" | "book" | "write",
    pid = projectId,
    cid?: string,
  ) => {
    try {
      await draftFlush.current?.();
      if (pid !== projectId) setProjectId(pid);
      if (cid) setChapterId(cid);
      setSurface(next);
      setMobileNav(false);
      if (next !== "write") setFocusMode(false);
      window.history.replaceState(
        null,
        "",
        next === "library"
          ? window.location.pathname
          : `#${next}/${pid}${cid ? "/" + cid : ""}`,
      );
    } catch (e) {
      notify(message(e));
    }
  };
  const act = async (fn: () => Promise<unknown>, success?: string) => {
    try {
      await fn();
      refresh();
      if (success) notify(success);
    } catch (e) {
      notify(message(e));
    }
  };
  const createChapter = () => {
    const title = prompt("新章节标题", "未命名章节");
    if (title === null) return;
    void act(async () => {
      await draftFlush.current?.();
      const c = await api<Chapter>(`/projects/${projectId}/chapters`, "POST", {
        title: title || "未命名章节",
        volume: "第一卷",
      });
      setChapterId(c.id);
    }, "新章节已创建");
  };
  const moveChapter = (target: Chapter, direction: number) => {
    if (!detail.data) return;
    const chapters = detail.data.chapters
      .filter((c) => !c.deleted)
      .sort((a, b) => a.order - b.order);
    const i = chapters.findIndex((c) => c.id === target.id),
      j = i + direction;
    if (j < 0 || j >= chapters.length) return;
    [chapters[i], chapters[j]] = [chapters[j], chapters[i]];
    void act(
      () =>
        api(`/projects/${projectId}/chapter-order`, "POST", {
          expectedProjectEpoch: detail.data!.project.epoch,
          chapterIds: chapters.map((c) => c.id),
        }),
      "章节顺序已更新；相关记忆需复核",
    );
  };
  const openWorkspaceStudio = (tab: StudioTab) =>
    void act(async () => {
      await draftFlush.current?.();
      await qc.fetchQuery({
        queryKey: ["project", projectId],
        queryFn: () => api<Detail>("/projects/" + projectId),
        staleTime: 0,
      });
      setStudioTab(tab);
      setStudioOpen(true);
      setMobileNav(false);
    });
  const toggleDirectory = () => {
    if (viewportWidth <= 760) {
      setFocusMode(false);
      setMobileNav((v) => !v);
    } else setDirectoryOpen((v) => !v);
  };
  useEffect(() => {
    if (surface !== "write" || !session.data?.authenticated) return;
    const shortcuts = (event: KeyboardEvent) => {
      if (
        event.isComposing ||
        event.repeat ||
        !(event.ctrlKey || event.metaKey)
      )
        return;
      if (document.querySelector('[role="dialog"]') && !commandsOpen) return;
      if (event.key.toLowerCase() === "k" && !event.shiftKey) {
        event.preventDefault();
        setCommandsOpen(true);
      } else if (event.key.toLowerCase() === "s" && !event.shiftKey) {
        event.preventDefault();
        void draftFlush
          .current?.()
          .then(() => notify("正文已保存"))
          .catch((e) => notify(message(e)));
      } else if (event.key.toLowerCase() === "f" && event.shiftKey) {
        event.preventDefault();
        setFocusMode((value) => !value);
        setMobileNav(false);
      }
    };
    window.addEventListener("keydown", shortcuts);
    return () => window.removeEventListener("keydown", shortcuts);
  }, [surface, session.data?.authenticated, commandsOpen, notify]);
  if (session.isPending)
    return (
      <div className="app-loading">
        <Feather size={30} />
        <p>正在打开书房…</p>
      </div>
    );
  if (!session.data?.authenticated)
    return (
      <Login
        onLogin={(value) => {
          setCSRF(value.csrfToken);
          qc.setQueryData(["session"], value);
        }}
      />
    );
  const chapters =
    detail.data?.chapters
      .filter((c) => !c.deleted)
      .sort((a, b) => a.order - b.order) || [];
  const commandStudio = async (tab: StudioTab) => {
    await draftFlush.current?.();
    await qc.fetchQuery({
      queryKey: ["project", projectId],
      queryFn: () => api<Detail>("/projects/" + projectId),
      staleTime: 0,
    });
    setStudioTab(tab);
    setStudioOpen(true);
    setMobileNav(false);
  };
  const commands: WorkspaceCommand[] = [
    {
      id: "save",
      label: "保存当前正文",
      description: "先完成保存，再继续创作",
      group: "写作",
      shortcut: "Ctrl S",
      onSelect: async () => {
        await draftFlush.current?.();
        notify("正文已保存");
      },
    },
    {
      id: "focus",
      label: focusMode ? "退出专注写作" : "进入专注写作",
      description: "收起目录与助手，保留当前稿件",
      group: "写作",
      shortcut: "Ctrl Shift F",
      onSelect: () => {
        setFocusMode(!focusMode);
        setMobileNav(false);
      },
    },
    {
      id: "assistant",
      label: "打开创作伙伴",
      description: "讨论选段、章纲和修改候选",
      group: "写作",
      onSelect: () => {
        setFocusMode(false);
        setAssistantOpen(true);
      },
    },
    {
      id: "directory",
      label: "切换章节目录",
      description: "显示或收起目录",
      group: "写作",
      onSelect: toggleDirectory,
    },
    {
      id: "history",
      label: "查看正文历史",
      description: "比较和恢复此前保存的版本",
      group: "写作",
      onSelect: async () => {
        await draftFlush.current?.();
        setModal({ kind: "history" });
      },
    },
    {
      id: "book",
      label: "打开全书规划",
      description: "回到当前作品的规划与设定",
      group: "作品",
      onSelect: async () => {
        await draftFlush.current?.();
        await navigate("book");
      },
    },
    {
      id: "library",
      label: "返回我的书房",
      description: "查看全部作品",
      group: "作品",
      onSelect: async () => {
        await draftFlush.current?.();
        await navigate("library");
      },
    },
    {
      id: "characters",
      label: "人物与世界",
      description: "人物弧、关系和世界设定",
      group: "作品",
      onSelect: () => commandStudio("characters"),
    },
    {
      id: "canvas",
      label: "打开故事画布",
      description: "安排故事节点与因果",
      group: "作品",
      onSelect: () => commandStudio("canvas"),
    },
    {
      id: "skills",
      label: "管理写作 Skills",
      description: "调整本书写法与适用任务",
      group: "作品",
      onSelect: () => setSkillsOpen(true),
    },
    {
      id: "publish",
      label: "出版与导出",
      description: "选择稿件范围和导出格式",
      group: "作品",
      onSelect: () => commandStudio("publishing"),
    },
    {
      id: "export-package",
      label: "导出完整作品包",
      description: "保存全部正文版本、设定与写作指南为JSON包",
      group: "作品",
      onSelect: async () => {
        await draftFlush.current?.();
        window.location.href = `/api/projects/${projectId}/export`;
      },
    },
    ...chapters.map((c) => ({
      id: "chapter-" + c.id,
      label: c.title,
      description: c.volume || "正文",
      group: "章节",
      onSelect: async () => {
        await draftFlush.current?.();
        await navigate("write", projectId, c.id);
      },
    })),
  ];
  return (
    <div
      className={
        "app " +
        (surface !== "write" ? "author-home-app " : "workspace-app ") +
        (assistantOpen ? "" : "assistant-hidden ") +
        (!directoryOpen ? "directory-collapsed " : "") +
        (focusMode ? "focus-mode " : "") +
        (!desktopAssistant && assistantOpen ? "assistant-overlay " : "")
      }
      style={
        surface === "write"
          ? ({
              "--directory-width": directoryWidth + "px",
              "--assistant-width": assistantWidth + "px",
            } as React.CSSProperties)
          : undefined
      }
    >
      <header className="topbar">
        {surface === "write" && (
          <button
            className="icon-button mobile-only"
            aria-label="打开章节导航"
            aria-expanded={mobileNav}
            onClick={toggleDirectory}
          >
            <Menu size={20} />
          </button>
        )}
        <div
          className="brand"
          onClick={() => void navigate("library")}
          role="button"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === "Enter") void navigate("library");
          }}
          aria-label="返回我的书房"
        >
          <span className="brand-icon">
            <Feather size={20} />
          </span>
          <strong>砚台</strong>
          <span className="brand-line" />
          <span className="brand-caption">小说创作工作台</span>
        </div>
        <div className="topbar-center">
          {surface === "write" ? (
            <>
              <BookOpen size={13} />
              <span className="workspace-book-title">
                {detail.data?.project.title || "写作工作台"}
              </span>
              <ChevronRight size={12} />
              <span>正文</span>
            </>
          ) : (
            <>
              <span className="tiny-dot" /> 自己的故事，自己的节奏
            </>
          )}
        </div>
        <div className="top-actions">
          {surface === "write" && (
            <button
              className="button workspace-command-open"
              aria-label="快捷操作"
              onClick={() => setCommandsOpen(true)}
            >
              <Search size={14} />
              <span>快速查找</span>
              <kbd>Ctrl K</kbd>
            </button>
          )}
          {surface === "write" && (
            <button
              className={
                "button workspace-focus-toggle " + (focusMode ? "active" : "")
              }
              aria-label={focusMode ? "退出专注模式" : "专注写作"}
              aria-pressed={focusMode}
              onClick={() => {
                setFocusMode((v) => !v);
                setMobileNav(false);
              }}
            >
              {focusMode ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
              <span>{focusMode ? "退出专注" : "专注"}</span>
            </button>
          )}
          {surface !== "library" && (
            <button
              className="button author-home-nav"
              onClick={() => void navigate("library")}
            >
              <Library size={14} /> 书房
            </button>
          )}
          {surface === "write" && (
            <button
              className="button author-home-nav"
              onClick={() => void navigate("book")}
            >
              <BookMarked size={14} /> 作品规划
            </button>
          )}
          {surface === "write" && projectId && (
            <button
              className="button skills-open"
              onClick={() => setSkillsOpen(true)}
            >
              <Leaf size={14} /> 写作 Skills
            </button>
          )}
          {session.data.user?.admin && (
            <button
              className="button provider-open"
              onClick={() => setProviderOpen(true)}
            >
              <Settings size={14} /> AI 供应商
            </button>
          )}
          {installPrompt && (
            <button
              className="button"
              onClick={() =>
                void installPrompt
                  .prompt()
                  .then(() => installPrompt.userChoice)
                  .then(() => setInstallPrompt(null))
              }
            >
              <Download size={14} /> 安装书房
            </button>
          )}
          {surface === "write" && (
            <button
              className="button studio-open"
              disabled={!detail.data}
              onClick={() =>
                void act(async () => {
                  await draftFlush.current?.();
                  await qc.fetchQuery({
                    queryKey: ["project", projectId],
                    queryFn: () => api<Detail>("/projects/" + projectId),
                    staleTime: 0,
                  });
                  setStudioOpen(true);
                })
              }
            >
              <LayersIcon /> 创作工作室
            </button>
          )}
          {surface !== "library" && (
            <button
              className="icon-button"
              title="作品设置"
              aria-label="作品设置"
              disabled={!detail.data}
              onClick={() => setModal({ kind: "settings" })}
            >
              <Settings size={18} />
            </button>
          )}
          <button
            className="icon-button"
            title="导入作品"
            aria-label="导入作品"
            onClick={() => setModal({ kind: "import" })}
          >
            <Upload size={18} />
          </button>
          {surface !== "library" && (
            <button
              className="icon-button"
              title="导出完整作品"
              aria-label="导出完整作品"
              disabled={!projectId}
              onClick={() =>
                void act(async () => {
                  await draftFlush.current?.();
                  window.location.href = `/api/projects/${projectId}/export`;
                })
              }
            >
              <Download size={18} />
            </button>
          )}
          <span className="top-separator" />
          <button
            className="author-avatar"
            title="退出登录"
            aria-label="退出登录"
            onClick={() =>
              void act(async () => {
                await draftFlush.current?.();
                await api("/session", "DELETE");
                qc.clear();
                qc.setQueryData(["session"], {
                  authenticated: false,
                  csrfToken: "",
                });
                setProjectId("");
                setSurface("library");
                window.history.replaceState(null, "", window.location.pathname);
              })
            }
          >
            {(session.data.username || "作").slice(0, 1)}
          </button>
        </div>
      </header>
      {surface === "write" && (
        <>
          <nav className="workspace-rail" aria-label="创作导航">
            <div className="workspace-rail-main">
              <button
                aria-label="书房"
                title="返回书房"
                onClick={() => void navigate("library")}
              >
                <Library size={19} />
                <span>书房</span>
              </button>
              <button
                aria-label="作品规划"
                title="全书规划"
                onClick={() => void navigate("book")}
              >
                <BookMarked size={19} />
                <span>规划</span>
              </button>
              <button
                className="active"
                aria-label={directoryOpen ? "收起章节目录" : "展开章节目录"}
                aria-expanded={directoryOpen}
                onClick={toggleDirectory}
              >
                {directoryOpen ? (
                  <PanelLeftClose size={19} />
                ) : (
                  <PanelLeftOpen size={19} />
                )}
                <span>正文</span>
              </button>
              <span className="rail-divider" />
              <button
                aria-label="人物与世界"
                title="人物、世界与规则"
                disabled={!detail.data}
                onClick={() => openWorkspaceStudio("characters")}
              >
                <Users size={19} />
                <span>人物</span>
              </button>
              <button
                aria-label="故事画布"
                title="故事画布"
                disabled={!detail.data}
                onClick={() => openWorkspaceStudio("canvas")}
              >
                <Waypoints size={19} />
                <span>画布</span>
              </button>
              <button
                aria-label="作品资料库"
                title="作品资料与来源 Wiki"
                disabled={!detail.data}
                onClick={() => openWorkspaceStudio("wiki")}
              >
                <NotebookPen size={19} />
                <span>资料</span>
              </button>
              <button
                aria-label="写作 Skills"
                title="本书写作 Skills"
                disabled={!projectId}
                onClick={() => setSkillsOpen(true)}
              >
                <Leaf size={19} />
                <span>Skills</span>
              </button>
            </div>
            <div className="workspace-rail-bottom">
              <button
                aria-label="出版与导出"
                title="出版与导出"
                disabled={!detail.data}
                onClick={() => openWorkspaceStudio("publishing")}
              >
                <Download size={18} />
                <span>导出</span>
              </button>
              <button
                aria-label="作品设置"
                title="作品设置"
                disabled={!detail.data}
                onClick={() => setModal({ kind: "settings" })}
              >
                <Settings size={18} />
                <span>设置</span>
              </button>
            </div>
          </nav>
          {mobileNav && (
            <button
              className="workspace-nav-scrim"
              aria-label="关闭章节导航"
              onClick={() => setMobileNav(false)}
            />
          )}
          <aside
            aria-label="章节目录"
            aria-hidden={viewportWidth <= 760 && !mobileNav}
            inert={viewportWidth <= 760 && !mobileNav}
            className={
              "sidebar workspace-directory " +
              (mobileNav ? "mobile-visible" : "")
            }
          >
            <div className="directory-heading">
              <span>
                <BookOpen size={15} /> 正文目录 <small>{chapters.length}</small>
              </span>
              <div>
                <button
                  className="icon-button"
                  title="新建作品"
                  aria-label="新建作品"
                  onClick={() => setModal({ kind: "project" })}
                >
                  <Plus size={15} />
                </button>
                <button
                  className="icon-button directory-close"
                  title="收起章节目录"
                  aria-label="收起目录"
                  onClick={() => {
                    setMobileNav(false);
                    if (viewportWidth > 760) setDirectoryOpen(false);
                  }}
                >
                  <PanelLeftClose size={16} />
                </button>
              </div>
            </div>
            <div className="project-switch">
              <select
                aria-label="选择作品"
                value={projectId}
                onChange={(e) => void navigate("book", e.target.value)}
              >
                <option value="" disabled>
                  选择一部作品
                </option>
                {projects.data?.map((p) => (
                  <option value={p.id} key={p.id}>
                    {p.title}
                  </option>
                ))}
              </select>
            </div>
            <label className="directory-search">
              <Search size={14} />
              <input
                aria-label="搜索章节"
                placeholder="搜索章节或卷名"
                value={chapterSearch}
                onChange={(e) => setChapterSearch(e.target.value)}
              />
              {chapterSearch && (
                <button
                  className="icon-button"
                  type="button"
                  aria-label="清空章节搜索"
                  onClick={() => setChapterSearch("")}
                >
                  <X size={12} />
                </button>
              )}
            </label>
            <nav className="chapter-list" aria-label="本书章节">
              {(() => {
                const needle = chapterSearch.trim().toLowerCase();
                const visible = chapters.filter(
                  (c) =>
                    !needle ||
                    (c.title + " " + c.volume).toLowerCase().includes(needle),
                );
                const groups: {
                  key: string;
                  name: string;
                  chapters: Chapter[];
                }[] = [];
                for (const c of visible) {
                  const last = groups.at(-1);
                  const name = c.volume || "正文";
                  if (last?.name === name) last.chapters.push(c);
                  else
                    groups.push({
                      key: name + ":" + c.id,
                      name,
                      chapters: [c],
                    });
                }
                if (!visible.length)
                  return (
                    <div className="nav-empty">
                      {needle ? "没有找到匹配章节" : "故事从第一章开始"}
                    </div>
                  );
                return groups.map((group) => {
                  const collapsed = !needle && collapsedVolumes.has(group.key);
                  return (
                    <section className="directory-volume" key={group.key}>
                      <button
                        className="volume-label"
                        aria-expanded={!collapsed}
                        aria-label={(collapsed ? "展开" : "收起") + group.name}
                        onClick={() =>
                          setCollapsedVolumes((old) => {
                            const next = new Set(old);
                            if (next.has(group.key)) next.delete(group.key);
                            else next.add(group.key);
                            return next;
                          })
                        }
                      >
                        {collapsed ? (
                          <ChevronRight size={12} />
                        ) : (
                          <ChevronDown size={12} />
                        )}
                        <span>{group.name}</span>
                        <small>{group.chapters.length}</small>
                      </button>
                      {!collapsed &&
                        group.chapters.map((c) => (
                          <div
                            className={
                              "chapter-nav " +
                              (chapterId === c.id ? "selected" : "")
                            }
                            key={c.id}
                          >
                            <button
                              className="chapter-link"
                              aria-current={
                                chapterId === c.id ? "page" : undefined
                              }
                              onClick={() =>
                                void navigate("write", projectId, c.id)
                              }
                            >
                              <span className="chapter-number">
                                {String(chapters.indexOf(c) + 1).padStart(
                                  2,
                                  "0",
                                )}
                              </span>
                              <span className="chapter-title">{c.title}</span>
                              {c.needsReview ? (
                                <span
                                  className="chapter-dot warning"
                                  title="正文需复核"
                                />
                              ) : c.finalRevisionId ? (
                                <Check size={12} className="chapter-check" />
                              ) : (
                                <span className="chapter-dot" />
                              )}
                            </button>
                            <div className="chapter-nav-actions">
                              <button
                                aria-label={"上移" + c.title}
                                onClick={() => moveChapter(c, -1)}
                              >
                                <ArrowUp size={11} />
                              </button>
                              <button
                                aria-label={"下移" + c.title}
                                onClick={() => moveChapter(c, 1)}
                              >
                                <ArrowDown size={11} />
                              </button>
                            </div>
                          </div>
                        ))}
                    </section>
                  );
                });
              })()}
            </nav>
            <button
              className="new-chapter"
              disabled={!projectId}
              onClick={createChapter}
            >
              <FilePlus2 size={15} /> 添加章节
            </button>
            <div className="directory-mobile-actions">
              <button
                aria-label="返回作品规划"
                onClick={() => void navigate("book")}
              >
                <BookMarked size={14} />
                作品规划
              </button>
              <button
                aria-label="打开创作工作室"
                onClick={() => openWorkspaceStudio("canvas")}
                disabled={!detail.data}
              >
                <Waypoints size={14} />
                创作工具
              </button>
              <button
                aria-label="目录中打开写作 Skills"
                onClick={() => {
                  setMobileNav(false);
                  setSkillsOpen(true);
                }}
              >
                <Leaf size={14} />
                写作指南
              </button>
            </div>
            <div className="sidebar-bottom">
              <button onClick={() => setModal({ kind: "impacts" })}>
                <Clock3 size={14} /> 改稿影响记录{" "}
                {detail.data?.project.health === "RebuildRequired" && (
                  <span className="tiny-dot warning" />
                )}
              </button>
              <div className="local-note">
                <Leaf size={12} />
                <span>文字留在自己的书房</span>
              </div>
            </div>
          </aside>
          <WorkspaceDivider
            className="directory-divider"
            label="调整章节目录宽度"
            value={directoryWidth}
            min={190}
            max={directoryMax}
            onChange={setPreferredDirectoryWidth}
          />
        </>
      )}
      {surface === "library" ? (
        <LibraryHome
          projects={projects.data || []}
          loading={projects.isLoading}
          error={projects.error}
          onOpen={(p) => void navigate("book", p.id)}
          onCreate={() => setModal({ kind: "project" })}
          onProvider={
            session.data.user?.admin ? () => setProviderOpen(true) : undefined
          }
          onImport={() => setModal({ kind: "import" })}
          recentKey={recentKey}
          onResume={(pid, cid) => void navigate("write", pid, cid)}
        />
      ) : surface === "book" ? (
        detail.data ? (
          <BookHome
            key={projectId}
            detail={detail.data}
            onWrite={(cid) =>
              void navigate("write", projectId, cid || chapters[0]?.id)
            }
            onLibrary={() => void navigate("library")}
            onStudio={(tab) => {
              setStudioTab(tab);
              setStudioOpen(true);
            }}
            onRefresh={refresh}
            notify={notify}
            config={config.data}
          />
        ) : (
          <main className="library-home">
            <p>正在打开作品…</p>
            {detail.isError && (
              <div className="error-box">{message(detail.error)}</div>
            )}
          </main>
        )
      ) : (
        <>
          <main className="workspace">
            {projects.isError || detail.isError || chapter.isError ? (
              <div className="page-empty">
                <AlertCircle />
                <h2>暂时无法打开作品</h2>
                <p>
                  {message(projects.error || detail.error || chapter.error)}
                </p>
                <Button onClick={refresh}>重新加载</Button>
              </div>
            ) : !projectId ? (
              <div className="page-empty welcome">
                <span className="empty-illustration">
                  <BookOpen size={55} strokeWidth={1} />
                  <Feather size={31} strokeWidth={1.3} />
                </span>
                <span className="eyebrow">YOUR NEXT CHAPTER</span>
                <h1>
                  让一个念头，
                  <br />
                  长成一个世界。
                </h1>
                <p>
                  章纲、正文与故事记忆，都在这里。
                  <br />
                  你掌握方向，AI 陪你推敲每一章。
                </p>
                <Button primary onClick={() => setModal({ kind: "project" })}>
                  <Plus size={16} /> 创建第一部作品
                </Button>
              </div>
            ) : !chapterId ? (
              <div className="page-empty">
                <BookMarked size={43} strokeWidth={1.2} />
                <h2>{detail.data?.project.title}</h2>
                <p>
                  {detail.data?.project.description ||
                    "为这个世界写下第一段故事。"}
                </p>
                <Button primary onClick={createChapter}>
                  <Plus size={16} /> 新建第一章
                </Button>
              </div>
            ) : chapter.data && detail.data ? (
              <ChapterWorkspace
                key={chapterId + "-" + nonce}
                chapter={chapter.data}
                registerFlush={registerFlush}
                detail={detail.data}
                readOnly={
                  !!access.data &&
                  !["owner", "editor"].includes(access.data.role)
                }
                config={config.data}
                assistantOpen={assistantOpen}
                focusMode={focusMode}
                assistantWidth={assistantWidth}
                assistantMax={assistantMax}
                onAssistantWidth={setPreferredAssistantWidth}
                directoryOpen={directoryOpen}
                onToggleDirectory={toggleDirectory}
                onOpenAssistant={() => {
                  setFocusMode(false);
                  setAssistantOpen(true);
                  setMobileNav(false);
                }}
                onToggleAssistant={() => {
                  setFocusMode(false);
                  setAssistantOpen(focusMode ? true : !assistantOpen);
                  setMobileNav(false);
                }}
                onSaved={refresh}
                notify={notify}
                onHistory={() => setModal({ kind: "history" })}
                onReload={() => {
                  void qc
                    .fetchQuery({
                      queryKey: ["chapter", chapterId],
                      queryFn: () => api<Chapter>("/chapters/" + chapterId),
                      staleTime: 0,
                    })
                    .then(() => {
                      refresh();
                      setNonce((n) => n + 1);
                    });
                }}
              />
            ) : (
              <div className="app-loading compact">
                <LoaderCircle className="spin" />
                正在打开章节…
              </div>
            )}
          </main>
        </>
      )}
      <WorkspaceCommandPalette
        open={commandsOpen && surface === "write"}
        onClose={() => setCommandsOpen(false)}
        commands={commands}
      />
      {notice && (
        <div className="toast" role="status">
          <span>{notice}</span>
          <button
            className="icon-button"
            onClick={() => setNotice("")}
            aria-label="关闭通知"
          >
            <X size={15} />
          </button>
        </div>
      )}
      {modal && (
        <Modal
          title={
            modal.kind === "project"
              ? "新的故事"
              : modal.kind === "history"
                ? "正文历史版本"
                : modal.kind === "settings"
                  ? "作品与写作设置"
                  : modal.kind === "impacts"
                    ? "改稿影响记录"
                    : "导入已有文字"
          }
          onClose={() => setModal(null)}
          wide={modal.kind === "history" || modal.kind === "import"}
        >
          {modal.kind === "project" && (
            <NewBookWizard
              onCreated={(p) => {
                setProjectId(p.id);
                setSurface("book");
                window.history.replaceState(null, "", `#book/${p.id}`);
                refresh();
                setModal(null);
              }}
            />
          )}
          {modal.kind === "settings" && (
            <ProjectSettings
              project={detail.data?.project}
              config={config.data}
              onSaved={() => {
                refresh();
                setModal(null);
              }}
              onDeleted={() => {
                qc.setQueryData<Project[]>(["projects"], (old) =>
                  (old || []).filter((p) => p.id !== projectId),
                );
                setProjectId("");
                setSurface("library");
                setChapterId("");
                setModal(null);
                void qc.invalidateQueries({ queryKey: ["projects"] });
                notify("作品已删除。");
              }}
            />
          )}
          {modal.kind === "import" && (
            <ImportForm
              projectId={projectId}
              onDone={(id) => {
                setProjectId(id);
                setSurface("book");
                window.history.replaceState(null, "", `#book/${id}`);
                refresh();
                setModal(null);
                notify("文字已导入，原有内容和版本均保留");
              }}
            />
          )}
          {modal.kind === "history" && chapter.data && (
            <HistoryPanel
              chapter={chapter.data}
              onRestore={() => {
                void qc
                  .fetchQuery({
                    queryKey: ["chapter", chapterId],
                    queryFn: () => api<Chapter>("/chapters/" + chapterId),
                    staleTime: 0,
                  })
                  .then(() => {
                    refresh();
                    setNonce((n) => n + 1);
                    setModal(null);
                    notify("历史正文已恢复为一个新版本");
                  });
              }}
            />
          )}
          {modal.kind === "impacts" && (
            <ImpactPanel
              projectId={projectId}
              onNavigate={(id) => {
                void navigate("write", projectId, id);
                setModal(null);
              }}
            />
          )}
        </Modal>
      )}
      {studioOpen && detail.data && (
        <Suspense
          fallback={<div className="studio-loading">正在打开创作工作室…</div>}
        >
          <LazyStudio
            key={studioTab + projectId}
            initialTab={studioTab}
            detail={detail.data}
            chapterId={chapterId}
            onClose={() => setStudioOpen(false)}
            onRefresh={refresh}
            notify={notify}
          />
        </Suspense>
      )}
      {skillsOpen && projectId && (
        <Suspense
          fallback={<div className="studio-loading">正在打开写作指南…</div>}
        >
          <LazyWritingSkills
            key={projectId}
            projectId={projectId}
            notify={notify}
            onClose={() => setSkillsOpen(false)}
          />
        </Suspense>
      )}
      {providerOpen && (
        <ProviderSettings
          onClose={() => setProviderOpen(false)}
          notify={notify}
        />
      )}
    </div>
  );
}
function LayersIcon() {
  return <BookMarked size={15} />;
}

function Login({ onLogin }: { onLogin: (s: Session) => void }) {
  const [username, setUsername] = useState("author"),
    [password, setPassword] = useState(""),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false),
    [invited, setInvited] = useState(false),
    [token, setToken] = useState(""),
    [displayName, setDisplayName] = useState("");
  return (
    <div className="login-page">
      <section className="login-story">
        <div className="brand light">
          <Feather size={25} />
          <strong>砚台</strong>
          <span>小说创作工作台</span>
        </div>
        <div className="login-quote">
          <span className="eyebrow">A PLACE FOR YOUR STORIES</span>
          <h1>
            故事未完，
            <br />
            落笔有处。
          </h1>
          <p>
            从一页章纲，到一个自洽的世界。
            <br />
            让创作有序，让想象自由。
          </p>
          <div className="login-rule" />
          <span className="login-footnote">写作 · 推敲 · 记忆 · 续章</span>
        </div>
        <div className="login-landscape">
          <div />
          <div />
          <div />
        </div>
      </section>
      <section className="login-form-wrap">
        <form
          className="login-form"
          onSubmit={(e) => {
            e.preventDefault();
            setLoading(true);
            setError("");
            (invited
              ? api("/invitations/accept", "POST", {
                  token,
                  username,
                  password,
                  displayName,
                })
              : Promise.resolve()
            )
              .then(() =>
                api<Session>("/session", "POST", { username, password }),
              )
              .then(onLogin)
              .catch((e) => setError(message(e)))
              .finally(() => setLoading(false));
          }}
        >
          <span className="eyebrow">WELCOME BACK</span>
          <h2>{invited ? "加入创作团队" : "回到你的书房"}</h2>
          <p>
            {invited
              ? "填写邀请令牌，注册新账户或验证已有账户。"
              : "使用作者或协作者账户登录。"}
          </p>
          {invited && (
            <>
              <label>
                邀请令牌
                <input
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  required
                />
              </label>
              <label>
                显示名称
                <input
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                />
              </label>
            </>
          )}
          <label>
            作者账户
            <input
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </label>
          <label>
            密码
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </label>
          {error && (
            <div className="error-box" role="alert">
              {error}
            </div>
          )}
          <button className="button primary login-submit" disabled={loading}>
            {loading ? (
              <LoaderCircle size={17} className="spin" />
            ) : (
              <BookOpen size={17} />
            )}{" "}
            {invited ? "接受邀请并进入" : "进入书房"}
          </button>
          <button
            className="text-button"
            type="button"
            onClick={() => {
              setInvited(!invited);
              setError("");
            }}
          >
            {invited ? "返回账户登录" : "使用邀请加入作品"}
          </button>
          <span className="login-privacy">
            <Leaf size={13} /> 自托管 · 受控协作 · 作品由你掌握
          </span>
        </form>
      </section>
    </div>
  );
}

function ChapterWorkspace({
  chapter,
  detail,
  config,
  assistantOpen,
  focusMode,
  assistantWidth,
  assistantMax,
  onAssistantWidth,
  directoryOpen,
  onToggleDirectory,
  onOpenAssistant,
  onToggleAssistant,
  onSaved,
  notify,
  onHistory,
  onReload,
  readOnly = false,
  registerFlush,
}: {
  chapter: Chapter;
  detail: Detail;
  config?: { model: string; externalModel: boolean };
  assistantOpen: boolean;
  focusMode: boolean;
  assistantWidth: number;
  assistantMax: number;
  onAssistantWidth: (width: number) => void;
  directoryOpen: boolean;
  onToggleDirectory: () => void;
  onOpenAssistant: () => void;
  onToggleAssistant: () => void;
  onSaved: () => void;
  notify: (s: string) => void;
  onHistory: () => void;
  onReload: () => void;
  readOnly?: boolean;
  registerFlush: (flush: (() => Promise<string>) | null) => void;
}) {
  const draft = useDraft(chapter, onSaved);
  useEffect(() => {
    registerFlush(draft.flush);
    return () => registerFlush(null);
  }, [draft.flush, registerFlush]);
  const [title, setTitle] = useState(chapter.title),
    [metadataBusy, setMetadataBusy] = useState(false);
  const [editorReset, setEditorReset] = useState(0);
  const [selection, setSelection] = useState("");
  const [assistantRequest, setAssistantRequest] = useState<{
    id: number;
    text: string;
  }>();
  const saveTitle = async () => {
    if (title === chapter.title) return;
    setMetadataBusy(true);
    try {
      await api(`/chapters/${chapter.id}`, "PUT", {
        title,
        volume: chapter.volume,
        blueprint: chapter.blueprint,
      });
      onSaved();
    } catch (e) {
      notify(message(e));
    } finally {
      setMetadataBusy(false);
    }
  };
  const finalize = async () => {
    try {
      const rid = await draft.flush();
      if (!rid) {
        notify("请先写下正文再定稿");
        return;
      }
      const preview = await api<Impact>(
        `/chapters/${chapter.id}/impact-preview`,
        "POST",
        {},
      );
      if (
        preview.chapterIds?.length &&
        chapter.finalRevisionId &&
        chapter.finalRevisionId !== rid &&
        !confirm(
          `替换定稿会影响 ${preview.chapterIds.length} 章的记忆与引用。后续正文保留，是否继续？`,
        )
      )
        return;
      await api(`/chapters/${chapter.id}/finalize`, "POST", {
        revisionId: rid,
      });
      onSaved();
      notify("正文已定稿；下一步审核记忆变更");
    } catch (e) {
      notify(message(e));
    }
  };
  const applyCandidate = async (text: string, runId: string) => {
    try {
      const baseline = await draft.flush();
      await api<Revision>(`/chapters/${chapter.id}/revisions`, "POST", {
        expectedCurrentRevisionId: baseline,
        document: documentFromText(text),
        source: "ai",
        runId,
      });
      onReload();
      notify("选中的修订已保存为新版本");
    } catch (e) {
      notify(message(e));
    }
  };
  const count = plainText(draft.doc).replace(/\s/g, "").length;
  const savedLabel = {
    saved: "已保存",
    dirty: "等待保存",
    saving: "保存中",
    error: "保存失败",
    conflict: "版本冲突",
  }[draft.status];
  return (
    <div className="chapter-workspace">
      <section className="writing-pane">
        <div className="chapter-header">
          <div className="breadcrumbs">
            <button
              className="icon-button directory-restore"
              aria-label={directoryOpen ? "收起正文目录" : "展开正文目录"}
              title={directoryOpen ? "收起章节目录" : "展开章节目录"}
              onClick={onToggleDirectory}
            >
              <PanelLeftOpen size={16} />
            </button>
            <BookOpen size={13} />
            <span>{detail.project.title}</span>
            <ChevronRight size={12} />
            <span>{chapter.volume || "正文"}</span>
          </div>
          <div className="chapter-header-actions">
            <button
              className="text-button"
              title="编辑卷名"
              aria-label="编辑卷名"
              onClick={() => {
                const volume = prompt("本章所属卷名", chapter.volume || "正文");
                if (volume !== null)
                  void api(`/chapters/${chapter.id}`, "PUT", {
                    title: chapter.title,
                    volume,
                    blueprint: chapter.blueprint,
                  })
                    .then(() => {
                      onSaved();
                      notify("卷名已保存");
                    })
                    .catch((e) => notify(message(e)));
              }}
            >
              <BookOpen size={14} />
            </button>
            <button className="text-button" onClick={onHistory}>
              <History size={15} /> 历史
            </button>
            <button
              className="icon-button"
              title="删除当前章节"
              aria-label="删除当前章节"
              onClick={() => {
                if (confirm("删除本章会保留历史来源并标记后续影响。确认删除？"))
                  void api("/chapters/" + chapter.id, "DELETE")
                    .then(() => {
                      onSaved();
                      notify("章节已删除，历史来源保留。");
                    })
                    .catch((e) => notify(message(e)));
              }}
            >
              <Trash2 size={15} />
            </button>
            <button
              className="icon-button"
              aria-label={assistantOpen && !focusMode ? "收起助手" : "打开助手"}
              onClick={onToggleAssistant}
            >
              {assistantOpen && !focusMode ? (
                <PanelRightClose size={18} />
              ) : (
                <PanelRightOpen size={18} />
              )}
            </button>
          </div>
        </div>
        <div className="writing-meta">
          <span className="eyebrow">
            CHAPTER {String(chapter.order).padStart(2, "0")}
          </span>
          <span className={"save-status " + draft.status}>
            {draft.status === "saving" ? (
              <LoaderCircle size={12} className="spin" />
            ) : draft.status === "saved" ? (
              <Check size={12} />
            ) : (
              <AlertCircle size={12} />
            )}{" "}
            {savedLabel}
            {metadataBusy && " · 标题保存中"}
          </span>
        </div>
        <div className="chapter-title-wrap">
          <input
            className="chapter-title-input"
            readOnly={readOnly}
            aria-label="章节标题"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            onBlur={() => void saveTitle()}
          />
          <div className="chapter-badges">
            {chapter.finalRevisionId === draft.revision && draft.revision ? (
              <span className="pill green">
                <CheckCheck size={11} /> 正文已定稿
              </span>
            ) : (
              <span className="pill">创作中</span>
            )}
            {chapter.needsReview && (
              <span className="pill amber">后文待复核</span>
            )}
            <span className="pill subtle">
              {statusLabel[detail.project.health] || detail.project.health}
            </span>
          </div>
        </div>
        {draft.recovery && (
          <div className="recovery-banner">
            <span>发现此章尚未保存的本地编辑。</span>
            <button
              onClick={() => {
                draft.recover();
                setEditorReset((n) => n + 1);
              }}
            >
              恢复编辑
            </button>
            <button onClick={draft.discardRecovery}>丢弃缓冲</button>
          </div>
        )}
        {(draft.status === "error" || draft.status === "conflict") && (
          <div className="error-banner" role="alert">
            <AlertCircle size={15} />
            <span>{draft.error} 编辑内容已保留在本机缓冲。</span>
            <button
              onClick={() =>
                draft.status === "conflict"
                  ? onHistory()
                  : void draft.flush().catch((e) => notify(message(e)))
              }
            >
              {draft.status === "conflict" ? "查看版本" : "重试保存"}
            </button>
          </div>
        )}
        <div className="editor-paper">
          <Editor
            initial={draft.doc}
            onChange={draft.change}
            onSelection={setSelection}
            resetKey={editorReset}
            readOnly={readOnly}
            onAskAI={(text: string) => {
              setAssistantRequest({ id: Date.now(), text });
              onOpenAssistant();
            }}
          />
          {!count && (
            <span className="editor-placeholder">
              在这里，写下故事的第一句话…
            </span>
          )}
        </div>
        <footer className="writing-footer">
          <span>
            {count.toLocaleString()} 字<span className="footer-dot">·</span>
            中文写作<span className="footer-dot">·</span>自动保存
          </span>
          <div>
            <button
              className="text-button"
              disabled={readOnly}
              onClick={() =>
                void draft
                  .flush()
                  .then(() => notify("正文已保存"))
                  .catch((e) => notify(message(e)))
              }
            >
              <Save size={14} /> 保存
            </button>
            <Button
              primary
              onClick={() => void finalize()}
              disabled={readOnly || draft.status === "conflict"}
            >
              <Check size={14} /> 确认定稿
            </Button>
          </div>
        </footer>
      </section>
      {assistantOpen && (
        <>
          <button
            className="workspace-assistant-scrim"
            aria-label="关闭助手面板"
            onClick={onToggleAssistant}
          />
          <WorkspaceDivider
            className="assistant-divider"
            label="调整创作伙伴宽度"
            value={assistantWidth}
            min={300}
            max={assistantMax}
            reverse
            onChange={onAssistantWidth}
          />
        </>
      )}
      {
        <Assistant
          hidden={!assistantOpen}
          composeRequest={assistantRequest}
          onClose={onToggleAssistant}
          chapter={chapter}
          detail={detail}
          currentRevisionId={draft.revision}
          currentText={plainText(draft.doc)}
          selection={selection}
          config={config}
          flush={draft.flush}
          onApply={applyCandidate}
          onSaved={onSaved}
          notify={notify}
        />
      }
    </div>
  );
}

export function Modal({
  title,
  onClose,
  children,
  wide = false,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [onClose]);
  return (
    <div
      className="modal-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <section
        className={"modal " + (wide ? "wide" : "")}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header>
          <div>
            <span className="eyebrow">YAN TAI WORKSPACE</span>
            <h2>{title}</h2>
          </div>
          <button
            className="icon-button"
            aria-label="关闭弹窗"
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </header>
        <div className="modal-content">{children}</div>
      </section>
    </div>
  );
}

function ProjectForm({
  initial,
  onSubmit,
}: {
  initial?: Project;
  onSubmit: (v: {
    title: string;
    description: string;
    genre: string;
  }) => Promise<void>;
}) {
  const [title, setTitle] = useState(initial?.title || ""),
    [description, setDescription] = useState(initial?.description || ""),
    [genre, setGenre] = useState(initial?.genre || "东方幻想"),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <form
      className="form-stack"
      onSubmit={(e) => {
        e.preventDefault();
        setBusy(true);
        onSubmit({ title, description, genre })
          .catch((e) => setError(message(e)))
          .finally(() => setBusy(false));
      }}
    >
      <label>
        作品名
        <input
          autoFocus
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="给这个世界一个名字"
          required
          maxLength={200}
        />
      </label>
      <label>
        题材
        <input
          value={genre}
          onChange={(e) => setGenre(e.target.value)}
          placeholder="例如：东方幻想、悬疑、现实"
        />
      </label>
      <label>
        故事简介
        <textarea
          rows={4}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="故事从哪里开始，又将走向何处？"
        />
      </label>
      {error && <div className="error-box">{error}</div>}
      <Button primary disabled={busy}>
        {busy ? (
          <LoaderCircle size={15} className="spin" />
        ) : (
          <Feather size={15} />
        )}{" "}
        {initial ? "保存作品信息" : "开始这个故事"}
      </Button>
    </form>
  );
}

function ProjectSettings({
  project,
  config,
  onSaved,
  onDeleted,
}: {
  project?: Project;
  config?: { model: string; externalModel: boolean };
  onSaved: () => void;
  onDeleted: () => void;
}) {
  const [error, setError] = useState("");
  return (
    <div>
      {project ? (
        <ProjectForm
          initial={project}
          onSubmit={async (body) => {
            await api("/projects/" + project.id, "PUT", body);
            onSaved();
          }}
        />
      ) : (
        <p>请先创建作品。</p>
      )}
      <div className="config-note">
        <span className="section-label">当前模型配置</span>
        <strong>{config?.model || "读取配置中"}</strong>
        <p>
          {config?.externalModel
            ? "生成任务会把本次组装的正文、设定和相关历史发送给已配置的模型服务。"
            : "当前使用本地确定性假模型，仅用于功能验证。"}
        </p>
        <p>密钥由后端配置管理。费用缺少价格依据时显示“未知”。</p>
      </div>
      {project && (
        <>
          <div className="export-actions">
            <span className="section-label">可读正文导出</span>
            <div className="row">
              <button
                className="button"
                onClick={() => {
                  window.location.href = `/api/projects/${project.id}/markdown`;
                }}
              >
                <Download size={14} /> Markdown
              </button>
              <button
                className="button"
                onClick={() => {
                  window.location.href = `/api/projects/${project.id}/markdown?format=txt`;
                }}
              >
                <FilePlus2 size={14} /> TXT
              </button>
            </div>
            <p className="muted small">
              正文导出便于阅读；完整版本、记忆和来源请使用顶栏的作品包导出。
            </p>
          </div>
          <div className="delete-project">
            <button
              className="text-button danger"
              onClick={() => {
                if (
                  confirm(
                    `删除“${project.title}”及其全部作品数据？请先导出作品包，这项操作无法在界面撤销。`,
                  )
                )
                  void api("/projects/" + project.id, "DELETE")
                    .then(onDeleted)
                    .catch((e) => setError(message(e)));
              }}
            >
              <Trash2 size={13} /> 删除这部作品
            </button>
            {error && <p className="error-box">{error}</p>}
          </div>
        </>
      )}
    </div>
  );
}

function HistoryPanel({
  chapter,
  onRestore,
}: {
  chapter: Chapter;
  onRestore: () => void;
}) {
  const history = useQuery({
    queryKey: ["history", chapter.id],
    queryFn: () => api<Revision[]>(`/chapters/${chapter.id}/revisions`),
  });
  const [selected, setSelected] = useState<Revision | null>(null),
    [error, setError] = useState("");
  return (
    <div className="history-layout">
      <div className="history-list">
        {history.data?.map((r) => (
          <button
            key={r.id}
            className={selected?.id === r.id ? "selected" : ""}
            onClick={() => setSelected(r)}
          >
            <strong>
              {r.source === "ai"
                ? "AI 修订"
                : r.source === "restored"
                  ? "历史恢复"
                  : "作者编辑"}
              {r.id === chapter.currentRevisionId && (
                <span className="pill green">当前</span>
              )}
            </strong>
            <span>{dateTime(r.createdAt)}</span>
            <small>
              {r.plainText.length} 字 · {r.id.slice(0, 8)}
            </small>
          </button>
        ))}
        {history.isError && <p>{message(history.error)}</p>}
      </div>
      <div className="history-preview">
        {selected ? (
          <>
            <div className="row-between">
              <span className="section-label">版本正文</span>
              <Button
                primary
                disabled={selected.id === chapter.currentRevisionId}
                onClick={() =>
                  void api(`/chapters/${chapter.id}/restore`, "POST", {
                    revisionId: selected.id,
                    expectedCurrentRevisionId: chapter.currentRevisionId,
                  })
                    .then(onRestore)
                    .catch((e) => setError(message(e)))
                }
              >
                恢复为新版本
              </Button>
            </div>
            <pre>{selected.plainText}</pre>
            {error && <div className="error-box">{error}</div>}
          </>
        ) : (
          <div className="mini-empty">
            <History />
            <p>
              选择左侧版本查看正文。
              <br />
              恢复会创建新版本，历史始终保留。
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function ImportForm({
  projectId,
  onDone,
}: {
  projectId: string;
  onDone: (id: string) => void;
}) {
  const [text, setText] = useState(""),
    [preview, setPreview] = useState<{ title: string; text: string }[] | null>(
      null,
    ),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <div className="form-stack">
      <p className="muted">
        TXT / Markdown 会先展示分章结果；完整作品包会恢复为一部新作品。
      </p>
      <p className="muted small">
        作品包保留正文、设定、记忆、工作室历史、研究与只读活动档案；不迁移账号权限与活跃队列，媒体需另存素材。作品包不是全库备份。
      </p>
      <label className="file-drop">
        <Upload size={22} />
        <span>选择 TXT、Markdown 或作品 JSON 包</span>
        <input
          type="file"
          accept=".txt,.md,.markdown,.json"
          onChange={async (e) => {
            const f = e.target.files?.[0];
            if (!f) return;
            if (f.size > 10 * 1024 * 1024) {
              setError("文件不能超过 10 MB");
              return;
            }
            const content = await f.text();
            if (f.name.endsWith(".json")) {
              setBusy(true);
              try {
                const result = await api<Detail>("/import", "POST", {
                  package: JSON.parse(content),
                });
                onDone(result.project.id);
              } catch (e) {
                setError(message(e));
              } finally {
                setBusy(false);
              }
            } else {
              setText(content);
              setPreview(null);
            }
          }}
        />
      </label>
      <textarea
        rows={7}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setPreview(null);
        }}
        placeholder="也可以直接粘贴已有正文…"
      />
      {error && <div className="error-box">{error}</div>}
      {preview ? (
        <>
          <div className="import-preview">
            <span className="section-label">
              将导入 {preview.length} 章，可编辑标题
            </span>
            {preview.map((p, i) => (
              <div key={i}>
                <input
                  aria-label={`导入第${i + 1}章标题`}
                  value={p.title}
                  onChange={(e) =>
                    setPreview(
                      preview.map((x, j) =>
                        j === i ? { ...x, title: e.target.value } : x,
                      ),
                    )
                  }
                />
                <span>{p.text.length} 字</span>
              </div>
            ))}
          </div>
          <Button
            primary
            disabled={!projectId || busy}
            onClick={() => {
              setBusy(true);
              api<Detail>(`/projects/${projectId}/import`, "POST", {
                chapters: preview,
              })
                .then((d) => onDone(d.project.id))
                .catch((e) => setError(message(e)))
                .finally(() => setBusy(false));
            }}
          >
            确认导入到当前作品
          </Button>
          {!projectId && (
            <p className="muted">导入正文前请先创建或选择一部作品。</p>
          )}
        </>
      ) : (
        <Button
          primary
          disabled={!text.trim() || busy}
          onClick={() => {
            setBusy(true);
            api<{ title: string; text: string }[]>("/import/preview", "POST", {
              text,
            })
              .then(setPreview)
              .catch((e) => setError(message(e)))
              .finally(() => setBusy(false));
          }}
        >
          预览分章
        </Button>
      )}
    </div>
  );
}

function ImpactPanel({
  projectId,
  onNavigate,
}: {
  projectId: string;
  onNavigate: (id: string) => void;
}) {
  const impacts = useQuery({
    queryKey: ["impacts", projectId],
    queryFn: () => api<Impact[]>(`/projects/${projectId}/impacts`),
    enabled: !!projectId,
  });
  return (
    <div className="impact-list">
      {impacts.data?.length ? (
        impacts.data.map((item) => (
          <article className="card" key={item.id}>
            <span className="section-label">{dateTime(item.createdAt)}</span>
            <h3>{item.reason}</h3>
            <p>
              从第 {item.earliestOrder} 章起，相关状态需要重建；后文正文保留。
            </p>
            <div className="chip-list">
              {item.chapterIds.map((id, i) => (
                <button
                  className="chip"
                  key={id}
                  onClick={() => onNavigate(id)}
                >
                  受影响章节 {i + 1}
                  <ChevronRight size={11} />
                </button>
              ))}
            </div>
          </article>
        ))
      ) : (
        <div className="mini-empty">
          <CheckCheck />
          <p>
            暂无改稿影响记录。
            <br />
            换版、重排或删除定稿时会保留影响范围。
          </p>
        </div>
      )}
      {impacts.isError && (
        <div className="error-box">{message(impacts.error)}</div>
      )}
    </div>
  );
}
