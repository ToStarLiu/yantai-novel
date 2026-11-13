import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Play,
  Pause,
  GitBranch,
  Check,
  RotateCcw,
  Snowflake,
} from "lucide-react";
import { api } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  Result,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Sandbox = {
  id: string;
  version: number;
  title: string;
  frozen: boolean;
  state: {
    turn: number;
    actors: {
      entityId: string;
      name: string;
      resources: number;
      tension: number;
      location: string;
      goal: string;
    }[];
  };
  candidate: { events: unknown[]; diff: unknown[] } | null;
  history: { id: string; state: { turn: number }; createdAt: string }[];
};
type Branch = {
  id: string;
  sourceProjectId: string;
  targetProjectId: string;
  title: string;
  sourceEpoch: number;
  createdAt: string;
};
export default function SimulationPanel({
  detail,
  notify,
  refresh,
  mode,
}: PanelProps & { mode: "sandbox" | "branches" }) {
  const base = `/projects/${detail.project.id}/studio`;
  const q = useQuery({
    queryKey: ["studio", detail.project.id, "simulation-document"],
    queryFn: () => api<{ sandboxes: Sandbox[] }>(base),
  });
  const branches = useQuery({
    queryKey: ["studio", detail.project.id, "branches"],
    queryFn: () => api<Branch[]>(base + "/branches"),
    enabled: mode === "branches",
  });
  const [title, setTitle] = useState(""),
    [entities, setEntities] = useState<string[]>([]),
    [selected, setSelected] = useState(""),
    [steps, setSteps] = useState(3),
    [seed, setSeed] = useState(42),
    [resourceDelta, setResourceDelta] = useState(-2),
    [tensionDelta, setTensionDelta] = useState(5);
  const { working, act } = useStudioAction(notify, refresh);
  const box =
    q.data?.sandboxes.find((s) => s.id === selected) || q.data?.sandboxes[0];
  return mode === "branches" ? (
    <div className="studio-two-col">
      <StudioCard
        title="探索另一条故事线"
        subtitle="完整复制作品、版本与来源。切换分支会打开另一部独立作品。"
      >
        <div className="studio-form">
          <label>
            分支名
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="如果林川没有打开箱子…"
            />
          </label>
          <StudioButton
            primary
            disabled={working || !title}
            onClick={() =>
              void act(async () => {
                const result = await api<{ project: { id: string } }>(
                  base + "/fork",
                  "POST",
                  { title, expectedEpoch: detail.project.epoch },
                );
                notify("分支已创建，可以在分支列表切换。");
                setTitle("");
              })
            }
          >
            <GitBranch size={14} /> 从当前版本分叉
          </StudioButton>
          <p className="muted small">
            原作品保持原样。每个分支拥有独立记忆和引用，不能将另一个分支的事实直接当作当前事实。
          </p>
        </div>
      </StudioCard>
      <StudioCard title="已有故事分支">
        {branches.data?.map((branch) => (
          <article className="studio-inset" key={branch.id}>
            <h4>{branch.title}</h4>
            <p className="muted small">从作品版本 {branch.sourceEpoch} 创建</p>
            <StudioButton
              onClick={() => {
                localStorage.setItem("novel-project", branch.targetProjectId);
                window.location.reload();
              }}
            >
              切换到这个分支
            </StudioButton>
          </article>
        ))}
        <StudioError error={branches.error} />
      </StudioCard>
    </div>
  ) : (
    <div className="studio-two-col">
      <StudioCard
        title="建立可回滚的世界实验"
        subtitle="推演只发生在沙盘，采用结果也不会自动改写权威正文。"
      >
        <div className="studio-form">
          <label>
            实验名称
            <input value={title} onChange={(e) => setTitle(e.target.value)} />
          </label>
          <label>
            参与实体
            <select
              multiple
              value={entities}
              onChange={(e) =>
                setEntities(
                  Array.from(e.target.selectedOptions).map((o) => o.value),
                )
              }
            >
              {detail.settings.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
          <div className="form-grid">
            <label>
              回合数
              <input
                type="number"
                min="1"
                max="20"
                value={steps}
                onChange={(e) => setSteps(Number(e.target.value))}
              />
            </label>
            <label>
              随机种子
              <input
                type="number"
                value={seed}
                onChange={(e) => setSeed(Number(e.target.value))}
              />
            </label>
            <label>
              每回合资源变化
              <input
                type="number"
                min="-100"
                max="100"
                value={resourceDelta}
                onChange={(e) => setResourceDelta(Number(e.target.value))}
              />
            </label>
            <label>
              每回合紧张度变化
              <input
                type="number"
                min="-100"
                max="100"
                value={tensionDelta}
                onChange={(e) => setTensionDelta(Number(e.target.value))}
              />
            </label>
          </div>
          <StudioButton
            primary
            disabled={working || !title || !entities.length}
            onClick={() =>
              void act(async () => {
                const r = await api<Sandbox>(base + "/sandboxes", "POST", {
                  title,
                  entityIds: entities,
                  steps,
                  seed,
                  parameters: {
                    initialResources: 100,
                    initialTension: 10,
                    resourceDelta,
                    tensionDelta,
                    eventLabel: "势力互动",
                  },
                });
                setSelected(r.id);
              }, "世界实验已建立。")
            }
          >
            <Play size={14} /> 建立沙盘
          </StudioButton>
        </div>
        <div className="studio-list">
          {q.data?.sandboxes.map((s) => (
            <button
              key={s.id}
              onClick={() => setSelected(s.id)}
              className={s.id === box?.id ? "selected" : ""}
            >
              <span>
                <strong>{s.title}</strong>
                <small>
                  第 {s.state.turn} 回合 · {s.frozen ? "已冻结" : "可推演"}
                </small>
              </span>
            </button>
          ))}
        </div>
        <StudioError error={q.error} />
      </StudioCard>
      {box && (
        <StudioCard
          title={box.title}
          subtitle={
            "第 " + box.state.turn + " 回合 · " + (box.frozen ? "冻结" : "开放")
          }
        >
          <div className="sandbox-actors">
            {box.state.actors.map((actor) => (
              <div className="studio-inset" key={actor.entityId}>
                <h4>{actor.name}</h4>
                <p>
                  {actor.location || "地点未知"} · {actor.goal || "目标未定"}
                </p>
                <label>
                  资源 {actor.resources}
                  <progress max="1000" value={actor.resources} />
                </label>
                <label>
                  紧张度 {actor.tension}
                  <progress max="100" value={actor.tension} />
                </label>
              </div>
            ))}
          </div>
          <div className="studio-actions">
            <StudioButton
              disabled={working || box.frozen}
              onClick={() =>
                void act(
                  () =>
                    api(base + `/sandboxes/${box.id}/simulate`, "POST", {
                      expectedVersion: box.version,
                      steps,
                      seed,
                    }),
                  "推演候选已生成，确认后才更新沙盘。",
                )
              }
            >
              <Play size={13} /> 推演 {steps} 回合
            </StudioButton>
            <StudioButton
              disabled={working || box.frozen || !box.candidate}
              primary
              onClick={() =>
                void act(
                  () =>
                    api(base + `/sandboxes/${box.id}/confirm`, "POST", {
                      expectedVersion: box.version,
                    }),
                  "已采用到沙盘；正文与确认事实未改变。",
                )
              }
            >
              <Check size={13} /> 采用沙盘候选
            </StudioButton>
            <StudioButton
              disabled={working || box.frozen}
              onClick={() =>
                void act(
                  () =>
                    api(base + `/sandboxes/${box.id}/freeze`, "POST", {
                      expectedVersion: box.version,
                    }),
                  "沙盘已冻结",
                )
              }
            >
              <Snowflake size={13} /> 冻结
            </StudioButton>
          </div>
          {box.candidate && (
            <>
              <h4>候选事件与变化</h4>
              <Result value={box.candidate} />
            </>
          )}
          <h4>检查点</h4>
          {box.history.map((h) => (
            <div className="row-between studio-checkpoint" key={h.id}>
              <span>第 {h.state.turn} 回合</span>
              <StudioButton
                disabled={working}
                onClick={() =>
                  void act(
                    () =>
                      api(base + `/sandboxes/${box.id}/rollback`, "POST", {
                        expectedVersion: box.version,
                        checkpoint: h.id,
                      }),
                    "沙盘已回滚，历史检查点保留。",
                  )
                }
              >
                <RotateCcw size={12} /> 回滚
              </StudioButton>
            </div>
          ))}
        </StudioCard>
      )}
    </div>
  );
}
