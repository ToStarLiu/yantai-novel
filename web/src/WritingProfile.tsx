import { useEffect, useRef, useState, lazy, Suspense } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookOpen,
  Brain,
  Check,
  ChevronDown,
  Feather,
  Layers3,
  LoaderCircle,
  Save,
  Sparkles,
} from "lucide-react";
import { api, APIError, message, type Detail } from "./api";
import type { Brief } from "./AuthorHome";
import type { AuthorMemoryList } from "./AuthorMemories";
import type { WritingSkillList } from "./WritingSkills";
import { openAuthorMemories } from "./authorMemoryEvents";
import { registerNavigationGuard } from "./navigationGuard";
import "./writingProfile.css";
const WritingSkills = lazy(() => import("./WritingSkills"));

type StyleDraft = { style: string; pace: string; restrictions: string };
export default function WritingProfile({
  detail,
  notify,
}: {
  detail: Detail;
  notify: (text: string) => void;
}) {
  const pid = detail.project.id,
    qc = useQueryClient();
  const studio = useQuery({
    queryKey: ["book-brief", pid],
    queryFn: () =>
      api<{ brief: { version: number; data: Brief } }>(
        "/projects/" + pid + "/studio",
      ),
  });
  const memories = useQuery({
    queryKey: ["author-memories", pid],
    queryFn: () =>
      api<AuthorMemoryList>(
        "/author-memories?projectId=" + encodeURIComponent(pid),
      ),
  });
  const skills = useQuery({
    queryKey: ["writing-skills", pid],
    queryFn: () =>
      api<WritingSkillList>("/projects/" + pid + "/writing-skills"),
  });
  const access = useQuery({
    queryKey: ["studio", pid, "access"],
    queryFn: () => api<{ role: string }>("/projects/" + pid + "/access"),
  });
  const canWrite = ["owner", "editor"].includes(access.data?.role || "");
  const [editing, setEditing] = useState(false),
    [draft, setDraft] = useState<StyleDraft>({
      style: "",
      pace: "",
      restrictions: "",
    });
  const [version, setVersion] = useState(0),
    [base, setBase] = useState<Brief | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [skillsOpen, setSkillsOpen] = useState(false);
  const dirty = useRef(false);
  dirty.current =
    editing &&
    (draft.style !== (base?.style || "") ||
      draft.pace !== (base?.pace || "") ||
      draft.restrictions !== (base?.restrictions || ""));
  useEffect(
    () =>
      registerNavigationGuard(
        () =>
          !dirty.current ||
          window.confirm(
            "文风与写法还有未保存的修改，离开后会放弃这些修改。仍要离开吗？",
          ),
      ),
    [],
  );
  useEffect(() => {
    const leave = (event: BeforeUnloadEvent) => {
      if (dirty.current) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", leave);
    return () => window.removeEventListener("beforeunload", leave);
  }, []);
  const active =
    memories.data?.items.filter((item) => item.status === "active") || [];
  const enabledSkills = skills.data?.items.filter((item) => item.enabled) || [];
  const style = studio.data?.brief.data;
  const stale = editing && studio.data?.brief.version !== version;
  const begin = () => {
    if (!studio.data) return;
    setBase(studio.data.brief.data);
    setDraft({
      style: style?.style || "",
      pace: style?.pace || "",
      restrictions: style?.restrictions || "",
    });
    setVersion(studio.data.brief.version);
    setEditing(true);
    setError("");
  };
  const save = async () => {
    if (!base || stale || busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/projects/" + pid + "/studio/brief", "PUT", {
        expectedVersion: version,
        data: { ...base, ...draft },
      });
      setEditing(false);
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["book-brief", pid] }),
        qc.invalidateQueries({ queryKey: ["studio", pid] }),
        qc.invalidateQueries({ queryKey: ["copilot-artifact-run"] }),
      ]);
      notify("本书文风与写法已保存，后续任务按新版本执行。");
    } catch (cause) {
      setError(message(cause));
      if (cause instanceof APIError && cause.status === 409)
        void studio.refetch();
    } finally {
      setBusy(false);
    }
  };
  const rememberSample = () =>
    openAuthorMemories({
      projectId: pid,
      initialDraft: {
        scope: "book",
        projectId: pid,
        kind: "voice",
        title: "我的文字样例与写法",
        content: "",
        pinned: true,
      },
    });
  return (
    <section className="writing-profile" aria-label="文风与写法">
      <header className="writing-profile-intro">
        <span className="eyebrow">YOUR VOICE, THIS STORY</span>
        <h2>让文字，听起来像你。</h2>
        <p>先确定这本书想怎样写，再选择方法。偏好和样例都可以随创作调整。</p>
      </header>
      <div className="writing-profile-order" aria-label="写法采用顺序">
        <Layers3 size={16} />
        <div>
          <strong>发生分歧时，按这个顺序参考</strong>
          <p>本次明确要求 → 本书设定与写法 → 你的长期偏好 → 通用 Skills</p>
          <small>
            人物知识和已确认事实始终保留。这里列出当前配置，具体任务会显示实际使用的版本。
          </small>
        </div>
      </div>
      {(studio.isError || memories.isError || skills.isError) && (
        <div role="alert" className="error-box">
          {message(studio.error || memories.error || skills.error)}
          <button
            className="text-button"
            onClick={() => {
              void studio.refetch();
              void memories.refetch();
              void skills.refetch();
            }}
          >
            重新读取写法设置
          </button>
        </div>
      )}
      <article className="writing-profile-card">
        <header>
          <span>
            <BookOpen size={17} />
            <strong>这本书的写法</strong>
          </span>
          <button
            className="button"
            disabled={!canWrite || !studio.data || busy}
            onClick={() => {
              if (editing) {
                if (
                  dirty.current &&
                  !window.confirm("放弃尚未保存的本书写法修改？")
                )
                  return;
                setEditing(false);
              } else begin();
            }}
          >
            {editing ? "取消编辑" : "编辑本书写法"}
          </button>
        </header>
        <p className="writing-profile-note">
          随作品分享给协作者；只调整本书，不改变你在其他作品中的偏好。
        </p>
        {editing ? (
          <form
            className="writing-profile-form"
            onSubmit={(e) => {
              e.preventDefault();
              void save();
            }}
          >
            <label>
              文风方向
              <textarea
                aria-label="本书文风方向"
                value={draft.style}
                rows={3}
                maxLength={6000}
                placeholder="例如：叙述冷静，情绪藏在动作里；保留人物各自的口吻。"
                onChange={(e) => setDraft({ ...draft, style: e.target.value })}
              />
            </label>
            <label>
              节奏与句式
              <textarea
                aria-label="本书节奏与句式"
                value={draft.pace}
                rows={2}
                maxLength={6000}
                placeholder="例如：追逐用短句，关系戏留出停顿。"
                onChange={(e) => setDraft({ ...draft, pace: e.target.value })}
              />
            </label>
            <label>
              需要避免的写法
              <textarea
                aria-label="本书避免的写法"
                value={draft.restrictions}
                rows={2}
                maxLength={6000}
                placeholder="例如：不在每段末尾点题，不替读者总结人物情绪。"
                onChange={(e) =>
                  setDraft({ ...draft, restrictions: e.target.value })
                }
              />
            </label>
            {error && (
              <p role="alert" className="error-box">
                {error}
              </p>
            )}
            {stale && (
              <div className="warning-box">
                作品规划已在别处更新，你的草稿仍保留。
                <button type="button" className="text-button" onClick={begin}>
                  载入最新写法（替换草稿）
                </button>
              </div>
            )}
            <button
              type="submit"
              className="button primary"
              disabled={!canWrite || busy || stale}
            >
              {busy ? (
                <LoaderCircle size={14} className="spin" />
              ) : (
                <Save size={14} />
              )}
              保存本书写法
            </button>
          </form>
        ) : (
          <dl className="writing-profile-book">
            <div>
              <dt>文风方向</dt>
              <dd>{style?.style || "还没有指定，先与创作伙伴讨论也可以。"}</dd>
            </div>
            <div>
              <dt>节奏与句式</dt>
              <dd>{style?.pace || "跟随场景与人物需要。"}</dd>
            </div>
            <div>
              <dt>需要避免</dt>
              <dd>{style?.restrictions || "还没有额外限制。"}</dd>
            </div>
          </dl>
        )}
        {detail.settings.some((item) => item.kind === "style") && (
          <details className="writing-profile-extra">
            <summary>
              其他已保存的本书文风设定
              <ChevronDown size={12} />
            </summary>
            {detail.settings
              .filter((item) => item.kind === "style")
              .map((item) => (
                <div key={item.id}>
                  <strong>{item.name}</strong>
                  <p>{item.content}</p>
                </div>
              ))}
          </details>
        )}
      </article>
      <article className="writing-profile-card">
        <header>
          <span>
            <Brain size={17} />
            <strong>我的长期偏好</strong>
          </span>
          <button
            className="button"
            onClick={() => openAuthorMemories({ projectId: pid })}
          >
            管理作者记忆
          </button>
        </header>
        <p className="writing-profile-note">
          仅你可见。作者通用偏好跨书使用，本书偏好只在这里生效。
        </p>
        {memories.data?.settings.useMemory === false && (
          <p className="writing-profile-paused">
            记忆使用已暂停；条目仍保留，可在作者记忆中重新开启。
          </p>
        )}
        {active.length ? (
          <ul className="writing-profile-memories">
            {active.map((item) => (
              <li key={item.id}>
                <span>{item.scope === "book" ? "本书" : "作者通用"}</span>
                <strong>{item.title}</strong>
                <p>{item.content}</p>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">
            还没有已确认的偏好。可以从一句“我喜欢这样写”开始。
          </p>
        )}
      </article>
      <article className="writing-profile-card writing-profile-sample">
        <header>
          <span>
            <Feather size={17} />
            <strong>用自己的文字说明</strong>
          </span>
          <button className="button" onClick={rememberSample}>
            添加我的文字样例
          </button>
        </header>
        <p>
          选一小段自己满意的文字，再说明想保留的地方，例如节奏、对白或情绪表达。比“模仿某某风格”更便于具体修改。
        </p>
        <small>
          保存为仅你可见的本书文风记忆。每条最多1200字，建议样例与说明合起来保持简短。
        </small>
      </article>
      <article className="writing-profile-card">
        <header>
          <span>
            <Sparkles size={17} />
            <strong>本书的写作 Skills</strong>
          </span>
          <button
            className="button"
            aria-expanded={skillsOpen}
            onClick={() => setSkillsOpen(!skillsOpen)}
          >
            {skillsOpen ? "收起 Skills 管理" : "选择与管理 Skills"}
          </button>
        </header>
        <p className="writing-profile-note">
          Skills
          是方法，作者偏好是取舍。启用互补的指南，规则冲突时优先本次要求和本书写法。
        </p>
        {enabledSkills.length ? (
          <ul className="writing-profile-skills">
            {enabledSkills.map((skill) => (
              <li key={skill.id}>
                <Check size={13} />
                <strong>{skill.name}</strong>
                <span>
                  v{skill.version} · 优先级 {skill.priority}
                </span>
                <small>
                  {skill.taskTypes.length
                    ? skill.taskTypes.join(" / ")
                    : "各类写作任务"}
                </small>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">还没有启用写作指南。</p>
        )}
        {skillsOpen && (
          <Suspense fallback={<p>正在打开写作 Skills…</p>}>
            <WritingSkills projectId={pid} notify={notify} />
          </Suspense>
        )}
      </article>
    </section>
  );
}
