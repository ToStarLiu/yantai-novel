import { useCallback, useEffect, useRef, useState } from "react";
import { api, APIError, type Chapter, type Doc, type Revision } from "./api";
export type SaveStatus = "saved" | "dirty" | "saving" | "error" | "conflict";
export function useDraft(chapter: Chapter, onSaved: () => void) {
  const [doc, setDoc] = useState<Doc>(
    chapter.revision?.document || {
      type: "doc",
      content: [{ type: "paragraph" }],
    },
  );
  const [revision, setRevision] = useState(chapter.currentRevisionId || "");
  const [status, setStatus] = useState<SaveStatus>("saved");
  const [error, setError] = useState("");
  const [recovery, setRecovery] = useState<Doc | null>(() => {
    try {
      const saved = localStorage.getItem("novel-draft-" + chapter.id);
      return saved ? JSON.parse(saved) : null;
    } catch {
      return null;
    }
  });
  const latest = useRef(doc),
    baseline = useRef(revision),
    generation = useRef(0),
    savedGeneration = useRef(0),
    running = useRef<Promise<string> | null>(null),
    blocked = useRef(false),
    timer = useRef<ReturnType<typeof setTimeout> | null>(null),
    alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);
  const flush = useCallback(async (): Promise<string> => {
    if (timer.current) clearTimeout(timer.current);
    if (running.current) {
      await running.current;
      if (generation.current !== savedGeneration.current) return flush();
      return baseline.current;
    }
    if (blocked.current)
      throw new Error("存在版本冲突，请先查看服务器版本并复制需要保留的内容。");
    if (generation.current === savedGeneration.current) return baseline.current;
    const persist = async () => {
      const current = generation.current;
      setStatus("saving");
      try {
        const result = await api<Revision>(
          `/chapters/${chapter.id}/revisions`,
          "POST",
          {
            expectedCurrentRevisionId: baseline.current,
            document: latest.current,
            source: "author",
          },
        );
        baseline.current = result.id;
        savedGeneration.current = current;
        if (alive.current) {
          setRevision(result.id);
          setStatus(generation.current === current ? "saved" : "dirty");
          setError("");
        }
        if (generation.current === current)
          localStorage.removeItem("novel-draft-" + chapter.id);
        onSaved();
        return result.id;
      } catch (e) {
        if (alive.current) {
          const conflict = e instanceof APIError && e.status === 409;
          blocked.current = conflict;
          setStatus(conflict ? "conflict" : "error");
          setError(e instanceof Error ? e.message : "保存失败");
        }
        throw e;
      } finally {
        running.current = null;
      }
    };
    running.current = persist();
    const result = await running.current;
    if (generation.current !== savedGeneration.current) return flush();
    return result;
  }, [chapter.id, onSaved]);
  const change = useCallback(
    (value: Doc) => {
      latest.current = value;
      generation.current++;
      setDoc(value);
      setStatus(blocked.current ? "conflict" : "dirty");
      try {
        localStorage.setItem(
          "novel-draft-" + chapter.id,
          JSON.stringify(value),
        );
      } catch {}
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        void flush().catch(() => {});
      }, 800);
    },
    [chapter.id, flush],
  );
  useEffect(() => {
    const protect = (e: BeforeUnloadEvent) => {
      if (generation.current !== savedGeneration.current) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", protect);
    return () => window.removeEventListener("beforeunload", protect);
  }, []);
  return {
    doc,
    revision,
    status,
    error,
    recovery,
    flush,
    change,
    recover: () => {
      if (recovery) change(recovery);
      setRecovery(null);
    },
    discardRecovery: () => {
      localStorage.removeItem("novel-draft-" + chapter.id);
      setRecovery(null);
    },
  };
}
