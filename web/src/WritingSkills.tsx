import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookOpen,
  Check,
  ChevronDown,
  Download,
  FileText,
  Import,
  Layers3,
  LoaderCircle,
  Pencil,
  Plus,
  Save,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { api, message } from "./api";
import "./writingSkills.css";

export type SkillResource = { path: string; content: string };
export type SkillSource = {
  url?: string;
  revision?: string;
  license?: string;
  author?: string;
  verification?: string;
};
export type SkillPreview = {
  name: string;
  description: string;
  content: string;
  resources: SkillResource[];
  source: SkillSource;
  hash: string;
  warnings: string[];
  previewToken?: string;
};
export type WritingSkill = SkillPreview & {
  id: string;
  projectId: string;
  version: number;
  enabled: boolean;
  priority: number;
  taskTypes: string[];
};
export type WritingSkillList = { items: WritingSkill[]; version: number };
type CatalogItem = {
  id: string;
  name: string;
  description: string;
  content?: string;
  url?: string;
  license?: string;
  source?: string;
  notes?: string;
  taskTypes: string[];
};
type SkillCatalog = { builtin: CatalogItem[]; external: CatalogItem[] };
const taskOptions = [
  ["copilot", "创作对话"],
  ["planning", "规划"],
  ["draft", "起草"],
  ["revision", "修订"],
  ["rewrite", "选段改写"],
  ["review", "审稿"],
  ["voice", "作者声音"],
  ["reader", "读者反馈"],
  ["developmental", "发展编辑"],
] as const;
const sample = `---\nname: my-writing-guide\ndescription: 为这本书保留具体、克制而有差异的人物表达\n---\n\n# 写作方向\n\n- 优先遵循作者当前要求、人物知识边界与已确认的作品设定。\n- 用具体动作、感官与人物选择推进故事，删去替读者总结情绪的句子。\n- 对白体现说话者的欲望和关系，不让所有人使用相同的解释腔。\n- 修改先保存为候选，保留作者决定。\n`;
const safeSource = (url?: string) =>
  url && /^https?:\/\//i.test(url) ? url : undefined;
const fileData = (file: File) =>
  new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(",")[1] || "");
    reader.onerror = () => reject(new Error("无法读取这个文件，请重新选择。"));
    reader.readAsDataURL(file);
  });

export default function WritingSkills({
  projectId,
  notify,
  onClose,
}: {
  projectId: string;
  notify: (text: string) => void;
  onClose?: () => void;
}) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["writing-skills", projectId],
    queryFn: () =>
      api<WritingSkillList>(`/projects/${projectId}/writing-skills`),
  });
  const catalog = useQuery({
    queryKey: ["writing-skill-catalog"],
    queryFn: () => api<SkillCatalog>("/writing-skill-catalog"),
  });
  const access = useQuery({
    queryKey: ["studio", projectId, "access"],
    queryFn: () => api<{ role: string }>(`/projects/${projectId}/access`),
  });
  const canWrite =
    !!access.data && ["owner", "editor"].includes(access.data.role);
  const [mode, setMode] = useState<"markdown" | "github" | "catalog">(
    "catalog",
  );
  const [sourceText, setSourceText] = useState("");
  const [url, setUrl] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<SkillPreview | null>(null);
  const [importing, setImporting] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [priority, setPriority] = useState(0);
  const [tasks, setTasks] = useState<string[]>([]);
  const [working, setWorking] = useState("");
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<WritingSkill | null>(null);
  const [editText, setEditText] = useState("");
  const [deleteId, setDeleteId] = useState("");
  const [toggleState, setToggleState] = useState<Record<string, boolean>>({});
  const form = useRef<HTMLDivElement>(null);
  const previewPanel = useRef<HTMLElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (importing || editing) form.current?.scrollIntoView({ block: "start" });
  }, [importing, editing?.id]);
  useEffect(() => {
    if (preview) previewPanel.current?.scrollIntoView({ block: "start" });
  }, [preview?.hash]);
  const refresh = async () => {
    await qc.invalidateQueries({ queryKey: ["writing-skills", projectId] });
    void qc.invalidateQueries({ queryKey: ["project", projectId] });
  };
  const action = async (id: string, fn: () => Promise<void>) => {
    setWorking(id);
    setError("");
    try {
      await fn();
      await refresh();
    } catch (e) {
      setError(message(e));
    } finally {
      setWorking("");
    }
  };
  const reset = () => {
    setPreview(null);
    setFile(null);
    setEditing(null);
    setError("");
    setEnabled(true);
    setPriority(0);
    setTasks([]);
    setDeleteId("");
    if (picker.current) picker.current.value = "";
  };
  const changeSource = () => {
    setPreview(null);
    setError("");
  };
  const checkPreview = () =>
    action("preview", async () => {
      let body: Record<string, unknown>;
      if (mode === "catalog") return;
      if (mode === "github") body = { format: "github", url: url.trim() };
      else if (file) {
        if (file.size > 1024 * 1024)
          throw new Error(
            "请选择不超过 1 MB 的 Skill 文件；ZIP 解压后的文档总量需在 512 KB 内。",
          );
        if (/\.zip$/i.test(file.name))
          body = {
            format: "zip",
            filename: file.name,
            archiveBase64: await fileData(file),
          };
        else
          body = {
            format: /\.json$/i.test(file.name) ? "bundle" : "markdown",
            filename: file.name,
            content: await file.text(),
          };
      } else body = { format: "markdown", content: sourceText };
      const value = await api<SkillPreview>(
        `/projects/${projectId}/writing-skills/preview`,
        "POST",
        body,
      );
      setPreview(value);
      const existing = query.data?.items.find(
        (item) => item.name === value.name,
      );
      if (existing) {
        setEnabled(existing.enabled);
        setPriority(existing.priority);
        setTasks(existing.taskTypes);
      }
    });
  const previewCatalog = (item: CatalogItem) =>
    action("preview", async () => {
      const body = item.content
        ? { format: "markdown", content: item.content }
        : { format: "github", url: item.url };
      const value = await api<SkillPreview>(
        `/projects/${projectId}/writing-skills/preview`,
        "POST",
        body,
      );
      setPreview(value);
      const existing = query.data?.items.find(
        (skill) => skill.name === value.name,
      );
      setEnabled(existing?.enabled ?? true);
      setPriority(existing?.priority ?? 0);
      setTasks(existing?.taskTypes ?? item.taskTypes ?? []);
    });
  const confirmImport = () =>
    action("import", async () => {
      if (!preview) return;
      const existing = query.data?.items.find(
        (item) => item.name === preview.name,
      );
      if (existing)
        await api(
          `/projects/${projectId}/writing-skills/${existing.id}`,
          "PUT",
          {
            expectedVersion: existing.version,
            preview,
            enabled,
            priority,
            taskTypes: tasks,
          },
        );
      else
        await api(`/projects/${projectId}/writing-skills`, "POST", {
          preview,
          enabled,
          priority,
          taskTypes: tasks,
        });
      setImporting(false);
      setPreview(null);
      setSourceText("");
      setFile(null);
      notify(
        existing
          ? "Skill 资料包已更新为新版本，已有任务历史保留。"
          : enabled
            ? "写作 Skill 已导入并在本书启用。"
            : "写作 Skill 已导入，尚未启用。",
      );
    });
  const update = (skill: WritingSkill, patch: Record<string, unknown>) =>
    action(skill.id, async () => {
      await api(`/projects/${projectId}/writing-skills/${skill.id}`, "PUT", {
        expectedVersion: skill.version,
        ...patch,
      });
      notify("写作 Skill 已更新；后续任务将使用新版本。");
    });
  const items = query.data?.items || [];
  const existingImport = preview
    ? items.find((item) => item.name === preview.name)
    : undefined;
  const editStale =
    !!editing &&
    !!query.data &&
    query.data.items.find((item) => item.id === editing.id)?.version !==
      editing.version;
  const body = (
    <>
      <div className="skills-heading">
        <div>
          <span className="eyebrow">YOUR VOICE, YOUR CRAFT</span>
          <h2>
            <BookOpen size={22} /> 写作 Skills
          </h2>
          <p>把你的写作方法带进这本书，让创作伙伴知道哪些表达值得留下。</p>
        </div>
        {onClose && (
          <button
            className="icon-button"
            onClick={onClose}
            aria-label="关闭写作 Skills"
          >
            <X size={20} />
          </button>
        )}
      </div>
      <div className="skills-principle">
        <Sparkles size={18} />
        <p>
          Skill
          是可复用的写作指南。作者要求、人物知识与作品设定始终优先；生成结果仍先成为候选，由你审阅采用。建议每任务启用
          1–3 个指南，长 Skill 与参考文件在对话中按需读取。
        </p>
      </div>
      {query.isError && (
        <div className="error-box" role="alert">
          {message(query.error)}{" "}
          <button className="text-button" onClick={() => void query.refetch()}>
            重新读取
          </button>
        </div>
      )}
      {!canWrite && access.data && (
        <p className="skills-readonly">
          你可以查阅本书指南；只有作者和编辑可导入或调整。
        </p>
      )}
      <div className="skills-toolbar">
        <span>
          <strong>{items.filter((s) => s.enabled).length}</strong> 项启用{" "}
          <span className="skills-muted">
            / {items.length} 项指南 · 本书独立配置
          </span>
        </span>
        <button
          className="button primary"
          disabled={!canWrite || !!working}
          onClick={() => {
            reset();
            setImporting(true);
          }}
        >
          <Plus size={15} /> 导入写作 Skill
        </button>
      </div>
      {!query.isLoading && !items.length && !importing && (
        <div className="skills-empty">
          <Layers3 size={34} strokeWidth={1} />
          <h3>从一句“我希望这样写”开始。</h3>
          <p>
            导入已有的 SKILL.md、ZIP 资料包或 GitHub
            skill，也可以自己写一份指南。
          </p>
          <button
            className="button"
            disabled={!canWrite}
            onClick={() => {
              reset();
              setMode("catalog");
              setImporting(true);
            }}
          >
            挑选适合这本书的指南
          </button>
        </div>
      )}
      {error && (
        <div className="error-box skills-error" role="alert">
          {error}
          <button className="text-button" onClick={() => void refresh()}>
            刷新当前版本
          </button>
        </div>
      )}
      <div className="skills-list">
        {items.map((skill) => (
          <article
            key={skill.id}
            className={"skill-card " + (skill.enabled ? "enabled" : "")}
            data-skill-id={skill.id}
          >
            <div className="skill-card-top">
              <div className="skill-title">
                <span className="skill-icon">
                  <FileText size={20} />
                </span>
                <div>
                  <h3>{skill.name}</h3>
                  <span className="skill-meta">
                    v{skill.version} · 优先级 {skill.priority} ·{" "}
                    {skill.enabled ? "已启用" : "未启用"}
                  </span>
                </div>
              </div>
              <label className="skill-switch">
                <input
                  aria-label={`启用 ${skill.name}`}
                  type="checkbox"
                  checked={toggleState[skill.id] ?? skill.enabled}
                  disabled={!canWrite || !!working}
                  onChange={(event) => {
                    const next = event.target.checked;
                    setToggleState((old) => ({ ...old, [skill.id]: next }));
                    void update(skill, { enabled: next }).finally(() =>
                      setToggleState((old) => {
                        const remaining = { ...old };
                        delete remaining[skill.id];
                        return remaining;
                      }),
                    );
                  }}
                />
                <span>{skill.enabled ? "启用中" : "已停用"}</span>
              </label>
            </div>
            <p className="skill-description">{skill.description}</p>
            <div className="skill-tags">
              {skill.taskTypes.length ? (
                skill.taskTypes.map((type) => (
                  <span key={type}>
                    {taskOptions.find((o) => o[0] === type)?.[1] || type}
                  </span>
                ))
              ) : (
                <span>全部写作任务</span>
              )}
              {skill.resources?.length > 0 && (
                <span>{skill.resources.length} 份参考资料</span>
              )}
            </div>
            {skill.source?.url && (
              <p className="skill-source">
                来源：
                <a
                  href={safeSource(skill.source.url)}
                  target="_blank"
                  rel="noreferrer"
                >
                  {skill.source.url}
                </a>
              </p>
            )}
            <details className="skill-inspect">
              <summary>
                查看指南与来源 <ChevronDown size={12} />
              </summary>
              <SkillContents skill={skill} />
            </details>
            <div className="skill-actions">
              <a
                className="text-button"
                href={`/api/projects/${projectId}/writing-skills/${skill.id}/export`}
                download
              >
                <Download size={13} /> 导出资料包
              </a>
              <button
                className="text-button"
                disabled={!canWrite || !!working}
                onClick={() => {
                  reset();
                  setImporting(false);
                  setEditing(skill);
                  setEditText(skill.content);
                  setPriority(skill.priority);
                  setTasks(skill.taskTypes);
                  setEnabled(skill.enabled);
                }}
              >
                <Pencil size={13} /> 编辑
              </button>
              <button
                className="text-button"
                disabled={!canWrite || !!working}
                onClick={() =>
                  setDeleteId(deleteId === skill.id ? "" : skill.id)
                }
              >
                <Trash2 size={13} /> 移除
              </button>
            </div>
            {deleteId === skill.id && (
              <div className="skill-delete">
                <span>从本书移除“{skill.name}”？已有任务记录保留。</span>
                <button className="button" onClick={() => setDeleteId("")}>
                  保留
                </button>
                <button
                  className="button danger"
                  disabled={!!working}
                  onClick={() =>
                    void action(skill.id, async () => {
                      await api(
                        `/projects/${projectId}/writing-skills/${skill.id}?expectedVersion=${skill.version}`,
                        "DELETE",
                      );
                      setDeleteId("");
                      notify("写作 Skill 已从本书移除。");
                    })
                  }
                >
                  确认移除
                </button>
              </div>
            )}
          </article>
        ))}
      </div>
      {(importing || editing) && (
        <div className="skill-form" ref={form}>
          <div className="row-between">
            <h3>{editing ? `编辑 ${editing.name}` : "带入你的写作方法"}</h3>
            <button
              className="icon-button"
              aria-label="收起 Skill 表单"
              onClick={() => {
                setImporting(false);
                setEditing(null);
              }}
            >
              <X size={18} />
            </button>
          </div>
          {editStale && editing && (
            <div className="warning-box">
              这项 Skill 已有更新。你的草稿保留在下方，请先核对最新版本。
              <button
                className="text-button"
                onClick={() => {
                  setError("");
                  const latest = items.find((item) => item.id === editing.id);
                  if (latest) {
                    setEditing(latest);
                    setEditText(latest.content);
                    setPriority(latest.priority);
                    setTasks(latest.taskTypes);
                    setEnabled(latest.enabled);
                  } else setEditing(null);
                }}
              >
                重新载入最新版本（替换下方草稿）
              </button>
            </div>
          )}
          {editing ? (
            <label>
              SKILL.md 内容
              <textarea
                aria-label="编辑 Skill 内容"
                className="skill-markdown"
                rows={15}
                value={editText}
                onChange={(e) => setEditText(e.target.value)}
              />
              <small>
                修改 frontmatter 的 name、description
                可更新名称与介绍；原参考资料继续保留。
              </small>
            </label>
          ) : (
            <>
              <div
                className="skill-import-tabs"
                role="tablist"
                aria-label="导入方式"
              >
                <button
                  role="tab"
                  aria-selected={mode === "catalog"}
                  onClick={() => {
                    setMode("catalog");
                    changeSource();
                  }}
                >
                  精选指南
                </button>
                <button
                  role="tab"
                  aria-selected={mode === "markdown"}
                  onClick={() => {
                    setMode("markdown");
                    changeSource();
                  }}
                >
                  文件或 Markdown
                </button>
                <button
                  role="tab"
                  aria-selected={mode === "github"}
                  onClick={() => {
                    setMode("github");
                    changeSource();
                  }}
                >
                  GitHub 链接
                </button>
              </div>
              {mode === "catalog" ? (
                <>
                  <p className="skills-readonly">
                    选择具体写作目标，先查看完整指南，再决定是否启用。外部项目附有固定来源与许可，适用中文小说时可继续编辑。
                  </p>
                  {catalog.isError && (
                    <div className="error-box">{message(catalog.error)}</div>
                  )}
                  {catalog.isLoading && (
                    <p className="skills-readonly">正在读取精选指南…</p>
                  )}
                  <h4 className="skill-catalog-title">砚台 · 中文写作起点</h4>
                  <div className="skill-catalog">
                    {catalog.data?.builtin.map((item) => (
                      <article className="skill-catalog-card" key={item.id}>
                        <h4>{item.name}</h4>
                        <p>{item.description}</p>
                        <button
                          className="text-button"
                          disabled={!!working || !canWrite}
                          onClick={() => void previewCatalog(item)}
                        >
                          预览 {item.name}
                        </button>
                      </article>
                    ))}
                  </div>
                  <h4 className="skill-catalog-title">公开写作 Skill</h4>
                  <div className="skill-catalog">
                    {catalog.data?.external.map((item) => (
                      <article className="skill-catalog-card" key={item.id}>
                        <h4>{item.name}</h4>
                        <p>{item.description}</p>
                        {item.notes && <p>{item.notes}</p>}
                        <small>
                          {item.license} · {item.source || "公开仓库"}
                        </small>
                        <button
                          className="text-button"
                          disabled={!!working || !canWrite}
                          onClick={() => void previewCatalog(item)}
                        >
                          获取并预览 {item.name}
                        </button>
                      </article>
                    ))}
                  </div>
                </>
              ) : mode === "github" ? (
                <label>
                  Skill 的 GitHub 地址
                  <input
                    aria-label="Skill GitHub 地址"
                    value={url}
                    onChange={(e) => {
                      setUrl(e.target.value);
                      changeSource();
                    }}
                    placeholder="https://github.com/作者/仓库/tree/版本/skill目录"
                  />
                  <small>
                    读取公开 SKILL.md 和参考资料，预览后再由你确认导入。
                  </small>
                </label>
              ) : (
                <>
                  <label className="skill-file-picker">
                    <Import size={20} />
                    <span>
                      {file
                        ? file.name
                        : "选择 SKILL.md、Markdown、ZIP 或导出的 JSON 包"}
                    </span>
                    <input
                      ref={picker}
                      type="file"
                      aria-label="选择 Skill 文件"
                      accept=".md,.markdown,.txt,.zip,.json"
                      onChange={(e) => {
                        setFile(e.target.files?.[0] || null);
                        changeSource();
                      }}
                    />
                  </label>
                  {!file && (
                    <label>
                      或粘贴 SKILL.md
                      <textarea
                        aria-label="粘贴 Skill 内容"
                        className="skill-markdown"
                        rows={10}
                        value={sourceText}
                        onChange={(e) => {
                          setSourceText(e.target.value);
                          changeSource();
                        }}
                        placeholder={sample}
                      />
                    </label>
                  )}
                  {file && (
                    <button
                      className="text-button"
                      onClick={() => {
                        setFile(null);
                        changeSource();
                        if (picker.current) picker.current.value = "";
                      }}
                    >
                      清除文件，改为粘贴
                    </button>
                  )}
                </>
              )}
              {mode !== "catalog" && (
                <button
                  className="button"
                  disabled={
                    !!working ||
                    !canWrite ||
                    (mode === "github"
                      ? !url.trim()
                      : !file && !sourceText.trim())
                  }
                  onClick={() => void checkPreview()}
                >
                  {working === "preview" ? (
                    <LoaderCircle size={15} className="spin" />
                  ) : (
                    <FileText size={15} />
                  )}{" "}
                  预览 Skill
                </button>
              )}
              {preview && (
                <section
                  className="skill-preview"
                  aria-label="Skill 导入预览"
                  ref={previewPanel}
                >
                  <div className="row">
                    <Check size={18} />
                    <h3>{preview.name}</h3>
                    <span>尚未导入</span>
                  </div>
                  <p>{preview.description}</p>
                  <SkillContents skill={preview} />
                </section>
              )}
            </>
          )}
          {(editing || preview) && (
            <>
              {!editing && existingImport && (
                <p className="skill-warnings" role="status">
                  本书已有同名 Skill（v{existingImport.version}
                  ）。确认后将以此资料包更新正文、参考资料与来源，保留原 ID
                  和已发生的任务历史。
                </p>
              )}
              <div className="skill-options">
                <label className="skill-enabled">
                  <input
                    type="checkbox"
                    aria-label="在本书启用 Skill"
                    checked={enabled}
                    onChange={(e) => setEnabled(e.target.checked)}
                  />{" "}
                  在本书启用
                </label>
                <label>
                  优先级
                  <input
                    aria-label="Skill 优先级"
                    type="number"
                    min={-100}
                    max={100}
                    value={priority}
                    onChange={(e) => setPriority(Number(e.target.value))}
                  />
                  <small>数值越大，越先提供给 AI；范围 -100 到 100。</small>
                </label>
              </div>
              <fieldset className="skill-tasks">
                <legend>
                  适用任务 <small>不选择表示全部写作任务</small>
                </legend>
                {taskOptions.map(([value, label]) => (
                  <label key={value}>
                    <input
                      type="checkbox"
                      aria-label={`Skill 适用于${label}`}
                      checked={tasks.includes(value)}
                      onChange={() =>
                        setTasks((old) =>
                          old.includes(value)
                            ? old.filter((x) => x !== value)
                            : [...old, value],
                        )
                      }
                    />{" "}
                    {label}
                  </label>
                ))}
              </fieldset>
              <div className="skill-save">
                <small>
                  启用或修改会影响后续 AI 请求，旧候选会重新检查来源。
                </small>
                <button
                  className="button primary"
                  disabled={
                    !!working ||
                    !canWrite ||
                    editStale ||
                    (editing ? !editText.trim() : !preview)
                  }
                  onClick={() =>
                    void (editing
                      ? action("edit", async () => {
                          await api(
                            `/projects/${projectId}/writing-skills/${editing.id}`,
                            "PUT",
                            {
                              expectedVersion: editing.version,
                              content: editText,
                              enabled,
                              priority,
                              taskTypes: tasks,
                            },
                          );
                          setEditing(null);
                          notify("写作 Skill 新版本已保存。");
                        })
                      : confirmImport())
                  }
                >
                  {working === "import" || working === "edit" ? (
                    <LoaderCircle className="spin" size={15} />
                  ) : (
                    <Save size={15} />
                  )}
                  {editing
                    ? "保存 Skill 新版本"
                    : existingImport
                      ? "确认更新 Skill 资料包"
                      : "确认导入 Skill"}
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </>
  );
  return onClose ? (
    <div className="modal-backdrop">
      <section
        className="modal wide writing-skills-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="写作 Skills 管理"
      >
        {body}
      </section>
    </div>
  ) : (
    <section className="writing-skills-panel" aria-label="本书写作 Skills">
      {body}
    </section>
  );
}
function SkillContents({ skill }: { skill: SkillPreview }) {
  return (
    <div className="skill-contents">
      {(skill.warnings || []).length > 0 && (
        <div className="skill-warnings">
          <strong>导入提示</strong>
          <ul>
            {skill.warnings.map((warning, index) => (
              <li key={index}>{warning}</li>
            ))}
          </ul>
        </div>
      )}
      <dl className="skill-provenance">
        {skill.source?.verification && (
          <>
            <dt>来源核验</dt>
            <dd>
              {skill.source.verification === "github-pinned"
                ? "GitHub 固定版本已核验"
                : "导入文件的来源声明"}
            </dd>
          </>
        )}
        {skill.source?.license && (
          <>
            <dt>许可</dt>
            <dd>{skill.source.license}</dd>
          </>
        )}
        {skill.source?.author && (
          <>
            <dt>作者</dt>
            <dd>{skill.source.author}</dd>
          </>
        )}
        {skill.source?.revision && (
          <>
            <dt>来源版本</dt>
            <dd>{skill.source.revision}</dd>
          </>
        )}
        {skill.source?.url && (
          <>
            <dt>来源</dt>
            <dd>
              <a
                href={safeSource(skill.source.url)}
                target="_blank"
                rel="noreferrer"
              >
                {skill.source.url}
              </a>
            </dd>
          </>
        )}
        {skill.hash && (
          <>
            <dt>内容指纹</dt>
            <dd>{skill.hash.slice(0, 16)}</dd>
          </>
        )}
      </dl>
      <details open>
        <summary>SKILL.md</summary>
        <pre>{skill.content}</pre>
      </details>
      {(skill.resources || []).length > 0 && (
        <div className="skill-resources">
          <strong>参考资料 · {skill.resources.length} 份</strong>
          {skill.resources.map((resource) => (
            <details key={resource.path}>
              <summary>
                <FileText size={13} /> {resource.path}
              </summary>
              <pre>{resource.content}</pre>
            </details>
          ))}
        </div>
      )}
    </div>
  );
}
