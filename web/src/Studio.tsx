import { useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Workflow,
  Network,
  Waypoints,
  Clapperboard,
  Users,
  Library,
  BookOpen,
  FlaskConical,
  GitBranch,
  Factory,
  SlidersHorizontal,
  Printer,
  ChevronRight,
  X,
  ArrowLeft,
  Plus,
  LoaderCircle,
  RefreshCw,
  Save,
  Play,
  Check,
  Trash2,
  Layers3,
  FileText,
  Upload,
  Sparkles,
  History,
} from "lucide-react";
import { api, message, type Detail, type Chapter, type Run } from "./api";
import { studioApi, type StudioTab, type StudioItem, title } from "./studioApi";
import CanvasPanel from "./studio/CanvasPanel";
import PlanningPanel from "./studio/PlanningPanel";
import ScenePanel from "./studio/ScenePanel";
import CharacterPanel from "./studio/CharacterPanel";
import KnowledgePanel from "./studio/KnowledgePanel";
import SimulationPanel from "./studio/SimulationPanel";
import WorkflowPanel from "./studio/WorkflowPanel";
import ModelPanel from "./studio/ModelPanel";
import PublishingPanel from "./studio/PublishingPanel";
import CommentPanel from "./studio/CommentPanel";
import CollaborationPanel from "./studio/CollaborationPanel";
import "./studio.css";

const tabs: {
  id: StudioTab;
  label: string;
  subtitle: string;
  Icon: typeof Network;
}[] = [
  {
    id: "collaboration",
    label: "协作与权限",
    subtitle: "成员、邀请与在线位置",
    Icon: Users,
  },
  {
    id: "canvas",
    label: "剧情画布",
    subtitle: "节点、条件与因果",
    Icon: Network,
  },
  {
    id: "planning",
    label: "故事规划",
    subtitle: "从核心命题到章纲",
    Icon: Waypoints,
  },
  {
    id: "scenes",
    label: "场景导演",
    subtitle: "目标、冲突与交接",
    Icon: Clapperboard,
  },
  {
    id: "characters",
    label: "人物弧光",
    subtitle: "欲望、情绪与认知",
    Icon: Users,
  },
  {
    id: "references",
    label: "参考资料",
    subtitle: "隔离分析与文风提炼",
    Icon: Library,
  },
  {
    id: "wiki",
    label: "故事百科",
    subtitle: "有来源的世界知识",
    Icon: BookOpen,
  },
  {
    id: "sandbox",
    label: "世界沙盘",
    subtitle: "预演、冻结与回滚",
    Icon: FlaskConical,
  },
  {
    id: "branches",
    label: "故事分支",
    subtitle: "探索另一种可能",
    Icon: GitBranch,
  },
  {
    id: "workflow",
    label: "流程与团队",
    subtitle: "职责、技能与权限",
    Icon: Workflow,
  },
  {
    id: "production",
    label: "批量制作",
    subtitle: "分批审批与预算",
    Icon: Factory,
  },
  {
    id: "models",
    label: "模型与检索",
    subtitle: "路由、缓存与召回",
    Icon: SlidersHorizontal,
  },
  {
    id: "publishing",
    label: "出版与声音",
    subtitle: "成书、封面与有声",
    Icon: Printer,
  },
];
export type PanelProps = {
  detail: Detail;
  chapterId: string;
  notify: (s: string) => void;
  refresh: () => void;
};
export function StudioCard({
  title,
  subtitle,
  children,
  actions,
}: {
  title: string;
  subtitle?: string;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section className="studio-card">
      <header>
        <div>
          <h3>{title}</h3>
          {subtitle && <p>{subtitle}</p>}
        </div>
        {actions}
      </header>
      {children}
    </section>
  );
}
export function StudioError({ error }: { error: unknown }) {
  return error ? (
    <div className="error-box" role="alert">
      {message(error)}
    </div>
  ) : null;
}
export function EmptyStudio({ text }: { text: string }) {
  return (
    <div className="studio-empty">
      <Layers3 size={35} strokeWidth={1.2} />
      <p>{text}</p>
    </div>
  );
}
export function StudioButton({
  children,
  onClick,
  disabled = false,
  primary = false,
  type = "button",
}: {
  children: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  primary?: boolean;
  type?: "button" | "submit";
}) {
  return (
    <button
      type={type}
      className={"button " + (primary ? "primary" : "")}
      onClick={onClick}
      disabled={disabled}
    >
      {children}
    </button>
  );
}
export function useStudioAction(
  notify: (s: string) => void,
  refresh: () => void,
) {
  const [working, setWorking] = useState(false);
  return {
    working,
    act: async (fn: () => Promise<unknown>, success?: string) => {
      setWorking(true);
      try {
        await fn();
        refresh();
        if (success) notify(success);
      } catch (e) {
        notify(message(e));
      } finally {
        setWorking(false);
      }
    },
  };
}
export function ChapterSelect({
  chapters,
  value,
  onChange,
  label = "目标章节",
}: {
  chapters: Chapter[];
  value: string;
  onChange: (s: string) => void;
  label?: string;
}) {
  return (
    <label>
      {label}
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">请选择章节</option>
        {chapters
          .filter((c) => !c.deleted)
          .map((c) => (
            <option key={c.id} value={c.id}>
              第 {c.order} 章 · {c.title}
            </option>
          ))}
      </select>
    </label>
  );
}
export function Result({ value }: { value: unknown }) {
  if (value == null) return null;
  const text =
    typeof value === "string" ? value : JSON.stringify(value, null, 2);
  return <pre className="studio-result">{text}</pre>;
}

export default function Studio({
  initialTab = "canvas",
  detail,
  chapterId,
  onClose,
  onRefresh,
  notify,
}: {
  initialTab?: StudioTab;
  detail: Detail;
  chapterId: string;
  onClose: () => void;
  onRefresh: () => void;
  notify: (s: string) => void;
}) {
  const [tab, setTab] = useState<StudioTab>(initialTab);
  const [commentsOpen, setCommentsOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const qc = useQueryClient();
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["studio", detail.project.id] });
    onRefresh();
  };
  const props = { detail, chapterId, notify, refresh };
  const active = tabs.find((t) => t.id === tab)!;
  return (
    <div
      className="studio-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="创作工作室"
    >
      <header className="studio-topbar">
        <div className="studio-brand">
          <span>
            <Workflow size={20} />
          </span>
          <div>
            <strong>创作工作室</strong>
            <small>{detail.project.title}</small>
          </div>
        </div>
        <div className="studio-top-note">
          <span className="tiny-dot" /> 探索可以大胆，采用始终由你决定
        </div>
        <div className="studio-actions">
          <StudioButton onClick={() => setCommentsOpen(!commentsOpen)}>
            本章批注
          </StudioButton>
          {["canvas", "planning", "scenes", "characters"].includes(tab) && (
            <StudioButton onClick={() => setHistoryOpen(!historyOpen)}>
              <History size={14} /> 规划历史
            </StudioButton>
          )}
          <StudioButton onClick={onClose}>
            <ArrowLeft size={15} /> 返回写作
          </StudioButton>
        </div>
      </header>
      <aside className="studio-nav">
        <span className="eyebrow">THE BIGGER PICTURE</span>
        {tabs.map(({ id, label, subtitle, Icon }) => (
          <button
            key={id}
            className={tab === id ? "active" : ""}
            onClick={() => setTab(id)}
          >
            <Icon size={18} />
            <span>
              <strong>{label}</strong>
              <small>{subtitle}</small>
            </span>
            <ChevronRight size={12} />
          </button>
        ))}
      </aside>
      <main
        className={"studio-main " + (tab === "canvas" ? "canvas-main" : "")}
      >
        <div className="studio-page-heading">
          <div>
            <span className="eyebrow">YAN TAI · CREATIVE STUDIO</span>
            <h2>{active.label}</h2>
            <p>{active.subtitle}</p>
          </div>
          <button
            className="icon-button"
            aria-label="刷新工作室"
            onClick={refresh}
          >
            <RefreshCw size={18} />
          </button>
        </div>
        {tab === "canvas" ? (
          <CanvasPanel {...props} />
        ) : tab === "planning" ? (
          <PlanningPanel {...props} />
        ) : tab === "scenes" ? (
          <ScenePanel {...props} />
        ) : tab === "characters" ? (
          <CharacterPanel {...props} />
        ) : tab === "references" || tab === "wiki" ? (
          <KnowledgePanel {...props} mode={tab} />
        ) : tab === "sandbox" || tab === "branches" ? (
          <SimulationPanel {...props} mode={tab} />
        ) : tab === "workflow" || tab === "production" ? (
          <WorkflowPanel {...props} mode={tab} />
        ) : tab === "collaboration" ? (
          <CollaborationPanel {...props} />
        ) : tab === "models" ? (
          <ModelPanel {...props} />
        ) : (
          <PublishingPanel {...props} />
        )}
      </main>
      {commentsOpen && (
        <CommentPanel {...props} onClose={() => setCommentsOpen(false)} />
      )}
      {historyOpen && (
        <PlanningHistory
          {...props}
          kind={
            tab === "canvas"
              ? "board"
              : tab === "planning"
                ? "brief"
                : tab === "characters"
                  ? "arcs"
                  : "scenes"
          }
          onClose={() => setHistoryOpen(false)}
        />
      )}
    </div>
  );
}

function PlanningHistory({
  detail,
  kind,
  notify,
  refresh,
  onClose,
}: PanelProps & { kind: string; onClose: () => void }) {
  const history = useQuery({
    queryKey: ["studio", detail.project.id, "history", kind],
    queryFn: () =>
      api<{ version: number; data: unknown; createdAt: string }[]>(
        `/projects/${detail.project.id}/studio/history/${kind}?id=main`,
      ),
  });
  const [selected, setSelected] = useState(0);
  const { working, act } = useStudioAction(notify, refresh);
  return (
    <aside className="studio-comments">
      <div className="row-between">
        <h3>规划历史</h3>
        <button className="icon-button" onClick={onClose}>
          <X size={18} />
        </button>
      </div>
      <p className="muted small">
        恢复会创建一个新版本，既往规划与运行输入保持可追溯。
      </p>
      {history.data?.map((entry) => (
        <div className="studio-inset" key={entry.version}>
          <span className="section-label">
            版本 {entry.version} ·{" "}
            {new Date(entry.createdAt).toLocaleString("zh-CN")}
          </span>
          <StudioButton onClick={() => setSelected(entry.version)}>
            查看此版本
          </StudioButton>
          {selected === entry.version && <Result value={entry.data} />}
          <StudioButton
            disabled={working || entry.version === history.data?.[0]?.version}
            onClick={() =>
              void act(
                () =>
                  api(
                    `/projects/${detail.project.id}/studio/history/${kind}/restore`,
                    "POST",
                    {
                      id: "main",
                      expectedVersion: history.data?.[0]?.version || 0,
                      version: entry.version,
                    },
                  ),
                "规划已恢复为新版本。",
              )
            }
          >
            恢复为新版本
          </StudioButton>
        </div>
      ))}
      <StudioError error={history.error} />
    </aside>
  );
}
