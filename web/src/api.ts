export type Doc = {
  type: string;
  text?: string;
  attrs?: Record<string, unknown>;
  content?: Doc[];
  marks?: { type: string; attrs?: Record<string, unknown> }[];
};
export type Project = {
  id: string;
  title: string;
  description: string;
  genre: string;
  epoch: number;
  health: string;
  stateVersionId: string;
  createdAt: string;
};
export type Setting = {
  id: string;
  projectId: string;
  kind: string;
  name: string;
  content: string;
};
export type Revision = {
  id: string;
  chapterId: string;
  parentId: string;
  document: Doc;
  plainText: string;
  hash: string;
  source: string;
  runId: string;
  createdAt: string;
};
export type Chapter = {
  id: string;
  projectId: string;
  volume: string;
  title: string;
  order: number;
  blueprint: string;
  blueprintVersion: number;
  currentRevisionId: string;
  finalRevisionId: string;
  needsReview: boolean;
  deleted: boolean;
  revision?: Revision;
};
export type Detail = {
  project: Project;
  chapters: Chapter[];
  settings: Setting[];
};
export type Issue = {
  kind: string;
  severity: string;
  quote: string;
  explanation: string;
  suggestedAction: string;
};
export type Run = {
  id: string;
  projectId: string;
  chapterId: string;
  type: string;
  taskType?: string;
  status: string;
  input: Record<string, unknown>;
  output: { text?: string; issues?: Issue[]; proposalId?: string };
  error?: string;
  stale: boolean;
  usage?: { inputTokens?: number; outputTokens?: number; reported?: boolean };
  costStatus?: string;
  createdAt: string;
  updatedAt?: string;
};
export type TaskAuthorContext = {
  runId: string;
  ownerId: string;
  ignoreMemory: boolean;
  excludedReason: string;
  current: boolean;
  sourceCurrent?: boolean;
  memory?: {
    ownerId: string;
    version: number;
    hash: string;
    enabled: boolean;
    items: {
      id: string;
      scope: "author" | "book";
      kind: string;
      title: string;
      content: string;
      version: number;
    }[];
  };
};
export type Impact = {
  id: string;
  projectId: string;
  earliestOrder: number;
  reason: string;
  chapterIds: string[];
  createdAt: string;
};
export type MemoryChange = {
  id: string;
  kind: string;
  subject: string;
  predicate: string;
  object: string;
  characterId?: string;
  truthStatus?: string;
  quote?: string;
  evidence?: { quote?: string; revisionId?: string; plainTextHash?: string };
  decision?: string;
  [key: string]: unknown;
};
export type Proposal = {
  id: string;
  chapterId: string;
  projectId: string;
  sourceRevisionId: string;
  baseStateId?: string;
  baseStateVersionId?: string;
  epoch: number;
  changes: MemoryChange[];
  status: string;
  createdAt: string;
};
export type Citation = {
  id?: string;
  chapterId: string;
  revisionId: string;
  quote?: string;
  content?: string;
  score?: number;
  title?: string;
};
export type StateView = {
  stateVersionId?: string;
  id?: string;
  health?: string;
  facts?: MemoryChange[];
  states?: unknown[];
  [key: string]: unknown;
};
export type Session = {
  authenticated: boolean;
  csrfToken: string;
  username?: string;
  user?: { id: string; username: string; displayName: string; admin: boolean };
};
export class APIError extends Error {
  status: number;
  currentRevisionId?: string;
  constructor(
    status: number,
    body: { error?: string; currentRevisionId?: string },
  ) {
    super(body.error || "请求未完成");
    this.status = status;
    this.currentRevisionId = body.currentRevisionId;
  }
}
let csrf = "";
export function setCSRF(value: string) {
  csrf = value;
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch("/api" + path, {
    method,
    credentials: "same-origin",
    headers: {
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
      ...(method !== "GET" ? { "X-CSRF-Token": csrf } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    let data = { error: `请求失败 (${response.status})` };
    try {
      data = await response.json();
    } catch {}
    throw new APIError(response.status, data);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}
export const id = () => crypto.randomUUID();
export function documentFromText(text: string): Doc {
  return {
    type: "doc",
    content: text.split(/\n\s*\n/).map((p) => ({
      type: "paragraph",
      ...(p ? { content: [{ type: "text", text: p }] } : {}),
    })),
  };
}
export function plainText(doc: Doc): string {
  if (doc.type === "text") return doc.text || "";
  if (doc.type === "hardBreak") return "\n";
  const inner = (doc.content || []).map(plainText);
  return inner.join(
    doc.type === "doc" ||
      doc.type === "bulletList" ||
      doc.type === "orderedList"
      ? "\n\n"
      : "",
  );
}
export function dateTime(value?: string) {
  return value
    ? new Date(value).toLocaleString("zh-CN", {
        month: "numeric",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "";
}
export function message(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
export const busy = (run: Run) =>
  ["Pending", "Running", "RetryScheduled"].includes(run.status);
export const statusLabel: Record<string, string> = {
  Pending: "等待执行",
  Running: "正在生成",
  Succeeded: "已完成",
  Failed: "执行失败",
  Cancelled: "已取消",
  Paused: "已暂停",
  RetryScheduled: "等待重试",
  WaitingForUser: "等待审核",
  NeedsReconciliation: "结果待核对",
  Ready: "记忆就绪",
  MemoryPending: "记忆待确认",
  RebuildRequired: "记忆待重建",
  Applied: "已提交",
  Proposed: "待审核",
};
