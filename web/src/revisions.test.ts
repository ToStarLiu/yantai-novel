import { describe, it, expect } from "vitest";
import { paragraphDiff, applyHunks, canApplyCandidate } from "./revisions";
describe("author-selected revision changes", () => {
  it("preserves Chinese text and repeated paragraphs during partial acceptance", () => {
    const a = "林川抬头。\n\n风来了。\n\n林川抬头。";
    const b = "林川抬头。\n\n雨来了。\n\n林川抬头。";
    const diff = paragraphDiff(a, b);
    expect(applyHunks(diff)).toBe(a);
    expect(applyHunks(diff.map((h) => ({ ...h, accepted: true })))).toBe(b);
  });
  it("handles insertion and deletion without adopting unselected changes", () => {
    const h = paragraphDiff("甲。\n\n乙。", "甲。\n\n新段。\n\n乙。");
    expect(applyHunks(h)).toBe("甲。\n\n乙。");
    expect(applyHunks(h.map((x) => ({ ...x, accepted: true })))).toBe(
      "甲。\n\n新段。\n\n乙。",
    );
  });
  it("blocks a stale or differently based candidate", () => {
    expect(canApplyCandidate("r1", "r2", false)).toBe(false);
    expect(canApplyCandidate("r1", "r1", true)).toBe(false);
    expect(canApplyCandidate("r1", "r1", false)).toBe(true);
  });
});

import { candidateDocumentText } from "./revisions";
it("compares a partial rewrite as a complete frozen candidate without losing surrounding prose", () => {
  expect(
    candidateDocumentText({
      type: "rewrite",
      input: { source: "开场。旧句。收尾。", selection: "旧句。" },
      output: { text: "新句。" },
    }),
  ).toBe("开场。新句。收尾。");
  expect(
    candidateDocumentText({
      type: "rewrite",
      input: { source: "旧句。旧句。", selection: "旧句。" },
      output: { text: "新句。" },
    }),
  ).toBeUndefined();
  expect(
    candidateDocumentText({
      type: "revision",
      input: {},
      output: { text: "整章候选" },
    }),
  ).toBe("整章候选");
});
