import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Check,
  KeyRound,
  LoaderCircle,
  Plus,
  RefreshCw,
  Save,
  Server,
  X,
} from "lucide-react";
import { api, message } from "./api";

type Provider = {
  id: string;
  name: string;
  protocol: string;
  baseUrl: string;
  model: string;
  enabled: boolean;
  isDefault: boolean;
  hasKey?: boolean;
  credentialSource?: string;
  noAuth?: boolean;
  storedHeaderNames?: string[];
  headers?: Record<string, string>;
  taskTypes: string[];
  currency?: string;
  inputPricePerMillion?: number | null;
  outputPricePerMillion?: number | null;
};
type Form = {
  id?: string;
  name: string;
  protocol: string;
  baseUrl: string;
  model: string;
  apiKey: string;
  noAuth: boolean;
  isDefault: boolean;
  headers: string;
};
const empty: Form = {
  name: "",
  protocol: "responses",
  baseUrl: "https://api.openai.com/v1",
  model: "",
  apiKey: "",
  noAuth: false,
  isDefault: true,
  headers: "",
};

export default function ProviderSettings({
  onClose,
  notify,
}: {
  onClose: () => void;
  notify: (text: string) => void;
}) {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["provider-settings"],
    queryFn: () =>
      api<{
        providers: Provider[];
        active: Provider;
      }>("/provider-settings"),
  });
  const [form, setForm] = useState<Form>(empty),
    [editing, setEditing] = useState(false),
    [working, setWorking] = useState(""),
    [error, setError] = useState(""),
    [models, setModels] = useState<{ id: string; name: string }[]>([]),
    [connection, setConnection] = useState("");
  const formElement = useRef<HTMLFormElement>(null);
  useEffect(() => {
    if (editing) formElement.current?.scrollIntoView({ block: "start" });
  }, [editing, form.id]);
  const patch = (key: keyof Form, value: string | boolean) =>
    setForm((old) => ({ ...old, [key]: value }));
  const body = () => {
    const headers: Record<string, string> = {};
    for (const line of form.headers.split("\n").filter((s) => s.trim())) {
      const at = line.indexOf(":");
      if (at <= 0) throw new Error("每个请求头使用 名称: 值 的格式。");
      headers[line.slice(0, at).trim()] = line.slice(at + 1).trim();
    }
    const existing = query.data?.providers.find((p) => p.id === form.id);
    return {
      currency: existing?.currency,
      inputPricePerMillion: existing?.inputPricePerMillion,
      outputPricePerMillion: existing?.outputPricePerMillion,
      ...form,
      headers,
      enabled: existing?.enabled ?? true,
      taskTypes: existing?.taskTypes ?? [],
      apiKey: form.apiKey || undefined,
    };
  };
  const refresh = async () => {
    await qc.invalidateQueries({ queryKey: ["provider-settings"] });
    await qc.invalidateQueries({ queryKey: ["providers"] });
    await qc.invalidateQueries({ queryKey: ["config"] });
  };
  const action = async (kind: string, fn: () => Promise<void>) => {
    setWorking(kind);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(message(e));
    } finally {
      setWorking("");
    }
  };
  const discover = () =>
    action("discover", async () => {
      setConnection("");
      const result = await api<{
        reachable: boolean;
        models: { id: string; name: string }[];
        message: string;
      }>("/provider-settings/discover", "POST", body());
      setModels(result.models || []);
      setConnection(
        (result.message
          ? `${result.message}${result.models?.length ? ` · 发现 ${result.models.length} 个模型` : ""}`
          : "") ||
          ((result.models || []).length
            ? `连接成功，发现 ${result.models.length} 个模型。`
            : "连接成功，可手动填写模型名称。"),
      );
      if (!result.reachable)
        throw new Error(result.message || "无法连接供应商");
    });
  const save = () =>
    action("save", async () => {
      if (!form.model.trim()) throw new Error("选择或填写一个模型名称。");
      await api("/provider-settings", "POST", body());
      setForm({ ...empty });
      setEditing(false);
      setModels([]);
      setConnection("");
      await refresh();
      notify("AI 供应商已保存，密钥不会回显。");
    });
  return (
    <div className="modal-backdrop provider-backdrop">
      <section
        className="modal wide provider-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="AI 供应商设置"
      >
        <header>
          <div>
            <span className="eyebrow">YOUR WRITING PARTNER</span>
            <h2>AI 供应商</h2>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label="关闭供应商设置"
          >
            <X size={20} />
          </button>
        </header>
        <div className="provider-body">
          <p className="provider-intro">
            选择陪你写作的模型。连接地址、密钥和模型都可以在这里设置。
          </p>
          {query.isError && (
            <div className="error-box">{message(query.error)}</div>
          )}
          {query.data?.active && (
            <div className="provider-active">
              <Server size={18} />
              <div>
                <strong>
                  当前模型 · {query.data.active.model || "未选择"}
                </strong>
                <small>
                  {query.data.active.protocol === "demo"
                    ? "当前为演示模型，添加真实供应商后可开始 AI 写作。"
                    : "后续对话与写作任务将使用当前默认模型。"}
                </small>
              </div>
            </div>
          )}
          <div className="provider-list">
            {query.data?.providers.map((p) => (
              <article className="provider-item" key={p.id}>
                <div>
                  <strong>{p.name}</strong>
                  <span>
                    {p.model}{" "}
                    <small>
                      ·{" "}
                      {p.protocol === "responses"
                        ? "Responses"
                        : p.protocol === "chat"
                          ? "Chat Completions"
                          : "Ollama"}
                    </small>
                  </span>
                  <small>{p.baseUrl}</small>
                  <small>
                    {!p.enabled ? "已停用 · " : ""}
                    {p.noAuth || p.protocol === "ollama"
                      ? "免密连接"
                      : p.hasKey
                        ? "密钥已配置"
                        : "尚未配置密钥"}
                  </small>
                </div>
                <div className="provider-item-actions">
                  {p.isDefault ? (
                    <span className="pill">
                      <Check size={12} /> 默认
                    </span>
                  ) : (
                    <button
                      className="text-button"
                      disabled={!!working || !p.enabled}
                      onClick={() =>
                        void action(p.id, async () => {
                          await api(
                            `/provider-settings/${p.id}/default`,
                            "PUT",
                            {},
                          );
                          await refresh();
                        })
                      }
                    >
                      设为默认
                    </button>
                  )}
                  <button
                    className="text-button"
                    onClick={() => {
                      setForm({
                        id: p.id,
                        name: p.name,
                        protocol: p.protocol,
                        baseUrl: p.baseUrl,
                        model: p.model,
                        apiKey: "",
                        noAuth: p.noAuth || false,
                        isDefault: p.isDefault,
                        headers: Object.entries(p.headers || {})
                          .map(([k, v]) => `${k}: ${v}`)
                          .join("\n"),
                      });
                      setEditing(true);
                      setModels([]);
                      setConnection("");
                      setError("");
                    }}
                  >
                    编辑
                  </button>
                  <button
                    className="text-button"
                    disabled={!!working}
                    onClick={() =>
                      void action(p.id, async () => {
                        await api(`/provider-settings/${p.id}/enabled`, "PUT", {
                          enabled: !p.enabled,
                        });
                        await refresh();
                      })
                    }
                  >
                    {p.enabled ? "停用" : "启用"}
                  </button>
                </div>
              </article>
            ))}
          </div>
          {!editing ? (
            <button
              className="button primary"
              onClick={() => {
                setForm({ ...empty });
                setEditing(true);
              }}
            >
              <Plus size={15} /> 添加 AI 供应商
            </button>
          ) : (
            <form
              ref={formElement}
              className="form-stack provider-form"
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <div className="row-between">
                <h3>{form.id ? "编辑供应商" : "新的 AI 供应商"}</h3>
                <button
                  type="button"
                  className="text-button"
                  onClick={() => setEditing(false)}
                >
                  收起
                </button>
              </div>
              <div className="author-form-grid">
                <label>
                  供应商名称
                  <input
                    required
                    value={form.name}
                    onChange={(e) => patch("name", e.target.value)}
                    placeholder="例如：我的 OpenAI 接口"
                  />
                </label>
                <label>
                  接口协议
                  <select
                    value={form.protocol}
                    onChange={(e) => {
                      patch("protocol", e.target.value);
                      setModels([]);
                    }}
                  >
                    <option value="responses">OpenAI Responses</option>
                    <option value="chat">OpenAI Chat Completions</option>
                    <option value="ollama">本地 Ollama</option>
                  </select>
                </label>
              </div>
              <label>
                API 地址
                <input
                  aria-label="API 地址"
                  required
                  type="url"
                  value={form.baseUrl}
                  onChange={(e) => {
                    patch("baseUrl", e.target.value);
                    setConnection("");
                    setModels([]);
                  }}
                  placeholder="https://api.example.com/v1"
                />
                <small>兼容服务通常包含 /v1；Ollama 填写服务根地址。</small>
                {form.id &&
                  query.data?.providers.find((p) => p.id === form.id)
                    ?.baseUrl !== form.baseUrl && (
                    <small>修改服务地址后，请重新填写密钥。</small>
                  )}
              </label>
              <label>
                <span>
                  <KeyRound size={13} /> API 密钥
                </span>
                <input
                  type="password"
                  autoComplete="off"
                  value={form.apiKey}
                  onChange={(e) => patch("apiKey", e.target.value)}
                  placeholder={
                    form.id
                      ? "已保存的密钥不会显示，留空继续使用"
                      : "粘贴服务提供的 API Key"
                  }
                />
              </label>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={form.noAuth}
                  onChange={(e) => patch("noAuth", e.target.checked)}
                />
                此服务不需要密钥（如本地模型）
              </label>
              <div className="provider-discovery">
                <button
                  className="button"
                  type="button"
                  disabled={!!working || !form.baseUrl}
                  onClick={() => void discover()}
                >
                  {working === "discover" ? (
                    <LoaderCircle className="spin" size={14} />
                  ) : (
                    <RefreshCw size={14} />
                  )}{" "}
                  测试连接并获取模型
                </button>
                {connection && <span role="status">{connection}</span>}
              </div>
              <label>
                使用的模型
                <input
                  aria-label="使用的模型"
                  list="provider-model-options"
                  required
                  value={form.model}
                  onChange={(e) => patch("model", e.target.value)}
                  placeholder="获取模型后选择，也可直接输入模型名"
                />
                <datalist id="provider-model-options">
                  {models.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name || m.id}
                    </option>
                  ))}
                </datalist>
              </label>
              <details>
                <summary>额外请求头</summary>
                {!!query.data?.providers.find((p) => p.id === form.id)
                  ?.storedHeaderNames?.length && (
                  <p className="muted small">
                    已保存的敏感请求头不会回显，留空会继续使用。
                  </p>
                )}
                <label>
                  每行一个请求头
                  <textarea
                    rows={3}
                    value={form.headers}
                    onChange={(e) => patch("headers", e.target.value)}
                    placeholder="x-custom-header: value"
                  />
                </label>
              </details>
              <label className="check-label">
                <input
                  type="checkbox"
                  checked={form.isDefault}
                  onChange={(e) => patch("isDefault", e.target.checked)}
                />
                设为默认写作模型
              </label>
              <div className="row">
                <button
                  type="submit"
                  className="button primary"
                  disabled={!!working}
                >
                  {working === "save" ? (
                    <LoaderCircle className="spin" size={14} />
                  ) : (
                    <Save size={14} />
                  )}{" "}
                  保存供应商
                </button>
                <span className="muted small">
                  密钥仅保存于服务端，界面不会回显。
                </span>
              </div>
            </form>
          )}
          {error && (
            <div className="error-box" role="alert">
              {error}
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
