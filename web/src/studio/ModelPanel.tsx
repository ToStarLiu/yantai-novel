import { useState, useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Plus,
  Save,
  Activity,
  RefreshCw,
  Search,
  ShieldCheck,
} from "lucide-react";
import { api, type Session } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  Result,
  ChapterSelect,
  useStudioAction,
  type PanelProps,
} from "../Studio";
import { strings } from "../studioApi";
import MCPPanel from "./MCPPanel";
type Provider = {
  id: string;
  name: string;
  protocol: string;
  baseUrl: string;
  apiKeyEnv: string;
  headers: Record<string, string>;
  model: string;
  taskTypes: string[];
  enabled: boolean;
  isDefault: boolean;
  currency?: string;
  inputPricePerMillion?: number | null;
  outputPricePerMillion?: number | null;
};
type Embedding = {
  id: string;
  name: string;
  baseUrl: string;
  apiKeyEnv: string;
  model: string;
  dimension: number;
  kind: string;
  enabled: boolean;
};
export default function ModelPanel({
  detail,
  chapterId,
  notify,
  refresh,
}: PanelProps) {
  const session = useQuery({
    queryKey: ["session"],
    queryFn: () => api<Session>("/session"),
  });
  const isAdmin = !!session.data?.user?.admin;
  const providers = useQuery({
    queryKey: ["studio", detail.project.id, "providers"],
    queryFn: () => api<Provider[]>("/provider-configs"),
    enabled: isAdmin,
  });
  const embeddings = useQuery({
    queryKey: ["studio", detail.project.id, "embedding"],
    queryFn: () => api<Embedding[]>("/embedding-configs"),
    enabled: isAdmin,
  });
  const metrics = useQuery({
    queryKey: ["studio", detail.project.id, "metrics"],
    queryFn: () => api("/metrics"),
    refetchInterval: 5000,
    enabled: isAdmin,
  });
  const [tab, setTab] = useState("providers"),
    [p, setP] = useState<Provider>({
      id: "",
      name: "",
      protocol: "responses",
      baseUrl: "",
      apiKeyEnv: "LLM_API_KEY",
      headers: {},
      model: "",
      taskTypes: [],
      enabled: true,
      isDefault: false,
    }),
    [e, setE] = useState<Embedding>({
      id: "",
      name: "",
      baseUrl: "",
      apiKeyEnv: "EMBEDDING_API_KEY",
      model: "",
      dimension: 1536,
      kind: "embedding",
      enabled: true,
    }),
    [term, setTerm] = useState(""),
    [mode, setMode] = useState("hybrid"),
    [embeddingID, setEmbeddingID] = useState(""),
    [rerankID, setRerankID] = useState(""),
    [before, setBefore] = useState(chapterId),
    [maxRunes, setMaxRunes] = useState(3000),
    [result, setResult] = useState<unknown>(null);
  const { working, act } = useStudioAction(notify, refresh);
  useEffect(() => {
    if (session.data && !isAdmin) setTab("retrieval");
  }, [session.data, isAdmin]);
  return (
    <>
      <div className="studio-subtabs">
        {[
          ["providers", "Provider 与路由"],
          ["retrieval", "语义检索与压缩"],
          ["metrics", "运行与缓存观测"],
          ["mcp", "MCP 工具"],
        ]
          .filter(([v]) => isAdmin || v === "retrieval")
          .map(([v, l]) => (
            <button
              key={v}
              className={tab === v ? "active" : ""}
              onClick={() => setTab(v)}
            >
              {l}
            </button>
          ))}
      </div>
      {tab === "providers" ? (
        <div className="studio-two-col">
          <StudioCard
            title="模型服务"
            subtitle="配置只保存环境变量名称，密钥始终留在服务端。"
          >
            <div className="studio-list">
              {providers.data?.map((provider) => (
                <button key={provider.id} onClick={() => setP(provider)}>
                  <ShieldCheck size={17} />
                  <span>
                    <strong>
                      {provider.name}
                      {provider.isDefault ? " · 默认" : ""}
                    </strong>
                    <small>
                      {provider.model} · {provider.protocol}
                    </small>
                  </span>
                </button>
              ))}
            </div>
            <StudioButton
              onClick={() =>
                setP({
                  id: "",
                  name: "",
                  protocol: "responses",
                  baseUrl: "",
                  apiKeyEnv: "LLM_API_KEY",
                  headers: {},
                  model: "",
                  taskTypes: [],
                  enabled: true,
                  isDefault: false,
                })
              }
            >
              <Plus size={13} /> 新建 Provider
            </StudioButton>
            <StudioError error={providers.error} />
          </StudioCard>
          <StudioCard title="服务配置与任务路由">
            <form
              className="studio-form"
              onSubmit={(event) => {
                event.preventDefault();
                void act(
                  async () =>
                    setP(await api<Provider>("/provider-configs", "POST", p)),
                  "Provider 配置已保存",
                );
              }}
            >
              {[
                ["name", "名称"],
                ["baseUrl", "API Base URL"],
                ["model", "模型 ID"],
                ["apiKeyEnv", "密钥环境变量名"],
              ].map(([k, l]) => (
                <label key={k}>
                  {l}
                  <input
                    value={String(p[k as keyof Provider])}
                    onChange={(event) =>
                      setP({ ...p, [k]: event.target.value })
                    }
                    required
                  />
                </label>
              ))}
              <label>
                协议
                <select
                  value={p.protocol}
                  onChange={(event) =>
                    setP({ ...p, protocol: event.target.value })
                  }
                >
                  <option value="responses">Responses</option>
                  <option value="chat">Chat Completions</option>
                  <option value="ollama">Ollama</option>
                </select>
              </label>
              <label>
                接管任务类型（逗号分隔）
                <input
                  value={p.taskTypes.join(",")}
                  onChange={(event) =>
                    setP({ ...p, taskTypes: strings(event.target.value) })
                  }
                  placeholder="draft,review,translation"
                />
              </label>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={p.enabled}
                  onChange={(event) =>
                    setP({ ...p, enabled: event.target.checked })
                  }
                />{" "}
                启用服务
              </label>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={p.isDefault}
                  onChange={(event) =>
                    setP({ ...p, isDefault: event.target.checked })
                  }
                />{" "}
                作为默认模型服务
              </label>
              <div className="form-grid">
                <label>
                  计价币种
                  <input
                    value={p.currency || ""}
                    onChange={(event) =>
                      setP({ ...p, currency: event.target.value })
                    }
                    placeholder="CNY / USD"
                  />
                </label>
                <label>
                  输入价格 / 百万 Token
                  <input
                    type="number"
                    min="0"
                    step="any"
                    value={p.inputPricePerMillion ?? ""}
                    onChange={(event) =>
                      setP({
                        ...p,
                        inputPricePerMillion:
                          event.target.value === ""
                            ? null
                            : Number(event.target.value),
                      })
                    }
                    placeholder="留空保持未知"
                  />
                </label>
                <label>
                  输出价格 / 百万 Token
                  <input
                    type="number"
                    min="0"
                    step="any"
                    value={p.outputPricePerMillion ?? ""}
                    onChange={(event) =>
                      setP({
                        ...p,
                        outputPricePerMillion:
                          event.target.value === ""
                            ? null
                            : Number(event.target.value),
                      })
                    }
                    placeholder="留空保持未知"
                  />
                </label>
              </div>
              <p className="muted small">
                金额为作者配置的估算单价。任务排队时冻结价格快照，留空不会假定为免费。
              </p>
              <StudioButton primary type="submit" disabled={working}>
                <Save size={14} /> 保存配置
              </StudioButton>
              {p.id && (
                <StudioButton
                  disabled={working}
                  onClick={() =>
                    void act(async () => {
                      await api(`/provider-configs/${p.id}/enabled`, "PUT", {
                        enabled: !p.enabled,
                      });
                      setP({ ...p, enabled: !p.enabled });
                    }, "该版本 Provider 状态已更新。")
                  }
                >
                  {p.enabled ? "停用这个服务版本" : "启用这个服务版本"}
                </StudioButton>
              )}
              {p.id && (
                <StudioButton
                  disabled={working}
                  onClick={() =>
                    void act(
                      async () =>
                        setResult(
                          await api(
                            `/provider-configs/${p.id}/health`,
                            "POST",
                            {},
                          ),
                        ),
                      "服务健康检查完成。",
                    )
                  }
                >
                  <Activity size={13} /> 检查连接，不发生成请求
                </StudioButton>
              )}
              <Result value={result} />
            </form>
          </StudioCard>
        </div>
      ) : tab === "retrieval" ? (
        <div className="studio-two-col">
          <StudioCard title="Embedding / Rerank 服务">
            <div className="studio-list">
              {embeddings.data?.map((config) => (
                <button key={config.id} onClick={() => setE(config)}>
                  <Search size={16} />
                  <span>
                    <strong>{config.name}</strong>
                    <small>
                      {config.kind} · {config.model}
                    </small>
                  </span>
                </button>
              ))}
            </div>
            <form
              className="studio-form"
              onSubmit={(event) => {
                event.preventDefault();
                void act(
                  async () =>
                    setE(await api<Embedding>("/embedding-configs", "POST", e)),
                  "语义检索服务配置已保存",
                );
              }}
            >
              {[
                ["name", "服务名称"],
                ["baseUrl", "API Base URL"],
                ["model", "模型"],
                ["apiKeyEnv", "密钥环境变量名"],
              ].map(([k, l]) => (
                <label key={k}>
                  {l}
                  <input
                    value={String(e[k as keyof Embedding])}
                    onChange={(event) =>
                      setE({ ...e, [k]: event.target.value })
                    }
                    required
                  />
                </label>
              ))}
              <label>
                用途
                <select
                  value={e.kind}
                  onChange={(event) => setE({ ...e, kind: event.target.value })}
                >
                  <option value="embedding">Embedding 向量</option>
                  <option value="rerank">Rerank 重排</option>
                </select>
              </label>
              <label>
                向量维数
                <input
                  type="number"
                  min="1"
                  value={e.dimension}
                  onChange={(event) =>
                    setE({ ...e, dimension: Number(event.target.value) })
                  }
                />
              </label>
              <StudioButton primary type="submit" disabled={working}>
                保存服务
              </StudioButton>
            </form>
          </StudioCard>
          <StudioCard
            title="有来源的混合召回"
            subtitle="作品、版本、时间与人物可见性仍是各路检索的硬过滤。"
          >
            <div className="studio-form">
              <ChapterSelect
                chapters={detail.chapters}
                value={before}
                onChange={setBefore}
              />
              <label>
                检索问题
                <input
                  value={term}
                  onChange={(event) => setTerm(event.target.value)}
                />
              </label>
              <label>
                检索模式
                <select
                  value={mode}
                  onChange={(event) => setMode(event.target.value)}
                >
                  <option value="keyword">关键词</option>
                  <option value="vector">向量语义</option>
                  <option value="hybrid">混合检索</option>
                </select>
              </label>
              <label>
                Embedding 服务
                <select
                  value={embeddingID}
                  onChange={(event) => setEmbeddingID(event.target.value)}
                >
                  <option value="">选择服务</option>
                  {embeddings.data
                    ?.filter((e) => e.kind === "embedding")
                    .map((e) => (
                      <option value={e.id} key={e.id}>
                        {e.name}
                      </option>
                    ))}
                </select>
              </label>
              <label>
                Rerank 服务
                <select
                  value={rerankID}
                  onChange={(event) => setRerankID(event.target.value)}
                >
                  <option value="">不做重排</option>
                  {embeddings.data
                    ?.filter((e) => e.kind === "rerank")
                    .map((e) => (
                      <option value={e.id} key={e.id}>
                        {e.name}
                      </option>
                    ))}
                </select>
              </label>
              <StudioButton
                disabled={working || !embeddingID}
                onClick={() =>
                  void act(
                    () =>
                      api(
                        `/projects/${detail.project.id}/embeddings/rebuild`,
                        "POST",
                        { configId: embeddingID },
                      ),
                    "向量索引重建完成。",
                  )
                }
              >
                <RefreshCw size={13} /> 重建该模型向量索引
              </StudioButton>
              <StudioButton
                primary
                disabled={working || !term}
                onClick={() =>
                  void act(async () =>
                    setResult(
                      await api(
                        `/projects/${detail.project.id}/semantic-search`,
                        "POST",
                        {
                          text: term,
                          beforeChapterId: before,
                          purpose: "AuthorReview",
                          limit: 10,
                          mode,
                          embeddingProviderId: embeddingID,
                          rerankProviderId: rerankID,
                        },
                      ),
                    ),
                  )
                }
              >
                <Search size={13} /> 对照召回结果
              </StudioButton>
              <label>
                压缩后上下文字符上限
                <input
                  type="number"
                  min="200"
                  max="20000"
                  value={maxRunes}
                  onChange={(event) => setMaxRunes(Number(event.target.value))}
                />
              </label>
              <StudioButton
                disabled={working || !term}
                onClick={() =>
                  void act(async () =>
                    setResult(
                      await api(
                        `/projects/${detail.project.id}/context-compression`,
                        "POST",
                        {
                          query: {
                            text: term,
                            beforeChapterId: before,
                            purpose: "AuthorReview",
                            limit: 15,
                          },
                          maxRunes,
                        },
                      ),
                    ),
                  )
                }
              >
                生成带来源的压缩上下文
              </StudioButton>
              <Result value={result} />
            </div>
          </StudioCard>
        </div>
      ) : tab === "mcp" ? (
        <MCPPanel
          detail={detail}
          chapterId={chapterId}
          notify={notify}
          refresh={refresh}
        />
      ) : (
        <StudioCard
          title="持久运行观测"
          subtitle="队列、尝试、延迟与冻结运行轨迹。缓存不能绕过输入版本或作品隔离。"
        >
          <Result value={metrics.data} />
          <StudioError error={metrics.error} />
        </StudioCard>
      )}
    </>
  );
}
