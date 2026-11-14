import { describe, expect, it } from "vitest";
import type { Chapter } from "./api";
import {
  bookProgress,
  chapterHasText,
  parseRecentWriting,
} from "./bookProgress";

function chapter(
  id: string,
  order: number,
  props: Partial<Chapter> = {},
): Chapter {
  return {
    id,
    projectId: "book",
    order,
    title: id,
    volume: "",
    blueprint: "",
    blueprintVersion: 0,
    currentRevisionId: "",
    hasText: false,
    finalRevisionId: "",
    needsReview: false,
    deleted: false,
    ...props,
  };
}

describe("book writing progress", () => {
  it("resumes the author's valid last chapter, including a newly prepared empty chapter", () => {
    const progress = bookProgress(
      "book",
      [
        chapter("first", 1, { currentRevisionId: "r1", hasText: true }),
        chapter("second", 2),
      ],
      { projectId: "book", chapterId: "second" },
    );
    expect(progress.resume?.id).toBe("second");
    expect(progress.resumeIsRecent).toBe(true);
  });
  it("ignores stale or foreign recent pointers and resumes the last written chapter in book order", () => {
    const input = [
      chapter("third", 3),
      chapter("second", 2, { currentRevisionId: "r2", hasText: true }),
      chapter("first", 1, { currentRevisionId: "r1", hasText: true }),
      chapter("deleted", 4, {
        deleted: true,
        currentRevisionId: "r4",
        hasText: true,
      }),
      chapter("foreign", 9, {
        projectId: "another",
        currentRevisionId: "rx",
        hasText: true,
      }),
    ];
    for (const recent of [
      { projectId: "book", chapterId: "deleted" },
      { projectId: "another", chapterId: "foreign" },
      undefined,
    ]) {
      const progress = bookProgress("book", input, recent);
      expect(progress.resume?.id).toBe("second");
      expect(progress.resumeIsRecent).toBe(false);
      expect(progress.chapters.map((item) => item.id)).toEqual([
        "first",
        "second",
        "third",
      ]);
    }
    expect(input[0].id).toBe("third");
  });
  it("counts final chapters only when the final version still matches the current saved version", () => {
    const progress = bookProgress("book", [
      chapter("matched", 1, {
        currentRevisionId: "r1",
        finalRevisionId: "r1",
        hasText: true,
      }),
      chapter("edited", 2, {
        currentRevisionId: "r2-new",
        hasText: true,
        finalRevisionId: "r2",
      }),
      chapter("review", 3, {
        currentRevisionId: "r3",
        needsReview: true,
        hasText: true,
      }),
      chapter("empty", 4),
    ]);
    expect(progress.written).toHaveLength(3);
    expect(progress.final.map((item) => item.id)).toEqual(["matched"]);
    expect(progress.review.map((item) => item.id)).toEqual(["review"]);
  });
  it("opens the first chapter of an unwritten book and handles an empty book", () => {
    expect(
      bookProgress("book", [chapter("second", 2), chapter("first", 1)]).resume
        ?.id,
    ).toBe("first");
    expect(bookProgress("book", []).resume).toBeUndefined();
  });
  it("does not count initial empty revisions as written or resume them ahead of real text", () => {
    const input = [
      chapter("first", 1, { currentRevisionId: "initial-1" }),
      chapter("written", 2, { currentRevisionId: "r2", hasText: true }),
      chapter("unwritten", 3, { currentRevisionId: "initial-3" }),
    ];
    const progress = bookProgress("book", input);
    expect(progress.written.map((item) => item.id)).toEqual(["written"]);
    expect(progress.resume?.id).toBe("written");
    expect(
      bookProgress("book", input, { projectId: "book", chapterId: "unwritten" })
        .resume?.id,
    ).toBe("unwritten");
  });
  it("uses loaded current text only when hasText is absent and never treats a revision ID as text", () => {
    const revision = {
      id: "r",
      chapterId: "chapter",
      parentId: "",
      document: { type: "doc" },
      plainText: "  已保存的正文  ",
      hash: "",
      source: "author",
      runId: "",
      createdAt: "",
    };
    expect(
      chapterHasText(
        chapter("id-only", 1, {
          currentRevisionId: "initial",
          hasText: undefined,
        }),
      ),
    ).toBe(false);
    expect(
      chapterHasText(
        chapter("loaded", 2, {
          currentRevisionId: "r",
          hasText: undefined,
          revision,
        }),
      ),
    ).toBe(true);
    expect(
      chapterHasText(
        chapter("blank", 3, {
          currentRevisionId: "r",
          hasText: undefined,
          revision: { ...revision, plainText: " \n\t　" },
        }),
      ),
    ).toBe(false);
    expect(
      chapterHasText(
        chapter("explicit", 4, {
          currentRevisionId: "r",
          hasText: false,
          revision,
        }),
      ),
    ).toBe(false);
  });
  it("discards malformed local preferences without inventing a resume target", () => {
    for (const value of [
      null,
      "{",
      "null",
      "[]",
      '"book"',
      '{"projectId":"book","chapterId":3}',
    ])
      expect(parseRecentWriting(value)).toBeUndefined();
    expect(
      parseRecentWriting('{"projectId":"book","chapterId":"chapter"}'),
    ).toEqual({ projectId: "book", chapterId: "chapter" });
  });
});
