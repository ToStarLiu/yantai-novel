import { diffArrays } from "diff";
export type Hunk = {
  id: number;
  before: string;
  after: string;
  changed: boolean;
  accepted: boolean;
};
// Paragraph-level changes avoid applying model-supplied character offsets.
export function paragraphDiff(before: string, after: string): Hunk[] {
  const old = before.split(/\n\s*\n/),
    next = after.split(/\n\s*\n/);
  const parts = diffArrays(old, next);
  const out: Hunk[] = [];
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (part.removed) {
      const following = parts[i + 1];
      const replacement = following?.added ? following.value.join("\n\n") : "";
      out.push({
        id: out.length,
        before: part.value.join("\n\n"),
        after: replacement,
        changed: true,
        accepted: false,
      });
      if (following?.added) i++;
    } else if (part.added) {
      out.push({
        id: out.length,
        before: "",
        after: part.value.join("\n\n"),
        changed: true,
        accepted: false,
      });
    } else
      out.push({
        id: out.length,
        before: part.value.join("\n\n"),
        after: part.value.join("\n\n"),
        changed: false,
        accepted: true,
      });
  }
  return out;
}
export function applyHunks(hunks: Hunk[]): string {
  return hunks
    .map((h) => (h.accepted ? h.after : h.before))
    .filter(Boolean)
    .join("\n\n");
}
export function canApplyCandidate(
  inputRevisionId: string,
  currentRevisionId: string,
  stale: boolean,
) {
  return !stale && inputRevisionId === currentRevisionId;
}

// Ordinary rewrite output contains only the replacement passage. Rebuild the
// whole frozen candidate before comparing it with a later complete draft.
export function candidateDocumentText(run: {
  type: string;
  output: { text?: string };
  input: Record<string, unknown>;
}): string | undefined {
  const text = run.output.text;
  if (typeof text !== "string") return undefined;
  if (run.type !== "rewrite") return text;
  const source = run.input.source,
    selection = run.input.selection;
  if (
    typeof source !== "string" ||
    typeof selection !== "string" ||
    !selection ||
    source.split(selection).length !== 2
  )
    return undefined;
  return source.replace(selection, () => text);
}
