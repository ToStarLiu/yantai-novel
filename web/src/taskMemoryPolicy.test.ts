import { describe, expect, it } from "vitest";
import {
  authorMemoryTaskExclusion,
  effectiveTaskMemory,
  privateTaskContextAccess,
} from "./taskMemoryPolicy";

describe("task memory privacy and use policy", () => {
  it("keeps factual tasks and source analysis independent of author preferences", () => {
    for (const task of [
      "review",
      "memory-extraction",
      "analysis",
      "voice",
      "handoff",
    ])
      expect(authorMemoryTaskExclusion(task)).not.toBe("");
    for (const task of [
      "draft",
      "rewrite",
      "revision",
      "planning",
      "reader",
      "developmental",
      "publisher",
      "translation",
    ])
      expect(authorMemoryTaskExclusion(task)).toBe("");
  });
  it("honors the global pause and keeps an explicit local opt-out across settings reloads", () => {
    expect(effectiveTaskMemory(undefined, null)).toBe(false);
    expect(effectiveTaskMemory({ useMemory: true }, null)).toBe(true);
    expect(effectiveTaskMemory({ useMemory: true }, false)).toBe(false);
    expect(effectiveTaskMemory({ useMemory: false }, true)).toBe(false);
    expect(effectiveTaskMemory({ useMemory: true }, true, true)).toBe(false);
  });
  it("never treats another user's task as an accessible memory snapshot", () => {
    expect(privateTaskContextAccess("author-a", "author-a")).toBe("owner");
    expect(privateTaskContextAccess("author-a", "admin")).toBe("other");
    expect(privateTaskContextAccess("author-a", "author-b")).toBe("other");
    expect(privateTaskContextAccess("author-a", undefined)).toBe("unknown");
    expect(privateTaskContextAccess(undefined, "author-a")).toBe("unknown");
  });
});
