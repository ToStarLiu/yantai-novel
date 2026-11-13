import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { X, Quote, Check, Plus } from "lucide-react";
import { api, type Revision } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Comment = {
  id: string;
  version: number;
  revisionId: string;
  quote: string;
  text: string;
  resolved: boolean;
  anchorStatus: string;
  createdAt: string;
};
export default function CommentPanel({
  detail,
  chapterId,
  notify,
  refresh,
  onClose,
}: PanelProps & { onClose: () => void }) {
  const [text, setText] = useState(""),
    [quote, setQuote] = useState(""),
    [showResolved, setShowResolved] = useState(false);
  const chapter = detail.chapters.find((c) => c.id === chapterId);
  const revision = useQuery({
    queryKey: [
      "studio",
      detail.project.id,
      "comment-revision",
      chapter?.currentRevisionId,
    ],
    queryFn: () => api<Revision>("/revisions/" + chapter?.currentRevisionId),
    enabled: !!chapter?.currentRevisionId,
  });
  const comments = useQuery({
    queryKey: ["studio", detail.project.id, "comments"],
    queryFn: () =>
      api<{ comments: Comment[] }>(`/projects/${detail.project.id}/studio`),
  });
  const { working, act } = useStudioAction(notify, refresh);
  return (
    <aside className="studio-comments">
      <div className="row-between">
        <div>
          <span className="eyebrow">AUTHOR ANNOTATIONS</span>
          <h3>本章批注</h3>
        </div>
        <button className="icon-button" onClick={onClose} aria-label="关闭批注">
          <X size={18} />
        </button>
      </div>
      <p className="muted small">
        批注绑定具体正文版本。重复引文无法唯一定位时会保留“需定位”提示。
      </p>
      <form
        className="studio-form"
        onSubmit={(e) => {
          e.preventDefault();
          void act(async () => {
            await api(
              `/projects/${detail.project.id}/studio/comments`,
              "POST",
              {
                revisionId: chapter?.currentRevisionId || "",
                quote,
                prefix: "",
                suffix: "",
                text,
              },
            );
            setText("");
            setQuote("");
          }, "批注已保存并关联正文版本。");
        }}
      >
        <label>
          引用正文
          <textarea
            rows={2}
            value={quote}
            onChange={(e) => setQuote(e.target.value)}
            placeholder="逐字引用本章需要推敲的一句"
            required
          />
        </label>
        <label>
          批注
          <textarea
            rows={3}
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="动机、伏笔、节奏或下一轮修订提醒…"
            required
          />
        </label>
        <StudioButton
          primary
          type="submit"
          disabled={working || !chapter?.currentRevisionId}
        >
          <Plus size={13} /> 添加批注
        </StudioButton>
      </form>
      <details className="comment-source">
        <summary>查看当前版本正文</summary>
        <pre>{revision.data?.plainText}</pre>
      </details>
      <label className="check-label">
        <input
          type="checkbox"
          checked={showResolved}
          onChange={(e) => setShowResolved(e.target.checked)}
        />{" "}
        包含已解决批注
      </label>
      {comments.data?.comments
        .filter(
          (c) =>
            c.revisionId === chapter?.currentRevisionId &&
            (showResolved || !c.resolved),
        )
        .map((c) => (
          <article className="studio-inset" key={c.id}>
            <span className="pill">
              {c.resolved
                ? "已解决"
                : c.anchorStatus === "Located"
                  ? "已定位"
                  : "需定位"}
            </span>
            <blockquote>
              <Quote size={13} />
              {c.quote}
            </blockquote>
            <p>{c.text}</p>
            <StudioButton
              disabled={working}
              onClick={() =>
                void act(
                  () =>
                    api(
                      `/projects/${detail.project.id}/studio/comments/${c.id}`,
                      "PUT",
                      {
                        expectedVersion: c.version,
                        text: c.text,
                        resolved: !c.resolved,
                      },
                    ),
                  "批注状态已更新。",
                )
              }
            >
              <Check size={13} />
              {c.resolved ? "重新打开" : "标记解决"}
            </StudioButton>
          </article>
        ))}
      <StudioError error={comments.error} />
    </aside>
  );
}
