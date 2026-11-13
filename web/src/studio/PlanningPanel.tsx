import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Check,
  ArrowRight,
  ArrowLeft,
  Plus,
  Save,
  Sparkles,
  Trash2,
} from "lucide-react";
import { api, id, type Run } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  useStudioAction,
  ChapterSelect,
  type PanelProps,
} from "../Studio";
type Brief = {
  plotPlan?: string;
  genre: string;
  audience: string;
  targetWords: number;
  targetChapters: number;
  pov: string;
  platform: string;
  theme: string;
  sellingPoint: string;
  coreConflict: string;
  protagonistDesire: string;
  failureCost: string;
  endingDirection: string;
  worldRules: string[];
  relationships: string;
  restrictions: string;
  style: string;
  pace: string;
  volumes: {
    id: string;
    title: string;
    goal: string;
    summary: string;
    chapterCount: number;
  }[];
};
const empty: Brief = {
  genre: "",
  audience: "",
  targetWords: 100000,
  targetChapters: 30,
  pov: "第三人称有限视角",
  platform: "",
  theme: "",
  sellingPoint: "",
  coreConflict: "",
  protagonistDesire: "",
  failureCost: "",
  endingDirection: "",
  worldRules: [],
  relationships: "",
  restrictions: "",
  style: "",
  pace: "",
  volumes: [],
};
export default function PlanningPanel({
  detail,
  chapterId,
  notify,
  refresh,
}: PanelProps) {
  const query = useQuery({
    queryKey: ["studio", detail.project.id],
    queryFn: () =>
      api<{ brief: { version: number; data: Brief } }>(
        `/projects/${detail.project.id}/studio`,
      ),
  });
  const [value, setValue] = useState(empty),
    [version, setVersion] = useState(0),
    [step, setStep] = useState(0),
    [target, setTarget] = useState(chapterId);
  const [scope, setScope] = useState("chapter"),
    [volumeID, setVolumeID] = useState(""),
    [selectedPlan, setSelectedPlan] = useState("");
  const plans = useQuery({
    queryKey: ["studio", detail.project.id, "plans"],
    queryFn: () => api<Run[]>(`/projects/${detail.project.id}/runs`),
    refetchInterval: 3000,
  });
  const plan = plans.data?.find((r) => r.id === selectedPlan);
  const { working, act } = useStudioAction(notify, refresh);
  useEffect(() => {
    if (query.data?.brief) {
      setValue({ ...empty, ...query.data.brief.data });
      setVersion(query.data.brief.version);
    }
  }, [query.data]);
  const field = (key: keyof Brief, label: string, multi = false) => (
    <label key={key}>
      {label}
      {multi ? (
        <textarea
          rows={3}
          value={String(value[key] || "")}
          onChange={(e) => setValue({ ...value, [key]: e.target.value })}
        />
      ) : (
        <input
          value={String(value[key] || "")}
          onChange={(e) => setValue({ ...value, [key]: e.target.value })}
        />
      )}
    </label>
  );
  const save = async () => {
    const result = await api<{ version: number }>(
      `/projects/${detail.project.id}/studio/brief`,
      "PUT",
      { expectedVersion: version, data: value },
    );
    setVersion(result.version);
  };
  return (
    <div className="studio-two-col">
      <StudioCard
        title="三层故事规划"
        subtitle="确定读者承诺，再确定人物选择，最后落到卷章。"
      >
        <div className="wizard-steps">
          {["故事承诺", "冲突与代价", "结构与节奏"].map((s, i) => (
            <button
              className={step === i ? "active" : ""}
              onClick={() => setStep(i)}
              key={s}
            >
              <span>{i + 1}</span>
              {s}
            </button>
          ))}
        </div>
        <div className="studio-form">
          {step === 0 ? (
            <>
              {field("genre", "题材")}
              {field("audience", "目标读者")}
              {field("platform", "连载平台 / 阅读场景")}
              {field("theme", "核心命题", true)}
              {field("sellingPoint", "一句话卖点与读者承诺", true)}
              <div className="form-grid">
                <label>
                  目标总字数
                  <input
                    type="number"
                    min="1000"
                    value={value.targetWords}
                    onChange={(e) =>
                      setValue({
                        ...value,
                        targetWords: Number(e.target.value),
                      })
                    }
                  />
                </label>
                <label>
                  目标章数
                  <input
                    type="number"
                    min="1"
                    max="5000"
                    value={value.targetChapters}
                    onChange={(e) =>
                      setValue({
                        ...value,
                        targetChapters: Number(e.target.value),
                      })
                    }
                  />
                </label>
              </div>
            </>
          ) : step === 1 ? (
            <>
              {field("coreConflict", "无法回避的核心冲突", true)}
              {field("protagonistDesire", "主角最想得到什么", true)}
              {field("failureCost", "失败必须付出什么代价", true)}
              {field("endingDirection", "结局方向与不提前揭示的谜底", true)}
              {field("relationships", "重要关系与对立力量", true)}
              <label>
                不可违背的世界规则
                <textarea
                  rows={4}
                  value={value.worldRules.join("\n")}
                  onChange={(e) =>
                    setValue({
                      ...value,
                      worldRules: e.target.value.split("\n").filter(Boolean),
                    })
                  }
                />
              </label>
            </>
          ) : (
            <>
              {field("pov", "叙事视角")}
              {field("style", "文风约束", true)}
              {field("pace", "节奏与章末悬念", true)}
              {field("restrictions", "必须避免的走向", true)}
              <div className="row-between">
                <span className="section-label">分卷规划</span>
                <button
                  className="text-button"
                  onClick={() =>
                    setValue({
                      ...value,
                      volumes: [
                        ...value.volumes,
                        {
                          id: id(),
                          title: "新卷",
                          goal: "",
                          summary: "",
                          chapterCount: 10,
                        },
                      ],
                    })
                  }
                >
                  <Plus size={13} /> 添加卷
                </button>
              </div>
              {value.volumes.map((v, i) => (
                <div className="studio-inset" key={v.id}>
                  <input
                    aria-label="卷标题"
                    value={v.title}
                    onChange={(e) =>
                      setValue({
                        ...value,
                        volumes: value.volumes.map((x, j) =>
                          j === i ? { ...x, title: e.target.value } : x,
                        ),
                      })
                    }
                  />
                  <textarea
                    placeholder="本卷目标、转折与结尾"
                    rows={3}
                    value={v.summary}
                    onChange={(e) =>
                      setValue({
                        ...value,
                        volumes: value.volumes.map((x, j) =>
                          j === i ? { ...x, summary: e.target.value } : x,
                        ),
                      })
                    }
                  />
                  <label>
                    章数
                    <input
                      type="number"
                      min="1"
                      value={v.chapterCount}
                      onChange={(e) =>
                        setValue({
                          ...value,
                          volumes: value.volumes.map((x, j) =>
                            j === i
                              ? { ...x, chapterCount: Number(e.target.value) }
                              : x,
                          ),
                        })
                      }
                    />
                  </label>
                </div>
              ))}
            </>
          )}
          <div className="row-between">
            <StudioButton
              disabled={step === 0}
              onClick={() => setStep(step - 1)}
            >
              <ArrowLeft size={13} /> 上一层
            </StudioButton>
            {step < 2 ? (
              <StudioButton primary onClick={() => setStep(step + 1)}>
                下一层 <ArrowRight size={13} />
              </StudioButton>
            ) : (
              <StudioButton
                primary
                disabled={working}
                onClick={() => void act(save, "故事规划已保存")}
              >
                <Save size={14} /> 保存完整规划
              </StudioButton>
            )}
          </div>
          <StudioError error={query.error} />
        </div>
      </StudioCard>
      <StudioCard
        title="把规划带进写作"
        subtitle="规划保存为作者计划，不会被当作已发生事实。"
      >
        <div className="studio-form">
          <label>
            规划层级
            <select value={scope} onChange={(e) => setScope(e.target.value)}>
              <option value="chapter">本章细纲</option>
              <option value="book">全书剧情计划</option>
              <option value="volume">分卷剧情计划</option>
            </select>
          </label>
          {scope === "volume" && (
            <label>
              目标卷
              <select
                value={volumeID}
                onChange={(e) => setVolumeID(e.target.value)}
              >
                <option value="">选择已规划的卷</option>
                {value.volumes.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.title}
                  </option>
                ))}
              </select>
            </label>
          )}
          <ChapterSelect
            chapters={detail.chapters}
            value={target}
            onChange={setTarget}
          />
          <StudioButton
            disabled={working || !target}
            onClick={() =>
              void act(async () => {
                await save();
                const c = detail.chapters.find((c) => c.id === target);
                await api(`/chapters/${target}/runs`, "POST", {
                  intentId: id(),
                  taskType: "planning",
                  scope,
                  volumeId: scope === "volume" ? volumeID : "",
                  inputRevisionId: c?.currentRevisionId || "",
                  instructions:
                    `使用已确认故事规划，生成${scope === "book" ? "全书剧情计划" : scope === "volume" ? "分卷剧情计划" : "本章可编辑章纲"}：` +
                    JSON.stringify(value),
                });
              }, "章纲规划任务已创建，请回到作者助手审核。")
            }
          >
            <Sparkles size={14} /> 生成
            {scope === "book" ? "全书" : scope === "volume" ? "分卷" : "本章"}
            规划候选
          </StudioButton>
          <p className="muted small">
            人物目标、核心冲突、失败代价与章末钩子将一同进入规划输入。生成结果仍是独立候选。
          </p>
          <label>
            已有规划候选
            <select
              value={selectedPlan}
              onChange={(e) => setSelectedPlan(e.target.value)}
            >
              <option value="">选择生成结果</option>
              {plans.data
                ?.filter(
                  (r) =>
                    r.type === "planning" &&
                    r.status === "Succeeded" &&
                    (r.input.scope || "chapter") === scope &&
                    (scope !== "volume" || r.input.volumeId === volumeID),
                )
                .map((r) => (
                  <option key={r.id} value={r.id}>
                    {String(r.input.scope || "chapter")} · {r.id.slice(0, 8)}
                  </option>
                ))}
            </select>
          </label>
          {plan && (
            <>
              <pre className="studio-result">{plan.output.text}</pre>
              <StudioButton
                primary
                disabled={working || plan.stale}
                onClick={() =>
                  void act(async () => {
                    if (scope === "chapter") {
                      const c = detail.chapters.find((c) => c.id === target);
                      await api(`/chapters/${target}`, "PUT", {
                        title: c?.title,
                        volume: c?.volume,
                        blueprint: plan.output.text,
                      });
                    } else {
                      await api(
                        `/projects/${detail.project.id}/studio/brief/apply-plan`,
                        "POST",
                        {
                          expectedVersion: version,
                          scope,
                          volumeId: scope === "volume" ? volumeID : "",
                          text: plan.output.text,
                          runId: plan.id,
                        },
                      );
                    }
                  }, "规划候选已由作者采用。")
                }
              >
                <Check size={13} /> 采用为
                {scope === "book"
                  ? "全书计划"
                  : scope === "volume"
                    ? "分卷计划"
                    : "章纲"}
              </StudioButton>
            </>
          )}
          {value.plotPlan && (
            <details>
              <summary>当前已采用全书计划</summary>
              <pre className="studio-result">{value.plotPlan}</pre>
            </details>
          )}
        </div>
      </StudioCard>
    </div>
  );
}
