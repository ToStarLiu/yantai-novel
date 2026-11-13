import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Brain,
  ChevronDown,
  LoaderCircle,
  LockKeyhole,
  ShieldCheck,
} from "lucide-react";
import {
  api,
  APIError,
  message,
  type Run,
  type Session,
  type TaskAuthorContext,
} from "./api";
import { openAuthorMemories } from "./authorMemoryEvents";
import type { AuthorMemorySettings } from "./AuthorMemories";
import {
  authorMemoryTaskExclusion,
  effectiveTaskMemory,
  privateTaskContextAccess,
} from "./taskMemoryPolicy";
import "./taskAuthorMemory.css";

export function useTaskAuthorMemory(projectId: string) {
  const settings = useQuery({
    queryKey: ["author-memory-settings"],
    queryFn: () => api<AuthorMemorySettings>("/author-memory-settings"),
    staleTime: 5000,
    retry: false,
  });
  const [choice, setChoice] = useState<boolean | null>(null);
  useEffect(() => setChoice(null), [projectId]);
  const enabled = effectiveTaskMemory(settings.data, choice, settings.isError);
  return {
    enabled,
    ignoreMemory: !enabled,
    loading: settings.isLoading,
    globalEnabled: settings.data?.useMemory === true,
    error: settings.isError,
    setEnabled: setChoice,
    retry: () => void settings.refetch(),
  };
}
export type TaskMemoryPreference = ReturnType<typeof useTaskAuthorMemory>;

export function TaskMemoryControl({
  preference,
  projectId,
  taskType,
  purpose = "本次创作",
  disabled = false,
  note,
  excludedReason,
}: {
  preference: TaskMemoryPreference;
  projectId: string;
  taskType?: string;
  purpose?: string;
  disabled?: boolean;
  note?: string;
  excludedReason?: string;
}) {
  const excluded = authorMemoryTaskExclusion(taskType) || excludedReason;
  if (excluded)
    return (
      <p className="task-memory-exclusion">
        <ShieldCheck size={13} />
        <span>{excluded}</span>
      </p>
    );
  return (
    <div className="task-memory-control" data-testid="task-memory-control">
      <label>
        <input
          type="checkbox"
          aria-label={purpose + "：使用作者记忆"}
          checked={preference.enabled}
          disabled={
            disabled ||
            preference.loading ||
            preference.error ||
            !preference.globalEnabled
          }
          onChange={(event) => preference.setEnabled(event.target.checked)}
        />
        <span>
          <Brain size={13} />
          使用作者记忆
        </span>
        {preference.loading && <LoaderCircle size={12} className="spin" />}
      </label>
      <button
        className="text-button"
        type="button"
        onClick={() => openAuthorMemories({ projectId })}
      >
        {preference.globalEnabled ? "管理偏好" : "记忆设置"}
      </button>
      <p>
        {preference.error
          ? "暂时无法读取记忆设置，本次将不加载作者偏好。"
          : preference.loading
            ? "正在读取你的记忆设置…"
            : !preference.globalEnabled
              ? "作者记忆已全局暂停，可在记忆设置中启用。"
              : preference.enabled
                ? "按本次任务选取你确认的相关偏好，当前要求仍优先。"
                : "本次不加载作者偏好，已保存的记忆保留。"}
      </p>
      {note && <small>{note}</small>}
      {preference.error && (
        <button
          className="text-button"
          type="button"
          onClick={preference.retry}
        >
          重新读取设置
        </button>
      )}
    </div>
  );
}

export function TaskMemoryExclusion({ taskType }: { taskType: string }) {
  const reason = authorMemoryTaskExclusion(taskType);
  return reason ? (
    <p className="task-memory-exclusion">
      <ShieldCheck size={13} />
      <span>{reason}</span>
    </p>
  ) : null;
}

export function TaskMemoryUsage({
  run,
  compact = false,
}: {
  run: Pick<Run, "id" | "input" | "type" | "stale">;
  compact?: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const session = useQuery({
    queryKey: ["session"],
    queryFn: () => api<Session>("/session"),
    retry: false,
  });
  const access = privateTaskContextAccess(
    run.input.ownerId,
    session.data?.user?.id,
  );
  const context = useQuery({
    queryKey: ["run-author-context", run.id, run.stale],
    queryFn: () => api<TaskAuthorContext>(`/runs/${run.id}/author-context`),
    enabled:
      expanded && access !== "other" && session.data?.authenticated === true,
    retry: false,
    refetchOnWindowFocus: expanded,
    staleTime: 0,
    gcTime: 0,
  });
  const excluded = authorMemoryTaskExclusion(run.type);
  if (access === "other")
    return (
      <p className="task-memory-private">
        <LockKeyhole size={12} />
        作者记忆仅任务创建者可见。
      </p>
    );
  const unavailable =
    context.error instanceof APIError &&
    [403, 404].includes(context.error.status);
  const used = context.data;
  return (
    <details
      className={"task-memory-usage" + (compact ? " compact" : "")}
      open={expanded}
      onToggle={(event) => setExpanded(event.currentTarget.open)}
    >
      <summary>
        <Brain size={12} />
        <span>本任务的作者记忆</span>
        <ChevronDown size={12} />
      </summary>
      {expanded && (
        <div>
          {context.isLoading ? (
            <p className="task-memory-muted">
              <LoaderCircle size={12} className="spin" />
              正在读取私人快照…
            </p>
          ) : unavailable ? (
            <p className="task-memory-muted">
              该任务没有可查看的私人快照，或它属于其他作者。
            </p>
          ) : context.isError ? (
            <p className="task-memory-error">
              {message(context.error)}
              <button
                className="text-button"
                type="button"
                onClick={() => void context.refetch()}
              >
                重新读取
              </button>
            </p>
          ) : used ? (
            <>
              {used.current === false && (
                <p className="task-memory-stale">
                  作者记忆已变化，这份候选按历史版本保留；请基于当前要求重新生成。
                </p>
              )}
              {used.current !== false &&
                (used.sourceCurrent === false || run.stale) && (
                  <p className="task-memory-stale">
                    作品来源已变化。这份私人记忆快照仍可回看，候选需要按当前正文与设定重新生成。
                  </p>
                )}
              {excluded ? (
                <p>{excluded}</p>
              ) : used.ignoreMemory ? (
                <p>本任务未使用作者记忆。</p>
              ) : used.excludedReason ? (
                <p>{used.excludedReason}</p>
              ) : !used.memory?.enabled ? (
                <p>提交时作者记忆已关闭。</p>
              ) : !used.memory.items.length ? (
                <p>本任务没有匹配的已确认作者记忆。</p>
              ) : (
                <>
                  <p className="task-memory-muted">
                    本任务冻结了{used.memory.items.length}条偏好 · 记忆版本{" "}
                    {used.memory.version}
                  </p>
                  <ul>
                    {used.memory.items.map((item) => (
                      <li key={item.id}>
                        <strong>{item.title}</strong>
                        <span>
                          {item.scope === "book" ? "仅此书" : "作者通用"} · v
                          {item.version}
                        </span>
                        <p>{item.content}</p>
                      </li>
                    ))}
                  </ul>
                </>
              )}
              <small>
                这份私人输入仅你可见；任务结果和作者采用仍按作品权限处理。
              </small>
            </>
          ) : excluded ? (
            <p>{excluded}</p>
          ) : null}
        </div>
      )}
    </details>
  );
}
