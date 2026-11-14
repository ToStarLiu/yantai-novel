export const authorMemoryTaskExclusions: Record<string, string> = {
  review: "一致性审稿只依据正文和事实证据，不加载作者偏好。",
  "memory-extraction":
    "事实提取不会加载作者偏好，避免把文风和想法写成故事事实。",
  analysis: "资料与因果分析只依据当前来源，不加载作者偏好。",
  voice: "声音分析依据作品原文，不用已有文风偏好替代分析。",
  handoff: "场景交接检查依据原文和约束，不加载作者偏好。",
};
export function authorMemoryTaskExclusion(taskType?: string): string {
  return taskType ? authorMemoryTaskExclusions[taskType] || "" : "";
}
export function effectiveTaskMemory(
  settings: { useMemory: boolean } | undefined,
  localChoice: boolean | null,
  unavailable = false,
): boolean {
  return !unavailable && settings?.useMemory === true && localChoice !== false;
}
export function privateTaskContextAccess(
  ownerId: unknown,
  authenticatedUserId?: string,
): "owner" | "other" | "unknown" {
  if (typeof ownerId !== "string" || !ownerId || !authenticatedUserId)
    return "unknown";
  return ownerId === authenticatedUserId ? "owner" : "other";
}
