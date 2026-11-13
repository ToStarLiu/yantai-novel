import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { UserPlus, Users, Copy, Trash2, Shield, Link2 } from "lucide-react";
import { api, message } from "../api";
import {
  StudioCard,
  StudioButton,
  StudioError,
  useStudioAction,
  type PanelProps,
} from "../Studio";
type Member = {
  userId: string;
  username: string;
  displayName: string;
  role: string;
  joinedAt: string;
};
type Presence = {
  userId: string;
  username: string;
  displayName: string;
  chapterId: string;
  revisionId: string;
  updatedAt: string;
};
type Invitation = {
  id: string;
  role: string;
  expiresAt: string;
  token?: string;
};
export default function CollaborationPanel({
  detail,
  chapterId,
  notify,
  refresh,
}: PanelProps) {
  const base = `/projects/${detail.project.id}`;
  const access = useQuery({
    queryKey: ["studio", detail.project.id, "access"],
    queryFn: () =>
      api<{ role: string; user: { admin: boolean } }>(base + "/access"),
  });
  const members = useQuery({
    queryKey: ["studio", detail.project.id, "members"],
    queryFn: () => api<Member[]>(base + "/members"),
  });
  const presence = useQuery({
    queryKey: ["studio", detail.project.id, "presence"],
    queryFn: () => api<Presence[]>(base + "/presence"),
    refetchInterval: 15000,
  });
  const invites = useQuery({
    queryKey: ["studio", detail.project.id, "invitations"],
    queryFn: () => api<Invitation[]>(base + "/invitations"),
    enabled: access.data?.role === "owner",
  });
  const [username, setUsername] = useState(""),
    [role, setRole] = useState("editor"),
    [token, setToken] = useState("");
  const [account, setAccount] = useState({
    username: "",
    password: "",
    displayName: "",
  });
  const { working, act } = useStudioAction(notify, refresh);
  return (
    <div className="studio-two-col">
      <StudioCard
        title="作品成员"
        subtitle="共享的是作品和可审阅的版本；并发改稿仍通过冲突校验保护。"
      >
        <p className="muted small">
          你的权限：{roleName(access.data?.role || "")}
        </p>
        <div className="member-list">
          {members.data?.map((member) => (
            <div className="member-row" key={member.userId}>
              <span className="member-avatar">
                {(member.displayName || member.username).slice(0, 1)}
              </span>
              <div>
                <strong>{member.displayName || member.username}</strong>
                <small>{member.username}</small>
              </div>
              {access.data?.role === "owner" ? (
                <>
                  <select
                    aria-label={"成员权限 " + member.username}
                    value={member.role}
                    onChange={(e) =>
                      void act(
                        () =>
                          api(base + "/members/" + member.userId, "PUT", {
                            role: e.target.value,
                          }),
                        "成员权限已更新",
                      )
                    }
                  >
                    {["owner", "editor", "reviewer", "viewer"].map((r) => (
                      <option key={r} value={r}>
                        {roleName(r)}
                      </option>
                    ))}
                  </select>
                  <button
                    className="icon-button danger"
                    aria-label={"移除成员 " + member.username}
                    onClick={() =>
                      void act(
                        () => api(base + "/members/" + member.userId, "DELETE"),
                        "成员已移除",
                      )
                    }
                  >
                    <Trash2 size={13} />
                  </button>
                </>
              ) : (
                <span className="pill">{roleName(member.role)}</span>
              )}
            </div>
          ))}
        </div>
        {access.data?.role === "owner" && (
          <form
            className="studio-form"
            onSubmit={(e) => {
              e.preventDefault();
              void act(
                () => api(base + "/members", "POST", { username, role }),
                "已有账户已加入作品",
              );
            }}
          >
            <label>
              添加已有账户
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="账户名"
                required
              />
            </label>
            <label>
              权限
              <select value={role} onChange={(e) => setRole(e.target.value)}>
                {["editor", "reviewer", "viewer", "owner"].map((r) => (
                  <option key={r} value={r}>
                    {roleName(r)}
                  </option>
                ))}
              </select>
            </label>
            <StudioButton type="submit" disabled={working}>
              <UserPlus size={13} /> 添加成员
            </StudioButton>
          </form>
        )}
        <StudioError error={members.error} />
      </StudioCard>
      <div>
        <StudioCard
          title="邀请链接"
          subtitle="新成员可使用令牌注册或验证已有账户后加入。"
        >
          {access.data?.role === "owner" ? (
            <div className="studio-form">
              <label>
                邀请权限
                <select value={role} onChange={(e) => setRole(e.target.value)}>
                  {["editor", "reviewer", "viewer"].map((r) => (
                    <option key={r} value={r}>
                      {roleName(r)}
                    </option>
                  ))}
                </select>
              </label>
              <StudioButton
                primary
                disabled={working}
                onClick={() =>
                  void act(async () => {
                    const invite = await api<Invitation>(
                      base + "/invitations",
                      "POST",
                      { role },
                    );
                    setToken(invite.token || "");
                  }, "邀请已创建；令牌只在本次展示。")
                }
              >
                <Link2 size={13} /> 创建邀请
              </StudioButton>
              {token && (
                <div className="invitation-token">
                  <span>邀请令牌，请私下交给协作者</span>
                  <code>{token}</code>
                  <StudioButton
                    onClick={() =>
                      void navigator.clipboard
                        .writeText(token)
                        .then(() => notify("邀请令牌已复制"))
                    }
                  >
                    <Copy size={13} /> 复制令牌
                  </StudioButton>
                </div>
              )}
              {invites.data?.map((invite) => (
                <div className="row-between studio-checkpoint" key={invite.id}>
                  <span>
                    {roleName(invite.role)} ·{" "}
                    {new Date(invite.expiresAt).toLocaleDateString("zh-CN")}
                  </span>
                  <button
                    className="text-button danger"
                    onClick={() =>
                      void act(
                        () => api(base + "/invitations/" + invite.id, "DELETE"),
                        "邀请已撤销",
                      )
                    }
                  >
                    撤销
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <p className="muted">仅作品所有者可以管理邀请。</p>
          )}
        </StudioCard>
        <StudioCard
          title="在线写作者"
          subtitle="最近一分钟的活跃位置，不代表正在合并正文。"
        >
          {presence.data?.map((person) => (
            <div className="member-row" key={person.userId}>
              <span className="tiny-dot" />
              <div>
                <strong>{person.displayName || person.username}</strong>
                <small>
                  {detail.chapters.find((c) => c.id === person.chapterId)
                    ?.title || "书房"}
                </small>
              </div>
            </div>
          ))}
          {!presence.data?.length && (
            <p className="muted small">暂无其他活跃成员。</p>
          )}
        </StudioCard>
        {access.data?.user?.admin && (
          <StudioCard
            title="管理作者账户"
            subtitle="新账户默认不访问任何作品，随后按作品授予成员权限。"
          >
            <form
              className="studio-form"
              onSubmit={(e) => {
                e.preventDefault();
                void act(async () => {
                  await api("/admin/users", "POST", account);
                  setAccount({ username: "", password: "", displayName: "" });
                }, "账户已创建，可以添加为作品成员。");
              }}
            >
              <label>
                新账户名
                <input
                  autoComplete="off"
                  value={account.username}
                  onChange={(e) =>
                    setAccount({ ...account, username: e.target.value })
                  }
                  required
                />
              </label>
              <label>
                显示名
                <input
                  value={account.displayName}
                  onChange={(e) =>
                    setAccount({ ...account, displayName: e.target.value })
                  }
                />
              </label>
              <label>
                初始密码
                <input
                  autoComplete="new-password"
                  type="password"
                  minLength={10}
                  value={account.password}
                  onChange={(e) =>
                    setAccount({ ...account, password: e.target.value })
                  }
                  required
                />
              </label>
              <StudioButton type="submit" disabled={working}>
                <UserPlus size={13} /> 创建账户
              </StudioButton>
            </form>
          </StudioCard>
        )}
      </div>
    </div>
  );
}
function roleName(role: string) {
  return (
    (
      {
        owner: "所有者",
        editor: "编辑者",
        reviewer: "审校者",
        viewer: "只读者",
      } as Record<string, string>
    )[role] || role
  );
}
