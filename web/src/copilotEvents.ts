export type CopilotSession = {
  id: string;
  projectId: string;
  ownerId: string;
  title: string;
  chapterId: string;
  scope?: "book" | "chapter";
  messages?: unknown[];
  version: number;
  createdAt: string;
  updatedAt: string;
};
export type CopilotArtifact = {
  kind:
    | "revision-candidate"
    | "blueprint-proposal"
    | "diff"
    | "book-plan-proposal"
    | "author-memory-proposal";
  memoryId?: string;
  scope?: string;
  status?: string;
  proposalId?: string;
  expectedVersion?: number;
  title: string;
  runId?: string;
  chapterId: string;
  sourceRevisionId: string;
  sourceHash?: string;
  text: string;
  blueprintVersion?: number;
  diff?: {
    id: number;
    before: string;
    after: string;
    changed: boolean;
    accepted: boolean;
  }[];
};
export type CopilotEvent = { id: number; kind: string; data: unknown };
export type CopilotTurn = {
  id: string;
  sessionId: string;
  projectId: string;
  ownerId: string;
  input: {
    text: string;
    chapterId: string;
    inputRevisionId: string;
    selection: string;
    maxSteps: number;
    mode: string;
    pointOfViewCharacterId?: string;
    ignoreMemory?: boolean;
    authorMemory?: {
      enabled: boolean;
      version: number;
      items: {
        id: string;
        scope: string;
        kind: string;
        title: string;
        content: string;
        version: number;
      }[];
    };
    writingSkills?: {
      version: number;
      hash: string;
      items: { id: string; name: string; version: number; hash: string }[];
    };
  };
  status: string;
  text: string;
  error?: string;
  createdAt: string;
  finishedAt?: string;
};
export type CopilotDetail = {
  session: CopilotSession;
  messages: unknown[];
  turns: CopilotTurn[];
};
export type ToolData = {
  name?: string;
  args?: Record<string, unknown>;
  toolCallId?: string;
  callId?: string;
  result?: {
    content?: unknown;
    artifact?: CopilotArtifact;
    isError?: boolean;
    error?: string;
  };
  error?: string;
};
export const copilotBusy = (status: string) =>
  ["Pending", "Running", "Cancelling"].includes(status);
export const toolNames: Record<string, string> = {
  read_book_brief: "读取全书设定",
  read_author_memory: "查阅作者记忆",
  propose_author_memory: "建议记住偏好",
  read_writing_skill: "查阅写作 Skill",
  propose_book_plan: "提出全书规划方案",
  read_settings: "查阅人物与世界设定",
  read_chapter: "读取本章",
  read_selection: "读取当前选段",
  read_blueprint: "查阅本章章纲",
  search_story_memory: "检索故事记忆",
  check_consistency: "检查一致性",
  create_revision_candidate: "生成改写候选",
  propose_blueprint: "提出章纲建议",
  show_diff: "比较正文差异",
};
export const copilotStatus: Record<string, string> = {
  Pending: "等待处理",
  Running: "正在推敲",
  Succeeded: "已完成",
  Cancelled: "已停止",
  Failed: "未完成",
  NeedsReconciliation: "结果待核对",
  Cancelling: "正在停止",
};

// Persisted event IDs make initial replay and reconnect overlap idempotent.
export function mergeCopilotEvents(
  before: CopilotEvent[],
  incoming: CopilotEvent[],
): CopilotEvent[] {
  const events = new Map(before.map((event) => [event.id, event]));
  for (const event of incoming)
    if (Number.isSafeInteger(event.id) && event.id > 0)
      events.set(event.id, event);
  return [...events.values()].sort((a, b) => a.id - b.id);
}
export function eventText(event: CopilotEvent): string {
  if (typeof event.data === "string") return event.data;
  const data = event.data as { text?: string; delta?: string } | undefined;
  return data?.text || data?.delta || "";
}
export type TimelineItem =
  | { kind: "text"; text: string; id: number }
  | { kind: "tool"; data: ToolData; pending: boolean; id: number }
  | { kind: "compaction"; id: number };
export function copilotTimeline(events: CopilotEvent[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  const calls = new Map<string, number>();
  for (const event of mergeCopilotEvents([], events)) {
    if (event.kind === "delta") {
      const text = eventText(event);
      if (!text) continue;
      const last = items.at(-1);
      if (last?.kind === "text") last.text += text;
      else items.push({ kind: "text", text, id: event.id });
    } else if (event.kind === "tool-start" || event.kind === "tool-result") {
      const data = event.data as ToolData;
      const key = data.toolCallId || data.callId || data.name || "";
      const index = calls.get(key);
      if (
        event.kind === "tool-result" &&
        index !== undefined &&
        items[index]?.kind === "tool"
      ) {
        const previous = items[index];
        items[index] = {
          kind: "tool",
          data: { ...previous.data, ...data },
          pending: false,
          id: previous.id,
        };
        calls.delete(key);
      } else {
        if (event.kind === "tool-start") calls.set(key, items.length);
        items.push({
          kind: "tool",
          data,
          pending: event.kind === "tool-start",
          id: event.id,
        });
      }
    } else if (event.kind === "compaction")
      items.push({ kind: "compaction", id: event.id });
  }
  return items;
}
