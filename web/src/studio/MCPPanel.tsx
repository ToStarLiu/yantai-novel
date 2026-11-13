import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plug, Save, Search, Play, ShieldCheck } from "lucide-react";
import { api } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  Result,
  useStudioAction,
  type PanelProps,
} from "../Studio";
import { strings } from "../studioApi";
type Config = {
  id: string;
  name: string;
  url: string;
  apiKeyEnv: string;
  allowedTools: string[];
  allowLoopback: boolean;
  enabled: boolean;
};
type Tool = {
  name: string;
  description: string;
  inputSchema: unknown;
  annotations?: { readOnlyHint?: boolean; destructiveHint?: boolean };
};
export default function MCPPanel({ detail, notify, refresh }: PanelProps) {
  const q = useQuery({
    queryKey: ["studio", detail.project.id, "mcp"],
    queryFn: () => api<Config[]>("/mcp-configs"),
  });
  const logs = useQuery({
    queryKey: ["studio", detail.project.id, "mcp-logs"],
    queryFn: () => api("/mcp-call-logs"),
  });
  const [config, setConfig] = useState<Config>({
      id: "",
      name: "",
      url: "",
      apiKeyEnv: "",
      allowedTools: [],
      allowLoopback: false,
      enabled: true,
    }),
    [tools, setTools] = useState<Tool[]>([]),
    [tool, setTool] = useState(""),
    [args, setArgs] = useState("{}"),
    [result, setResult] = useState<unknown>(null);
  const { working, act } = useStudioAction(notify, refresh);
  return (
    <div className="studio-two-col">
      <StudioCard
        title="MCP 只读工具服务"
        subtitle="只执行显式授权且服务声明为只读的工具，不授予任意系统操作。"
      >
        <div className="studio-list">
          {q.data?.map((c) => (
            <button
              key={c.id}
              onClick={() => {
                setConfig(c);
                setTools([]);
              }}
            >
              <Plug size={16} />
              <span>
                <strong>{c.name}</strong>
                <small>{c.url}</small>
              </span>
            </button>
          ))}
        </div>
        <form
          className="studio-form"
          onSubmit={(e) => {
            e.preventDefault();
            void act(
              async () =>
                setConfig(await api<Config>("/mcp-configs", "POST", config)),
              "MCP 服务配置已保存。",
            );
          }}
        >
          <label>
            名称
            <input
              value={config.name}
              onChange={(e) => setConfig({ ...config, name: e.target.value })}
              required
            />
          </label>
          <label>
            服务 URL
            <input
              value={config.url}
              onChange={(e) => setConfig({ ...config, url: e.target.value })}
              placeholder="https://example.com/mcp"
              required
            />
          </label>
          <label>
            密钥环境变量名（可选）
            <input
              value={config.apiKeyEnv}
              onChange={(e) =>
                setConfig({ ...config, apiKeyEnv: e.target.value })
              }
            />
          </label>
          <label>
            允许工具名（每行或逗号分隔）
            <textarea
              rows={3}
              value={config.allowedTools.join("\n")}
              onChange={(e) =>
                setConfig({ ...config, allowedTools: strings(e.target.value) })
              }
            />
          </label>
          <label className="check-label">
            <input
              type="checkbox"
              checked={config.allowLoopback}
              onChange={(e) =>
                setConfig({ ...config, allowLoopback: e.target.checked })
              }
            />{" "}
            明确允许本机回环 HTTP 服务
          </label>
          <label className="check-label">
            <input
              type="checkbox"
              checked={config.enabled}
              onChange={(e) =>
                setConfig({ ...config, enabled: e.target.checked })
              }
            />{" "}
            启用
          </label>
          <StudioButton type="submit" primary disabled={working}>
            <Save size={13} /> 保存 MCP 服务
          </StudioButton>
          {config.id && (
            <StudioButton
              disabled={working}
              onClick={() =>
                void act(async () => {
                  await api(`/mcp-configs/${config.id}/enabled`, "PUT", {
                    enabled: !config.enabled,
                  });
                  setConfig({ ...config, enabled: !config.enabled });
                }, "该版本 MCP 服务状态已更新。")
              }
            >
              {config.enabled ? "停用这个服务版本" : "启用这个服务版本"}
            </StudioButton>
          )}
        </form>
        <StudioError error={q.error} />
      </StudioCard>
      <StudioCard title="发现、审阅与调用">
        <div className="studio-form">
          <StudioButton
            disabled={working || !config.id}
            onClick={() =>
              void act(async () =>
                setTools(
                  (
                    await api<{ tools: Tool[] }>(
                      `/mcp-configs/${config.id}/tools`,
                      "POST",
                      {},
                    )
                  ).tools,
                ),
              )
            }
          >
            <Search size={13} /> 发现工具与类型定义
          </StudioButton>
          {tools.map((t) => (
            <article className="studio-inset" key={t.name}>
              <div className="row-between">
                <strong>{t.name}</strong>
                <span className="pill">
                  {t.annotations?.readOnlyHint &&
                  !t.annotations?.destructiveHint
                    ? "只读声明"
                    : "不可执行"}
                </span>
              </div>
              <p className="muted small">{t.description}</p>
              <details>
                <summary>输入 Schema</summary>
                <Result value={t.inputSchema} />
              </details>
              <StudioButton
                disabled={
                  !t.annotations?.readOnlyHint ||
                  t.annotations?.destructiveHint ||
                  !config.allowedTools.includes(t.name)
                }
                onClick={() => setTool(t.name)}
              >
                选择此授权工具
              </StudioButton>
            </article>
          ))}
          <label>
            工具名
            <input value={tool} onChange={(e) => setTool(e.target.value)} />
          </label>
          <label>
            类型化参数（JSON）
            <textarea
              rows={6}
              value={args}
              onChange={(e) => setArgs(e.target.value)}
            />
          </label>
          <StudioButton
            disabled={working || !tool || !config.id}
            onClick={() =>
              void act(async () =>
                setResult(
                  await api(`/mcp-configs/${config.id}/call`, "POST", {
                    name: tool,
                    arguments: JSON.parse(args),
                  }),
                ),
              )
            }
          >
            <Play size={13} /> 执行授权只读调用
          </StudioButton>
          <Result value={result} />
          <details>
            <summary>调用记录</summary>
            <Result value={logs.data} />
          </details>
        </div>
      </StudioCard>
    </div>
  );
}
