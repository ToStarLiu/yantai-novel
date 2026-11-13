import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Download,
  Printer,
  Image,
  Volume2,
  Sparkles,
  Eye,
  BookOpen,
  Languages,
  Check,
} from "lucide-react";
import {
  api,
  id,
  busy,
  statusLabel,
  type Run,
  type Revision,
  documentFromText,
  type Chapter,
} from "../api";
import { paragraphDiff } from "../revisions";
import {
  StudioCard,
  StudioButton,
  StudioError,
  Result,
  ChapterSelect,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Media = {
  id: string;
  kind: string;
  status: string;
  mime: string;
  error: string;
  costStatus: string;
  createdAt: string;
};
type CandidateGroup = {
  id: string;
  chapterId: string;
  status: string;
  runIds: string[];
  rankings: {
    runId: string;
    score: number;
    issues: unknown[];
    explanation: string;
  }[];
};
export default function PublishingPanel({
  detail,
  chapterId,
  notify,
  refresh,
}: PanelProps) {
  const base = `/projects/${detail.project.id}`;
  const media = useQuery({
    queryKey: ["studio", detail.project.id, "media"],
    queryFn: () => api<Media[]>(base + "/media"),
    refetchInterval: 3000,
  });
  const config = useQuery({
    queryKey: ["media-config"],
    queryFn: () =>
      api<{
        configured: boolean;
        imageModel: string;
        speechModel: string;
        maxSpeechCharacters: number;
      }>("/media/config"),
  });
  const groups = useQuery({
    queryKey: ["studio", detail.project.id, "candidates"],
    queryFn: () => api<CandidateGroup[]>(base + "/candidates"),
    refetchInterval: 3000,
  });
  const runs = useQuery({
    queryKey: ["studio", detail.project.id, "publishing-runs"],
    queryFn: () => api<Run[]>(base + "/runs"),
    refetchInterval: 3000,
  });
  const [tab, setTab] = useState("export"),
    [author, setAuthor] = useState(""),
    [language, setLanguage] = useState("zh-CN"),
    [finalOnly, setFinalOnly] = useState(true),
    [synopsis, setSynopsis] = useState(true),
    [target, setTarget] = useState(chapterId),
    [count, setCount] = useState(2),
    [instructions, setInstructions] = useState(""),
    [selectedRun, setSelectedRun] = useState(""),
    [mediaKind, setMediaKind] = useState("cover"),
    [prompt, setPrompt] = useState(""),
    [model, setModel] = useState(""),
    [voice, setVoice] = useState("alloy"),
    [size, setSize] = useState("1024x1536");
  const [voiceName, setVoiceName] = useState("作者声音档案"),
    [voiceContent, setVoiceContent] = useState("");
  const { working, act } = useStudioAction(notify, refresh);
  const selected = runs.data?.find((r) => r.id === selectedRun);
  const chapter = detail.chapters.find((c) => c.id === target);
  const baseline = useQuery({
    queryKey: [
      "studio",
      detail.project.id,
      "candidate-baseline",
      selected?.chapterId,
    ],
    queryFn: () => api<Chapter>(`/chapters/${selected!.chapterId}`),
    enabled: !!selected?.chapterId,
  });
  const downloadPublication = async (format: string) => {
    const response = await fetch(
      `/api/projects/${detail.project.id}/publish/${format}?author=${encodeURIComponent(author)}&language=${encodeURIComponent(language)}&useFinal=${finalOnly}&includeSynopsis=${synopsis}`,
      { credentials: "same-origin" },
    );
    if (!response.ok) {
      let error = "出版导出失败";
      try {
        error = (await response.json()).error || error;
      } catch {}
      throw new Error(error);
    }
    const url = URL.createObjectURL(await response.blob());
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `${detail.project.title}.${format}`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <>
      <div className="studio-subtabs">
        {[
          ["export", "成书导出"],
          ["candidates", "高质量多候选"],
          ["editorial", "虚拟编辑部"],
          ["media", "封面与有声"],
        ].map(([v, l]) => (
          <button
            className={tab === v ? "active" : ""}
            onClick={() => setTab(v)}
            key={v}
          >
            {l}
          </button>
        ))}
      </div>
      {tab === "export" ? (
        <div className="studio-two-col">
          <StudioCard
            title="把故事装订成书"
            subtitle="导出当前可读正文；完整版本迁移仍使用作品包。"
          >
            <div className="studio-form">
              <label>
                署名
                <input
                  value={author}
                  onChange={(e) => setAuthor(e.target.value)}
                />
              </label>
              <label>
                语言
                <input
                  value={language}
                  onChange={(e) => setLanguage(e.target.value)}
                />
              </label>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={finalOnly}
                  onChange={(e) => setFinalOnly(e.target.checked)}
                />{" "}
                优先使用已定稿正文
              </label>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={synopsis}
                  onChange={(e) => setSynopsis(e.target.checked)}
                />{" "}
                附作品简介
              </label>
              <div className="publish-formats">
                {[
                  ["docx", "Word 文稿"],
                  ["epub", "EPUB 电子书"],
                  ["pdf", "PDF 阅读版"],
                  ["html", "HTML 网页"],
                ].map(([format, label]) => (
                  <button
                    key={format}
                    onClick={() => void act(() => downloadPublication(format))}
                  >
                    <Printer size={24} />
                    <strong>{label}</strong>
                    <span>
                      {format.toUpperCase()} <Download size={12} />
                    </span>
                  </button>
                ))}
              </div>
            </div>
          </StudioCard>
          <StudioCard title="从创作到出版">
            <p className="muted">
              出版文件按照卷章结构整理，保留中文标题与段落。封面、声音与出版文案都应在作者审核后使用。
            </p>
            <p className="muted small">
              导出的电子书不会包含密钥、登录会话或可继续执行的模型任务。
            </p>
          </StudioCard>
        </div>
      ) : tab === "candidates" ? (
        <div className="studio-two-col">
          <StudioCard
            title="多候选质量模式"
            subtitle="独立生成 2–3 份候选，再逐份审稿和排序；文学判断仍归作者。"
          >
            <div className="studio-form">
              <ChapterSelect
                chapters={detail.chapters}
                value={target}
                onChange={setTarget}
              />
              <label>
                候选数
                <select
                  value={count}
                  onChange={(e) => setCount(Number(e.target.value))}
                >
                  <option value="2">2 份独立候选</option>
                  <option value="3">3 份独立候选</option>
                </select>
              </label>
              <label>
                本轮方向
                <textarea
                  rows={4}
                  value={instructions}
                  onChange={(e) => setInstructions(e.target.value)}
                />
              </label>
              <StudioButton
                primary
                disabled={working || !target}
                onClick={() =>
                  void act(
                    () =>
                      api(`/chapters/${target}/candidates`, "POST", {
                        count,
                        inputRevisionId: chapter?.currentRevisionId || "",
                        instructions,
                      }),
                    "多候选任务已创建，生成与审稿均计入用量。",
                  )
                }
              >
                <Sparkles size={14} /> 生成并评价候选
              </StudioButton>
              <p className="muted small">
                每份候选独立保存；评分侧重可核对的一致性问题，不代表小说文学质量保证。
              </p>
            </div>
            {groups.data?.map((g) => (
              <div className="studio-inset" key={g.id}>
                <span className="pill">
                  {statusLabel[g.status] || g.status}
                </span>
                <h4>候选组 {g.id.slice(0, 8)}</h4>
                {g.rankings?.map((r, i) => (
                  <button
                    className="candidate-ranking"
                    key={r.runId}
                    onClick={() => setSelectedRun(r.runId)}
                  >
                    <span>{i + 1}</span>
                    <strong>候选 {r.runId.slice(0, 6)}</strong>
                    <small>
                      {r.score} 分 · {r.issues?.length || 0} 项问题
                    </small>
                  </button>
                ))}
                {!g.rankings?.length &&
                  g.runIds?.map((rid, i) => (
                    <button
                      className="text-button"
                      key={rid}
                      onClick={() => setSelectedRun(rid)}
                    >
                      查看候选 {i + 1}
                    </button>
                  ))}
              </div>
            ))}
          </StudioCard>
          <StudioCard title="候选比较与采用">
            {selected ? (
              <>
                <span className="pill">
                  {statusLabel[selected.status] || selected.status}
                </span>
                <Result value={selected.output.text} />
                <details>
                  <summary>对照原稿与候选差异</summary>
                  {paragraphDiff(
                    baseline.data?.revision?.plainText || "",
                    selected.output.text || "",
                  ).map((h) => (
                    <div className="candidate-diff" key={h.id}>
                      {h.changed && h.before && <del>{h.before}</del>}
                      {h.changed ? <ins>{h.after}</ins> : <p>{h.after}</p>}
                    </div>
                  ))}
                </details>
                {groups.data
                  ?.flatMap((g) => g.rankings || [])
                  .filter((r) => r.runId === selected.id)
                  .map((r) => (
                    <div className="studio-inset" key={r.runId}>
                      <strong>一致性评分 {r.score}</strong>
                      <p>{r.explanation}</p>
                      <Result value={r.issues} />
                    </div>
                  ))}
                <StudioButton
                  primary
                  disabled={
                    working || selected.status !== "Succeeded" || selected.stale
                  }
                  onClick={() =>
                    void act(async () => {
                      const current = await api<{ currentRevisionId: string }>(
                        `/chapters/${selected.chapterId}`,
                      );
                      await api<Revision>(
                        `/chapters/${selected.chapterId}/revisions`,
                        "POST",
                        {
                          expectedCurrentRevisionId: current.currentRevisionId,
                          document: documentFromText(
                            selected.output.text || "",
                          ),
                          source: "ai",
                          runId: selected.id,
                        },
                      );
                    }, "候选已采用为新的正文版本；原稿与其他候选保留。")
                  }
                >
                  <Check size={14} /> 采用这份候选为新版本
                </StudioButton>
              </>
            ) : (
              <p className="muted">选择左侧候选查看完整正文和排序依据。</p>
            )}
          </StudioCard>
        </div>
      ) : tab === "editorial" ? (
        <div className="studio-two-col">
          <StudioCard
            title="虚拟编辑部"
            subtitle="以明确职责提供建议，不让一个万能角色修改所有内容。"
          >
            <div className="studio-form">
              <ChapterSelect
                chapters={detail.chapters}
                value={target}
                onChange={setTarget}
              />
              <label>
                本次要求
                <textarea
                  rows={4}
                  value={instructions}
                  onChange={(e) => setInstructions(e.target.value)}
                  placeholder="目标读者、出版平台、翻译语言与需要保留的作者声音…"
                />
              </label>
              <div className="editorial-roles">
                <button
                  disabled={working || !target}
                  onClick={() =>
                    void act(async () => {
                      const run = await api<Run>(
                        `/chapters/${target}/hook-review`,
                        "POST",
                        {
                          intentId: id(),
                          inputRevisionId: chapter?.currentRevisionId || "",
                          instructions,
                        },
                      );
                      setSelectedRun(run.id);
                    }, "章末钩子专项审稿已创建。")
                  }
                >
                  <BookOpen size={18} />
                  <span>
                    <strong>章末钩子</strong>
                    <small>悬念、回报与继续阅读动力</small>
                  </span>
                  <Sparkles size={14} />
                </button>
                {[
                  ["reader", "第一读者", "阅读体验、困惑与情绪回报"],
                  ["developmental", "发展编辑", "结构、人物动机与节奏"],
                  ["voice", "作者声音", "提炼声音与表达边界"],
                  ["publisher", "出版策划", "简介、卖点与投稿资料"],
                  ["translation", "翻译编辑", "按指定语言翻译，保留术语"],
                ].map(([type, name, description]) => (
                  <button
                    disabled={working || !target}
                    key={type}
                    onClick={() =>
                      void act(async () => {
                        const run = await api<Run>(
                          `/chapters/${target}/runs`,
                          "POST",
                          {
                            intentId: id(),
                            taskType: type,
                            inputRevisionId: chapter?.currentRevisionId || "",
                            instructions,
                          },
                        );
                        setSelectedRun(run.id);
                      }, name + "任务已创建")
                    }
                  >
                    <BookOpen size={18} />
                    <span>
                      <strong>{name}</strong>
                      <small>{description}</small>
                    </span>
                    <Sparkles size={14} />
                  </button>
                ))}
              </div>
            </div>
          </StudioCard>
          <StudioCard title="编辑建议与资料">
            {selected ? (
              <>
                <span className="pill">
                  {statusLabel[selected.status] || selected.status}
                </span>
                <Result value={selected.output.text || selected.error} />
                {selected.type === "voice" &&
                  selected.status === "Succeeded" && (
                    <div className="studio-form">
                      <label>
                        声音档案名称
                        <input
                          value={voiceName}
                          onChange={(e) => setVoiceName(e.target.value)}
                        />
                      </label>
                      <label>
                        作者审核后的声音约束
                        <textarea
                          rows={7}
                          value={voiceContent || selected.output.text || ""}
                          onChange={(e) => setVoiceContent(e.target.value)}
                        />
                      </label>
                      <StudioButton
                        primary
                        disabled={working || selected.stale}
                        onClick={() =>
                          void act(
                            () =>
                              api(base + "/studio/voice/apply", "POST", {
                                runId: selected.id,
                                name: voiceName,
                                content: voiceContent || selected.output.text,
                              }),
                            "声音候选已采用为作品文风档案。",
                          )
                        }
                      >
                        <Check size={13} /> 采用为作者声音档案
                      </StudioButton>
                    </div>
                  )}
              </>
            ) : (
              <p className="muted">
                从左侧选择一项明确任务。所有意见以独立产物保存，作者决定是否使用。
              </p>
            )}
            <div className="studio-list">
              {runs.data
                ?.filter((r) =>
                  [
                    "reader",
                    "developmental",
                    "voice",
                    "publisher",
                    "translation",
                  ].includes(r.type),
                )
                .map((r) => (
                  <button key={r.id} onClick={() => setSelectedRun(r.id)}>
                    <span>
                      <strong>{r.type}</strong>
                      <small>{statusLabel[r.status] || r.status}</small>
                    </span>
                  </button>
                ))}
            </div>
          </StudioCard>
        </div>
      ) : (
        <div className="studio-two-col">
          <StudioCard
            title="为故事增加封面与声音"
            subtitle="只有明确提交才向配置的媒体服务请求，费用未知会如实保留。"
          >
            <div className="studio-form">
              <label>
                媒体类型
                <select
                  value={mediaKind}
                  onChange={(e) => {
                    setMediaKind(e.target.value);
                    setModel("");
                  }}
                >
                  <option value="cover">封面图像</option>
                  <option value="tts">章节有声</option>
                </select>
              </label>
              {mediaKind === "tts" && (
                <ChapterSelect
                  chapters={detail.chapters}
                  value={target}
                  onChange={setTarget}
                />
              )}
              <label>
                {mediaKind === "cover" ? "封面创意与美术方向" : "朗读要求"}
                <textarea
                  rows={5}
                  value={prompt}
                  onChange={(e) => setPrompt(e.target.value)}
                  placeholder={
                    mediaKind === "cover"
                      ? "构图、意象、色彩、气氛；留出书名空间…"
                      : "语速、语气、人物声音边界…"
                  }
                />
              </label>
              <label>
                模型
                <input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder={
                    (mediaKind === "cover"
                      ? config.data?.imageModel
                      : config.data?.speechModel) || "填写媒体服务支持的模型"
                  }
                />
              </label>
              {mediaKind === "tts" ? (
                <label>
                  声音
                  <input
                    value={voice}
                    onChange={(e) => setVoice(e.target.value)}
                  />
                </label>
              ) : (
                <label>
                  尺寸
                  <select
                    value={size}
                    onChange={(e) => setSize(e.target.value)}
                  >
                    <option>1024x1536</option>
                    <option>1024x1024</option>
                    <option>1536x1024</option>
                  </select>
                </label>
              )}
              <StudioButton
                primary
                disabled={working || (mediaKind === "tts" && !target)}
                onClick={() =>
                  void act(
                    () =>
                      api(base + "/media", "POST", {
                        kind: mediaKind,
                        chapterId: mediaKind === "tts" ? target : "",
                        prompt,
                        model:
                          model ||
                          (mediaKind === "cover"
                            ? config.data?.imageModel
                            : config.data?.speechModel) ||
                          "",
                        voice,
                        size,
                        instructions: prompt,
                      }),
                    "媒体任务已提交，可在右侧查看进度。",
                  )
                }
              >
                <Sparkles size={14} /> 明确提交
                {mediaKind === "cover" ? "封面生成" : "有声生成"}
              </StudioButton>
              <StudioError error={config.error} />
            </div>
          </StudioCard>
          <StudioCard title="媒体产物">
            {media.data?.map((asset) => (
              <article className="studio-inset" key={asset.id}>
                <div className="row-between">
                  <strong>
                    {asset.kind === "cover" ? "封面候选" : "章节音频"}
                  </strong>
                  <span className="pill">
                    {statusLabel[asset.status] || asset.status}
                  </span>
                </div>
                {asset.status === "Succeeded" &&
                  (asset.kind === "cover" ? (
                    <img
                      className="studio-cover"
                      alt="作品封面候选"
                      src={`/api/projects/${detail.project.id}/media/${asset.id}/content`}
                    />
                  ) : (
                    <audio
                      controls
                      src={`/api/projects/${detail.project.id}/media/${asset.id}/content`}
                    />
                  ))}
                {asset.error && <p className="error-box">{asset.error}</p>}
                <p className="muted small">
                  费用{" "}
                  {asset.costStatus === "Unknown" ? "未知" : asset.costStatus}
                </p>
                {asset.status === "Succeeded" && (
                  <a
                    className="text-button"
                    href={`/api/projects/${detail.project.id}/media/${asset.id}/content`}
                    download
                  >
                    <Download size={13} /> 下载产物
                  </a>
                )}
                {asset.status === "Pending" && (
                  <StudioButton
                    onClick={() =>
                      void act(
                        () =>
                          api(
                            base + "/media/" + asset.id + "/cancel",
                            "POST",
                            {},
                          ),
                        "待执行媒体任务已取消。",
                      )
                    }
                  >
                    取消
                  </StudioButton>
                )}
              </article>
            ))}
            <StudioError error={media.error} />
          </StudioCard>
        </div>
      )}
    </>
  );
}
