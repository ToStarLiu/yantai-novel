import { describe, expect, it } from "vitest";
import { copilotTimeline, mergeCopilotEvents } from "./copilotEvents";

describe("persistent Copilot events", () => {
  it("replays reconnect overlap once and preserves tool and Chinese delta order", () => {
    const events = mergeCopilotEvents(
      [
        { id: 1, kind: "delta", data: { text: "先查" } },
        {
          id: 2,
          kind: "tool-start",
          data: { name: "read_chapter", toolCallId: "a" },
        },
      ],
      [
        { id: 1, kind: "delta", data: { text: "先查" } },
        { id: 4, kind: "delta", data: { text: "继续。" } },
        {
          id: 3,
          kind: "tool-result",
          data: {
            name: "read_chapter",
            toolCallId: "a",
            result: { content: { text: "正文" } },
          },
        },
      ],
    );
    const timeline = copilotTimeline(events);
    expect(timeline.map((item) => item.kind)).toEqual(["text", "tool", "text"]);
    expect(timeline[0]).toMatchObject({ text: "先查" });
    expect(timeline[1]).toMatchObject({
      pending: false,
      data: { result: { content: { text: "正文" } } },
    });
    expect(timeline[2]).toMatchObject({ text: "继续。" });
  });
  it("does not merge repeated calls of the same tool or erase a completed candidate", () => {
    const timeline = copilotTimeline([
      {
        id: 1,
        kind: "tool-start",
        data: { name: "read_chapter", toolCallId: "a" },
      },
      {
        id: 2,
        kind: "tool-result",
        data: {
          name: "read_chapter",
          toolCallId: "a",
          result: { artifact: { kind: "revision-candidate", runId: "run-1" } },
        },
      },
      {
        id: 3,
        kind: "tool-start",
        data: { name: "read_chapter", toolCallId: "b" },
      },
      { id: 4, kind: "compaction", data: {} },
    ]);
    expect(timeline).toHaveLength(3);
    expect(timeline[0]).toMatchObject({
      data: { result: { artifact: { runId: "run-1" } } },
      pending: false,
    });
    expect(timeline[1]).toMatchObject({
      pending: true,
      data: { toolCallId: "b" },
    });
    expect(timeline[2].kind).toBe("compaction");
  });
});
