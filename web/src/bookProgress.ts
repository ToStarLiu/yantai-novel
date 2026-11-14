import type { Chapter } from "./api";

export type RecentWriting = { projectId: string; chapterId: string };
export function parseRecentWriting(
  value: string | null,
): RecentWriting | undefined {
  try {
    const item: unknown = JSON.parse(value || "null");
    if (!item || typeof item !== "object") return;
    const record = item as Record<string, unknown>;
    if (
      typeof record.projectId === "string" &&
      typeof record.chapterId === "string"
    )
      return { projectId: record.projectId, chapterId: record.chapterId };
  } catch {
    /* A stale local preference must not prevent opening a book. */
  }
}

export function chapterHasText(chapter: Chapter): boolean {
  if (typeof chapter.hasText === "boolean") return chapter.hasText;
  return !!chapter.revision?.plainText.trim();
}

export function bookProgress(
  projectId: string,
  allChapters: Chapter[],
  recent?: RecentWriting,
) {
  const chapters = allChapters
    .filter((chapter) => !chapter.deleted && chapter.projectId === projectId)
    .slice()
    .sort((a, b) => a.order - b.order);
  const written = chapters.filter(chapterHasText);
  const final = chapters.filter(
    (chapter) =>
      !!chapter.currentRevisionId &&
      chapter.finalRevisionId === chapter.currentRevisionId,
  );
  const review = chapters.filter((chapter) => chapter.needsReview);
  const lastRead =
    recent?.projectId === projectId
      ? chapters.find((chapter) => chapter.id === recent.chapterId)
      : undefined;
  return {
    chapters,
    written,
    final,
    review,
    resume: lastRead || written.at(-1) || chapters[0],
    resumeIsRecent: !!lastRead,
  };
}
