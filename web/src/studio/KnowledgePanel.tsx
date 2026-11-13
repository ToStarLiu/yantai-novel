import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Upload,
  Search,
  Trash2,
  BookOpen,
  Link2,
  Save,
  Quote,
} from "lucide-react";
import { api } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  Result,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Reference = {
  id: string;
  title: string;
  text: string;
  sourceNote: string;
  analysis: {
    wordCount: number;
    paragraphCount: number;
    dialogueRatio: number;
    topTerms: { term: string; count: number }[];
    structure: { title: string; paragraph: number }[];
    writingProfile: string;
  };
};
type Wiki = {
  entityId: string;
  name: string;
  kind: string;
  authorNotes: string;
  claims: {
    predicate: string;
    object: string;
    supporting: Source[];
    opposing: Source[];
  }[];
  supporting: Source[];
  opposing: Source[];
  beliefs: {
    fact: unknown;
    recallWeight: number;
    forgottenProjection: boolean;
  }[];
};
type Source = {
  revisionId: string;
  quote: string;
  chapterOrder: number;
  truthStatus: string;
};
export default function KnowledgePanel({
  detail,
  notify,
  refresh,
  mode,
}: PanelProps & { mode: "references" | "wiki" }) {
  const base = `/projects/${detail.project.id}/studio`;
  const q = useQuery({
    queryKey: ["studio", detail.project.id, "reference-document"],
    queryFn: () => api<{ references: Reference[] }>(base),
  });
  const wiki = useQuery({
    queryKey: ["studio", detail.project.id, "wiki"],
    queryFn: () => api<Wiki[]>(base + "/wiki"),
    enabled: mode === "wiki",
  });
  const [title, setTitle] = useState(""),
    [text, setText] = useState(""),
    [note, setNote] = useState(""),
    [selected, setSelected] = useState(""),
    [term, setTerm] = useState(""),
    [results, setResults] = useState<unknown>(null);
  const [researchTopic, setResearchTopic] = useState(""),
    [urls, setUrls] = useState(""),
    [researchResult, setResearchResult] = useState<unknown>(null),
    [searchHits, setSearchHits] = useState<
      { title: string; url: string; snippet: string }[]
    >([]);
  const researchConfig = useQuery({
    queryKey: ["research-config", detail.project.id],
    queryFn: () =>
      api<{ webSearchConfigured: boolean }>(
        `/projects/${detail.project.id}/research/config`,
      ),
  });
  const { working, act } = useStudioAction(notify, refresh);
  const reference =
    q.data?.references.find((r) => r.id === selected) || q.data?.references[0];
  return mode === "references" ? (
    <div className="studio-two-col">
      <StudioCard
        title="隔离的参考书架"
        subtitle="只用于结构与文风分析，参考原文不会进入活动正文或事实索引。"
      >
        <form
          className="studio-form"
          onSubmit={(e) => {
            e.preventDefault();
            void act(async () => {
              const r = await api<Reference>(base + "/references", "POST", {
                title,
                text,
                sourceNote: note,
              });
              setSelected(r.id);
              setTitle("");
              setText("");
            }, "参考资料已隔离导入并分析。");
          }}
        >
          <label>
            资料名
            <input
              aria-label="参考资料名"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              required
            />
          </label>
          <label>
            来源与使用说明
            <input
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="作者自有材料、已授权片段等"
            />
          </label>
          <label className="studio-file">
            <Upload size={16} /> 导入文本文件
            <input
              type="file"
              accept=".txt,.md"
              onChange={async (e) => {
                const file = e.target.files?.[0];
                if (file) {
                  setText(await file.text());
                  if (!title) setTitle(file.name);
                }
              }}
            />
          </label>
          <label>
            参考文本
            <textarea
              aria-label="参考资料正文"
              rows={8}
              value={text}
              onChange={(e) => setText(e.target.value)}
              required
            />
          </label>
          <StudioButton primary type="submit" disabled={working}>
            分析并保存资料
          </StudioButton>
        </form>
        <div className="studio-list">
          {q.data?.references.map((r) => (
            <button
              key={r.id}
              className={reference?.id === r.id ? "selected" : ""}
              onClick={() => setSelected(r.id)}
            >
              <BookOpen size={16} />
              <span>
                <strong>{r.title}</strong>
                <small>
                  {r.analysis.wordCount} 字 · {r.analysis.paragraphCount} 段
                </small>
              </span>
            </button>
          ))}
        </div>
        <StudioError error={q.error} />
      </StudioCard>
      <div>
        {reference && (
          <StudioCard title={reference.title} subtitle={reference.sourceNote}>
            <div className="studio-metrics">
              <div>
                <strong>{reference.analysis.wordCount}</strong>
                <span>字数</span>
              </div>
              <div>
                <strong>{reference.analysis.paragraphCount}</strong>
                <span>段落</span>
              </div>
              <div>
                <strong>
                  {(reference.analysis.dialogueRatio * 100).toFixed(0)}%
                </strong>
                <span>对白比例</span>
              </div>
            </div>
            <h4>结构拆解</h4>
            {reference.analysis.structure?.map((s, i) => (
              <p className="studio-structure" key={i}>
                <span>{i + 1}</span>
                {s.title}
                <small>第 {s.paragraph} 段</small>
              </p>
            ))}
            <h4>文风画像</h4>
            <Result value={reference.analysis.writingProfile} />
            <StudioButton
              disabled={working}
              onClick={() =>
                void act(
                  () =>
                    api(`/projects/${detail.project.id}/settings`, "POST", {
                      kind: "style",
                      name: reference.title + " · 文风参考",
                      content: reference.analysis.writingProfile,
                    }),
                  "已保存为可编辑文风设定，不引用参考情节。",
                )
              }
            >
              <Save size={13} /> 保存为作者文风参考
            </StudioButton>
            <button
              className="text-button danger"
              onClick={() =>
                void act(
                  () => api(base + "/references/" + reference.id, "DELETE"),
                  "参考资料已删除",
                )
              }
            >
              <Trash2 size={12} /> 删除隔离资料
            </button>
          </StudioCard>
        )}
        <StudioCard
          title="公开资料研究"
          subtitle="明确选择公开网页进行抓取，内容只进入隔离参考库。"
        >
          <div className="studio-form">
            <label>
              研究主题
              <input
                value={researchTopic}
                onChange={(e) => setResearchTopic(e.target.value)}
                placeholder="题材趋势、历史背景、公开榜单…"
              />
            </label>
            <label>
              公开 URL（最多 5 条，每行一条）
              <textarea
                rows={4}
                value={urls}
                onChange={(e) => setUrls(e.target.value)}
              />
            </label>
            <StudioButton
              disabled={working || !urls.trim()}
              onClick={() =>
                void act(
                  async () =>
                    setResearchResult(
                      await api(
                        `/projects/${detail.project.id}/research`,
                        "POST",
                        {
                          topic: researchTopic,
                          urls: urls
                            .split("\n")
                            .map((s) => s.trim())
                            .filter(Boolean),
                        },
                      ),
                    ),
                  "研究结果已保存；逐项显示抓取成功或失败。",
                )
              }
            >
              <Search size={13} /> 抓取选定公开网页
            </StudioButton>
            {researchConfig.data?.webSearchConfigured ? (
              <>
                <StudioButton
                  disabled={working || !researchTopic}
                  onClick={() =>
                    void act(async () =>
                      setSearchHits(
                        await api(
                          `/projects/${detail.project.id}/research/search`,
                          "POST",
                          { query: researchTopic },
                        ),
                      ),
                    )
                  }
                >
                  搜索公开资料候选
                </StudioButton>
                {searchHits.map((hit) => (
                  <div className="studio-inset" key={hit.url}>
                    <strong>{hit.title}</strong>
                    <p className="muted small">{hit.snippet}</p>
                    <StudioButton
                      onClick={() =>
                        setUrls((old) =>
                          [
                            ...new Set([
                              ...old.split("\n").filter(Boolean),
                              hit.url,
                            ]),
                          ].join("\n"),
                        )
                      }
                    >
                      选择此 URL
                    </StudioButton>
                  </div>
                ))}
              </>
            ) : (
              <p className="muted small">
                未配置网页搜索服务，可直接填写公开 URL
                抓取。不会把未执行的搜索显示为结果。
              </p>
            )}
            <Result value={researchResult} />
            <StudioButton
              disabled={
                working || !detail.chapters.some((c) => c.currentRevisionId)
              }
              onClick={() =>
                void act(async () =>
                  setResearchResult(
                    await api(
                      `/projects/${detail.project.id}/studio/references/overlap?revisionId=${detail.chapters.find((c) => c.currentRevisionId)?.currentRevisionId}`,
                    ),
                  ),
                )
              }
            >
              检查当前稿与参考原文重合
            </StudioButton>
          </div>
        </StudioCard>
        <StudioCard title="只在参考资料中查找">
          <div className="studio-form">
            <label>
              关键词
              <input value={term} onChange={(e) => setTerm(e.target.value)} />
            </label>
            <StudioButton
              onClick={() =>
                void act(async () =>
                  setResults(
                    await api(
                      base + "/references/search?q=" + encodeURIComponent(term),
                    ),
                  ),
                )
              }
            >
              <Search size={13} /> 检索参考
            </StudioButton>
            <Result value={results} />
          </div>
        </StudioCard>
      </div>
    </div>
  ) : (
    <div className="wiki-grid">
      {wiki.data?.map((entity) => (
        <StudioCard
          key={entity.entityId}
          title={entity.name}
          subtitle={entity.kind === "character" ? "人物百科" : "世界设定"}
        >
          <p className="wiki-notes">{entity.authorNotes}</p>
          {entity.claims?.map((claim, i) => (
            <div className="wiki-claim" key={i}>
              <strong>
                {claim.predicate} · {claim.object}
              </strong>
              <SourceList sources={claim.supporting} label="支持证据" />
              <SourceList sources={claim.opposing} label="冲突证据" />
            </div>
          ))}
          {!!entity.beliefs?.length && (
            <details>
              <summary>角色记忆投影 · {entity.beliefs.length}</summary>
              {entity.beliefs.map((belief, i) => (
                <div className="studio-inset" key={i}>
                  <span className="pill">
                    召回权重 {belief.recallWeight.toFixed(2)} ·{" "}
                    {belief.forgottenProjection ? "已淡出投影" : "可见"}
                  </span>
                  <Result value={belief.fact} />
                </div>
              ))}
            </details>
          )}
        </StudioCard>
      ))}
      <StudioError error={wiki.error} />
      {!wiki.data?.length && (
        <StudioCard title="百科等待有来源的故事">
          <p className="muted">
            先添加人物与世界设定，再确认章节事实。百科会按实体归集来源、冲突证据和角色信念。
          </p>
        </StudioCard>
      )}
    </div>
  );
}
function SourceList({ sources, label }: { sources: Source[]; label: string }) {
  const [open, setOpen] = useState(""),
    [body, setBody] = useState("");
  return (
    <div className="wiki-sources">
      {!!sources?.length && <span className="section-label">{label}</span>}
      {sources?.map((s, i) => (
        <div key={i}>
          <blockquote>
            <Quote size={12} />
            {s.quote}
          </blockquote>
          <button
            className="text-button"
            onClick={() =>
              void api<{ plainText: string }>(
                `/revisions/${s.revisionId}`,
              ).then((r) => {
                setOpen(s.revisionId);
                setBody(r.plainText);
              })
            }
          >
            <Link2 size={11} /> 第 {s.chapterOrder} 章来源
          </button>
          {open === s.revisionId && <pre className="studio-result">{body}</pre>}
        </div>
      ))}
    </div>
  );
}
