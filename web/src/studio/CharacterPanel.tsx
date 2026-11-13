import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Save, Users } from "lucide-react";
import { api } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Arc = {
  entityId: string;
  arc: string;
  emotion: string;
  goal: string;
  beliefImportance: number;
  recencyWeight: number;
  forgetAfterChapters: number;
  notes: string;
};
export default function CharacterPanel({
  detail,
  notify,
  refresh,
}: PanelProps) {
  const q = useQuery({
    queryKey: ["studio", detail.project.id],
    queryFn: () =>
      api<{ arcs: { version: number; data: { characters: Arc[] } } }>(
        `/projects/${detail.project.id}/studio`,
      ),
  });
  const [arcs, setArcs] = useState<Arc[]>([]),
    [version, setVersion] = useState(0),
    [selected, setSelected] = useState(
      detail.settings.find((s) => s.kind === "character")?.id || "",
    );
  const { working, act } = useStudioAction(notify, refresh);
  useEffect(() => {
    if (q.data?.arcs) {
      setArcs(q.data.arcs.data.characters || []);
      setVersion(q.data.arcs.version);
    }
  }, [q.data]);
  const arc = arcs.find((a) => a.entityId === selected) || {
    entityId: selected,
    arc: "",
    emotion: "",
    goal: "",
    beliefImportance: 0.7,
    recencyWeight: 0.5,
    forgetAfterChapters: 0,
    notes: "",
  };
  const patch = (v: Partial<Arc>) => {
    const next = { ...arc, ...v };
    setArcs(
      arcs.some((a) => a.entityId === selected)
        ? arcs.map((a) => (a.entityId === selected ? next : a))
        : [...arcs, next],
    );
  };
  return (
    <div className="studio-two-col">
      <StudioCard
        title="人物视角"
        subtitle="同一个世界，每个人只知道自己经历过的部分。"
      >
        <div className="studio-list">
          {detail.settings
            .filter((s) => s.kind === "character")
            .map((s) => (
              <button
                key={s.id}
                className={s.id === selected ? "selected" : ""}
                onClick={() => setSelected(s.id)}
              >
                <Users size={18} />
                <span>
                  <strong>{s.name}</strong>
                  <small>{s.content.slice(0, 70)}</small>
                </span>
              </button>
            ))}
        </div>
        <StudioError error={q.error} />
      </StudioCard>
      {selected && (
        <StudioCard
          title={
            detail.settings.find((s) => s.id === selected)?.name || "人物弧光"
          }
          subtitle="情绪和目标是创作投影；有来源的历史事实仍然保留。"
        >
          <div className="studio-form">
            {[
              ["arc", "人物弧：从哪里走向哪里"],
              ["goal", "此刻最想得到什么"],
              ["emotion", "当前情绪与诱因"],
              ["notes", "声音、行为与关系提醒"],
            ].map(([k, l]) => (
              <label key={k}>
                {l}
                <textarea
                  rows={3}
                  value={String(arc[k as keyof Arc])}
                  onChange={(e) => patch({ [k]: e.target.value })}
                />
              </label>
            ))}
            <label>
              记忆重要性 · {arc.beliefImportance}
              <input
                type="range"
                min="0"
                max="1"
                step=".05"
                value={arc.beliefImportance}
                onChange={(e) =>
                  patch({ beliefImportance: Number(e.target.value) })
                }
              />
            </label>
            <label>
              新近程度权重 · {arc.recencyWeight}
              <input
                type="range"
                min="0"
                max="1"
                step=".05"
                value={arc.recencyWeight}
                onChange={(e) =>
                  patch({ recencyWeight: Number(e.target.value) })
                }
              />
            </label>
            <label>
              认知投影淡出章数（0 表示保留）
              <input
                type="number"
                min="0"
                value={arc.forgetAfterChapters}
                onChange={(e) =>
                  patch({ forgetAfterChapters: Number(e.target.value) })
                }
              />
            </label>
            <StudioButton
              primary
              disabled={working}
              onClick={() =>
                void act(
                  () =>
                    api(`/projects/${detail.project.id}/studio/arcs`, "PUT", {
                      expectedVersion: version,
                      data: { characters: arcs },
                    }),
                  "人物弧与记忆投影已保存。",
                )
              }
            >
              <Save size={14} /> 保存人物弧
            </StudioButton>
            <p className="muted small">
              投影淡出不删除来源；百科中可查看被弱化或淡出的角色记忆。
            </p>
          </div>
        </StudioCard>
      )}
    </div>
  );
}
