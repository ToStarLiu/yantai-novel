import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, Save, Trash2, Sparkles } from "lucide-react";
import { api, id } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  ChapterSelect,
  useStudioAction,
  type PanelProps,
} from "../Studio";
import { strings } from "../studioApi";
type Handoff = {
  endLocation: string;
  presentCharacters: string[];
  newFacts: string[];
  inventoryChanges: string[];
  openActions: string[];
  requiredContinuity: string[];
  forbiddenChanges: string[];
};
type Scene = {
  id: string;
  chapterId: string;
  order: number;
  title: string;
  goal: string;
  conflict: string;
  causes: string[];
  observedBy: string[];
  forbiddenChanges: string[];
  handoff: Handoff;
  nodeIds?: string[];
  boardVersion?: number;
};
export default function ScenePanel({
  detail,
  chapterId,
  notify,
  refresh,
}: PanelProps) {
  const q = useQuery({
    queryKey: ["studio", detail.project.id],
    queryFn: () =>
      api<{
        scenes: { version: number; data: { scenes: Scene[] } };
        board: { data: { nodes: { id: string; label: string }[] } };
      }>(`/projects/${detail.project.id}/studio`),
  });
  const [scenes, setScenes] = useState<Scene[]>([]),
    [version, setVersion] = useState(0),
    [selected, setSelected] = useState("");
  const { working, act } = useStudioAction(notify, refresh);
  useEffect(() => {
    if (q.data?.scenes) {
      setScenes(q.data.scenes.data.scenes || []);
      setVersion(q.data.scenes.version);
    }
  }, [q.data]);
  const scene = scenes.find((s) => s.id === selected) || scenes[0];
  const patch = (v: Partial<Scene>) =>
    setScenes(scenes.map((s) => (s.id === scene?.id ? { ...s, ...v } : s)));
  const save = () =>
    api(`/projects/${detail.project.id}/studio/scenes`, "PUT", {
      expectedVersion: version,
      data: { scenes },
    });
  return (
    <div className="studio-two-col">
      <StudioCard
        title="场景序列"
        subtitle="把章节拆成有目标、有冲突、有后果的场景。"
        actions={
          <StudioButton
            onClick={() => {
              const s: Scene = {
                id: id(),
                chapterId,
                order: scenes.length + 1,
                title: "新场景",
                goal: "",
                conflict: "",
                causes: [],
                observedBy: [],
                forbiddenChanges: [],
                handoff: {
                  endLocation: "",
                  presentCharacters: [],
                  newFacts: [],
                  inventoryChanges: [],
                  openActions: [],
                  requiredContinuity: [],
                  forbiddenChanges: [],
                },
              };
              setScenes([...scenes, s]);
              setSelected(s.id);
            }}
          >
            <Plus size={13} /> 新场景
          </StudioButton>
        }
      >
        <div className="studio-list">
          {scenes.map((s) => (
            <button
              className={scene?.id === s.id ? "selected" : ""}
              key={s.id}
              onClick={() => setSelected(s.id)}
            >
              <span className="scene-number">
                {String(s.order).padStart(2, "0")}
              </span>
              <span>
                <strong>{s.title}</strong>
                <small>
                  {detail.chapters.find((c) => c.id === s.chapterId)?.title ||
                    "尚未关联章节"}
                </small>
              </span>
            </button>
          ))}
        </div>
        <StudioError error={q.error} />
        <StudioButton
          primary
          disabled={working}
          onClick={() => void act(save, "场景与交接状态已保存")}
        >
          <Save size={14} /> 保存场景序列
        </StudioButton>
      </StudioCard>
      {scene && (
        <StudioCard
          title="场景导演卡"
          subtitle="交接描述是下一场景的必要约束，不能越过定稿审核。"
        >
          <div className="studio-form">
            <label>
              场景名
              <input
                value={scene.title}
                onChange={(e) => patch({ title: e.target.value })}
              />
            </label>
            <ChapterSelect
              chapters={detail.chapters}
              value={scene.chapterId}
              onChange={(v) => patch({ chapterId: v })}
            />
            <label>
              目标
              <textarea
                rows={2}
                value={scene.goal}
                onChange={(e) => patch({ goal: e.target.value })}
              />
            </label>
            <label>
              冲突
              <textarea
                rows={2}
                value={scene.conflict}
                onChange={(e) => patch({ conflict: e.target.value })}
              />
            </label>
            <label>
              前因场景
              <select
                multiple
                value={scene.causes}
                onChange={(e) =>
                  patch({
                    causes: Array.from(e.target.selectedOptions).map(
                      (o) => o.value,
                    ),
                  })
                }
              >
                {scenes
                  .filter((s) => s.id !== scene.id)
                  .map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.title}
                    </option>
                  ))}
              </select>
            </label>
            <label>
              亲历人物
              <select
                multiple
                value={scene.observedBy}
                onChange={(e) =>
                  patch({
                    observedBy: Array.from(e.target.selectedOptions).map(
                      (o) => o.value,
                    ),
                  })
                }
              >
                {detail.settings
                  .filter((s) => s.kind === "character")
                  .map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
              </select>
            </label>
            <label>
              结束地点
              <input
                value={scene.handoff.endLocation}
                onChange={(e) =>
                  patch({
                    handoff: { ...scene.handoff, endLocation: e.target.value },
                  })
                }
              />
            </label>
            <label>
              在场人物
              <select
                multiple
                value={scene.handoff.presentCharacters}
                onChange={(e) =>
                  patch({
                    handoff: {
                      ...scene.handoff,
                      presentCharacters: Array.from(
                        e.target.selectedOptions,
                      ).map((o) => o.value),
                    },
                  })
                }
              >
                {detail.settings
                  .filter((s) => s.kind === "character")
                  .map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
              </select>
            </label>
            <label>
              关联画布节点
              <select
                multiple
                value={scene.nodeIds || []}
                onChange={(e) =>
                  patch({
                    nodeIds: Array.from(e.target.selectedOptions).map(
                      (o) => o.value,
                    ),
                  })
                }
              >
                {q.data?.board.data.nodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.label}
                  </option>
                ))}
              </select>
            </label>
            {[
              ["newFacts", "本场新事实"],
              ["inventoryChanges", "物品变化"],
              ["openActions", "尚未完成的动作"],
              ["requiredContinuity", "下场必须延续"],
              ["forbiddenChanges", "禁止无依据变化"],
            ].map(([k, l]) => (
              <label key={k}>
                {l}
                <textarea
                  rows={2}
                  value={(
                    (scene.handoff[k as keyof Handoff] as string[]) || []
                  ).join("\n")}
                  onChange={(e) =>
                    patch({
                      handoff: {
                        ...scene.handoff,
                        [k]: strings(e.target.value),
                      },
                    })
                  }
                />
              </label>
            ))}
            <StudioButton
              disabled={working || !scene.chapterId}
              onClick={() =>
                void act(async () => {
                  await save();
                  await api(`/chapters/${scene.chapterId}/runs`, "POST", {
                    intentId: id(),
                    taskType: "handoff",
                    sceneId: scene.id,
                    inputRevisionId:
                      detail.chapters.find((c) => c.id === scene.chapterId)
                        ?.currentRevisionId || "",
                    instructions:
                      "检查当前场景的因果与交接约束：" + JSON.stringify(scene),
                  });
                }, "场景交接检查任务已创建。")
              }
            >
              <Sparkles size={14} /> 检查场景交接
            </StudioButton>
            <button
              className="text-button danger"
              onClick={() => setScenes(scenes.filter((s) => s.id !== scene.id))}
            >
              <Trash2 size={13} /> 删除场景卡
            </button>
          </div>
        </StudioCard>
      )}
    </div>
  );
}
