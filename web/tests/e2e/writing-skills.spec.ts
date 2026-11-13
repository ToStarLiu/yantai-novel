import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { authenticate } from "./auth";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
const guide = `---\nname: concrete-writing-guide\ndescription: 用具体动作和不说破的对白保留人物差异\nlicense: MIT\n---\n\n# 我的写作方向\n每段至少保留一个人物可观察的具体细节。\n只生成候选，不改变人物已知信息。\n`;
async function setup(page: Page) {
  const session = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  const response = await page.request.post("/api/projects", {
    headers,
    data: {
      title: `写作 Skill 验收 · ${Date.now()}`,
      description: "Skill 作者完整流程",
      genre: "悬疑",
    },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  const project = await response.json();
  await page.goto(`/#book/${project.id}`);
  await page.getByRole("button", { name: "写作 Skills", exact: true }).click();
  return { project, headers };
}
async function openMarkdown(page: Page, content: string) {
  await page
    .getByRole("button", { name: "导入写作 Skill", exact: true })
    .click();
  await page.getByRole("tab", { name: "文件或 Markdown", exact: true }).click();
  await page.getByLabel("粘贴 Skill 内容").fill(content);
  await page.getByRole("button", { name: "预览 Skill", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Skill 导入预览" }),
  ).toContainText("尚未导入");
}
test("writing Skills preview, versioned author control, export, scoped context and mobile work together", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const { project, headers } = await setup(page);
  await expect(
    page.getByRole("heading", { name: "写作 Skills", exact: true }),
  ).toBeVisible();
  await openMarkdown(page, guide);
  expect(
    (
      await (
        await page.request.get(`/api/projects/${project.id}/writing-skills`)
      ).json()
    ).items,
  ).toHaveLength(0);
  await page.getByLabel("Skill 优先级", { exact: true }).fill("20");
  await page.getByLabel("Skill 适用于创作对话").check();
  await page.getByLabel("Skill 适用于起草").check();
  await page
    .getByRole("button", { name: "确认导入 Skill", exact: true })
    .click();
  const card = page
    .locator(".skill-card")
    .filter({ hasText: "concrete-writing-guide" });
  await expect(card).toContainText("v1");
  await expect(
    card.getByRole("checkbox", {
      name: "启用 concrete-writing-guide",
      exact: true,
    }),
  ).toBeChecked();
  const saved = (
    await (
      await page.request.get(`/api/projects/${project.id}/writing-skills`)
    ).json()
  ).items[0];
  expect(saved.priority).toBe(20);
  expect(saved.taskTypes).toEqual(["copilot", "draft"]);
  await card.getByRole("button", { name: "编辑", exact: true }).click();
  const edited = guide + "不要在结尾机械点题或总结人生。\n";
  await page.getByLabel("编辑 Skill 内容").fill(edited);
  await page
    .getByRole("button", { name: "保存 Skill 新版本", exact: true })
    .click();
  await expect(card).toContainText("v2");
  const download = page.waitForEvent("download");
  await card.getByRole("link", { name: "导出资料包", exact: true }).click();
  const exported = await download;
  const path = await exported.path();
  expect(path).toBeTruthy();
  const bundle = JSON.parse(await readFile(path!, "utf8"));
  expect(bundle.content).toContain("机械点题");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/skills-mobile.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole("button", { name: "全书规划", exact: true }).click();
  await page
    .getByLabel("给创作伙伴的消息")
    .fill("结合本书启用的写作指南，提出可采用的主线规划。");
  await page.getByRole("button", { name: "发送消息", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "采用全书方案", exact: true }),
  ).toBeEnabled({ timeout: 30000 });
  await expect(page.locator(".copilot-turn-skills")).toContainText(
    "本轮写作指南",
  );
  await page.locator(".copilot-turn-skills summary").click();
  await expect(page.locator(".copilot-turn-skills")).toContainText(
    "concrete-writing-guide v2",
  );
  await page.locator(".copilot-skills summary").click();
  await expect(page.locator(".copilot-skills")).toContainText(
    "后续对话的写作指南",
  );
  await page.screenshot({
    path: "test-results/skills-context-snapshot.png",
    fullPage: true,
  });
  const sessions = await (
    await page.request.get(
      `/api/projects/${project.id}/copilot/sessions?scope=book`,
    )
  ).json();
  const conversation = await (
    await page.request.get(`/api/copilot/sessions/${sessions[0].id}`)
  ).json();
  expect(conversation.turns[0].input.writingSkills.items[0].content).toContain(
    "机械点题",
  );
  await page.getByRole("button", { name: "写作 Skills", exact: true }).click();
  await card
    .getByRole("checkbox", { name: "启用 concrete-writing-guide", exact: true })
    .uncheck();
  await expect(card).toContainText("已停用");
  await expect(page.locator(".copilot-skills")).toHaveCount(0);
  await expect(page.locator(".copilot-turn-skills")).toContainText(
    "concrete-writing-guide v2",
  );
  const current = (
    await (
      await page.request.get(`/api/projects/${project.id}/writing-skills`)
    ).json()
  ).items[0];
  const deniedOld = await page.request.put(
    `/api/projects/${project.id}/writing-skills/${saved.id}`,
    { headers, data: { expectedVersion: 1, enabled: true } },
  );
  expect(deniedOld.status()).toBe(409);
  await card.getByRole("button", { name: "编辑", exact: true }).click();
  await page
    .getByLabel("编辑 Skill 内容")
    .fill(edited + "保留这个尚未保存的草稿。");
  const remote = await page.request.put(
    `/api/projects/${project.id}/writing-skills/${saved.id}`,
    { headers, data: { expectedVersion: current.version, enabled: true } },
  );
  expect(remote.ok(), await remote.text()).toBeTruthy();
  await page
    .getByRole("button", { name: "保存 Skill 新版本", exact: true })
    .click();
  await expect(page.getByRole("alert")).toBeVisible();
  await expect(page.getByLabel("编辑 Skill 内容")).toHaveValue(
    /尚未保存的草稿/,
  );
  await page.getByRole("button", { name: "刷新当前版本", exact: true }).click();
  await expect(
    page.getByText(
      "这项 Skill 已有更新。你的草稿保留在下方，请先核对最新版本。",
    ),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "保存 Skill 新版本", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("button", {
      name: "重新载入最新版本（替换下方草稿）",
      exact: true,
    })
    .click();
  await page
    .getByRole("button", { name: "收起 Skill 表单", exact: true })
    .click();
  await page.screenshot({
    path: "test-results/skills-desktop.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await page.getByLabel("章节标题", { exact: true }).fill("第一章 Skill 创作");
  await page
    .getByRole("button", { name: "创建并进入写作", exact: true })
    .click();
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "写作 Skills", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "写作 Skills 管理" });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText("concrete-writing-guide");
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/skills-workspace-mobile.png",
    fullPage: true,
  });
  await dialog
    .getByRole("button", { name: "关闭写作 Skills", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(errors).toEqual([]);
});

const packageBase64 =
  "UEsDBBQAAAAIAHpEbV1JjZAVkwAAAJcAAAAXAAAAZGlhbG9ndWUtc2tpbGwvU0tJTEwubWTT1dXlykvMTbVSSMlMzMlPL03VLUotzi8tSk7VTS/NTEnlSkktTi7KLCjJzM+zUng6f9eTXX0vtrY8mzbz+ayWp+t3Pp+591lP+9P1e7h0gUZxPW3teDmj9cX63QpFqWmpRal5yanF+jCj9XJT3u/pebJ76dOuFU/2znk6qecZkOzf8GJ977MpO5+2zYSY97ihCQBQSwMEFAAAAAgAekRtXSYNYclcAAAAWgAAACUAAABkaWFsb2d1ZS1za2lsbC9yZWZlcmVuY2VzL2RpYWxvZ3VlLm1kDcu7CoAgFADQX7fAIcoicOsB1ZJDGLiIVn5M6r1O/UJuZzneGKiE1yw4Go8tTWsYZjj4S0rge3YqnNdtIlWsBS47Pk+gCpT9riavODiUFgkFZ1H28R5Dd6Jkuf9QSwMEFAAAAAgAekRtXcV+5fccAAAAGgAAAB0AAABkaWFsb2d1ZS1za2lsbC9zY3JpcHRzL3J1bi5zaEtNzshXyEstSy3SLSrN0y3JyCzWLU4uyiwoAQBQSwECFAAUAAAACAB6RG1dSY2QFZMAAACXAAAAFwAAAAAAAAAAAAAAgAEAAAAAZGlhbG9ndWUtc2tpbGwvU0tJTEwubWRQSwECFAAUAAAACAB6RG1dJg1hyVwAAABaAAAAJQAAAAAAAAAAAAAAgAHIAAAAZGlhbG9ndWUtc2tpbGwvcmVmZXJlbmNlcy9kaWFsb2d1ZS5tZFBLAQIUABQAAAAIAHpEbV3FfuX3HAAAABoAAAAdAAAAAAAAAAAAAACAAWcBAABkaWFsb2d1ZS1za2lsbC9zY3JpcHRzL3J1bi5zaFBLBQYAAAAAAwADAOMAAAC+AQAAAAA=";
test("Skill packages preview reference files and never execute scripts; originals can be selected", async ({
  page,
}) => {
  const { project, headers } = await setup(page);
  await page
    .getByRole("button", { name: "导入写作 Skill", exact: true })
    .click();
  await expect(
    page.getByRole("tab", { name: "精选指南", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByRole("heading", { name: "具体叙事与反套路", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/skills-catalog.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "预览 具体叙事与反套路", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Skill 导入预览" }),
  ).toContainText("natural-chinese-fiction");
  await page.getByRole("tab", { name: "文件或 Markdown", exact: true }).click();
  await page.getByLabel("选择 Skill 文件").setInputFiles({
    name: "dialogue-skill.zip",
    mimeType: "application/zip",
    buffer: Buffer.from(packageBase64, "base64"),
  });
  await page.getByRole("button", { name: "预览 Skill", exact: true }).click();
  const preview = page.getByRole("region", { name: "Skill 导入预览" });
  await expect(preview).toContainText("references/dialogue.md");
  await expect(preview).toContainText(/脚本|scripts|忽略/);
  await preview.getByText("references/dialogue.md", { exact: true }).click();
  await expect(preview).toContainText("不替读者翻译潜台词");
  await preview.screenshot({ path: "test-results/skills-package-preview.png" });
  expect(
    (
      await (
        await page.request.get(`/api/projects/${project.id}/writing-skills`)
      ).json()
    ).items,
  ).toHaveLength(0);
  await page
    .getByRole("button", { name: "确认导入 Skill", exact: true })
    .click();
  const card = page
    .locator(".skill-card")
    .filter({ hasText: "dialogue-resource-guide" });
  await expect(card).toContainText("1 份参考资料");
  const original = (
    await (
      await page.request.get(`/api/projects/${project.id}/writing-skills`)
    ).json()
  ).items[0];
  const exportedResponse = await page.request.get(
    `/api/projects/${project.id}/writing-skills/${original.id}/export`,
  );
  expect(exportedResponse.ok()).toBeTruthy();
  const exportedBundle = await exportedResponse.json();
  exportedBundle.content += "\n保留对话中没有说出口的愿望。\n";
  exportedBundle.resources[0].content +=
    "\n升级后的参考资料：用选择代替性格说明。\n";
  await page
    .getByRole("button", { name: "导入写作 Skill", exact: true })
    .click();
  await page.getByRole("tab", { name: "文件或 Markdown", exact: true }).click();
  await page.getByLabel("选择 Skill 文件").setInputFiles({
    name: "dialogue-resource-guide.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify(exportedBundle)),
  });
  await page.getByRole("button", { name: "预览 Skill", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: "本书已有同名 Skill" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "确认更新 Skill 资料包", exact: true })
    .click();
  await expect(card).toContainText("v2");
  const upgraded = (
    await (
      await page.request.get(`/api/projects/${project.id}/writing-skills`)
    ).json()
  ).items[0];
  expect(upgraded.id).toBe(original.id);
  expect(upgraded.resources[0].content).toContain("升级后的参考资料");
  expect(upgraded.content).toContain("没有说出口的愿望");
  const library = await page.request.post("/api/projects", {
    headers: headers,
    data: {
      title: `写作 Skill 资料包复用 · ${Date.now()}`,
      description: "导出后跨作品复用",
      genre: "悬疑",
    },
  });
  expect(library.ok(), await library.text()).toBeTruthy();
  const target = await library.json();
  const exportedFinal = await (
    await page.request.get(
      `/api/projects/${project.id}/writing-skills/${original.id}/export`,
    )
  ).json();
  const reusePreview = await page.request.post(
    `/api/projects/${target.id}/writing-skills/preview`,
    {
      headers: headers,
      data: { format: "bundle", content: JSON.stringify(exportedFinal) },
    },
  );
  expect(reusePreview.ok(), await reusePreview.text()).toBeTruthy();
  const reused = await reusePreview.json();
  expect(reused.resources[0].content).toContain("升级后的参考资料");
  await card.getByRole("button", { name: "移除", exact: true }).click();
  await page.getByRole("button", { name: "确认移除", exact: true }).click();
  await expect(card).toHaveCount(0);
});

test("read-only collaborators can inspect Skills but cannot enable, edit or import them", async ({
  page,
  browser,
}) => {
  const { project, headers } = await setup(page);
  const preview = await (
    await page.request.post(
      `/api/projects/${project.id}/writing-skills/preview`,
      { headers, data: { format: "markdown", content: guide } },
    )
  ).json();
  const created = await page.request.post(
    `/api/projects/${project.id}/writing-skills`,
    { headers, data: { preview, enabled: true, priority: 0, taskTypes: [] } },
  );
  expect(created.ok(), await created.text()).toBeTruthy();
  const skill = await created.json();
  const invitationResponse = await page.request.post(
    `/api/projects/${project.id}/invitations`,
    { headers, data: { role: "viewer" } },
  );
  expect(invitationResponse.ok(), await invitationResponse.text()).toBeTruthy();
  const invitation = await invitationResponse.json();
  const viewer = await browser.newContext();
  try {
    const p = await viewer.newPage();
    const username = `skill-viewer-${Date.now()}`;
    const accept = await p.request.post(`${origin}/api/invitations/accept`, {
      headers: { Origin: origin },
      data: {
        token: invitation.token,
        username,
        password: "skill-viewer-only-2026",
        displayName: "Skill 只读验收",
      },
    });
    expect(accept.ok(), await accept.text()).toBeTruthy();
    const login = await p.request.post(`${origin}/api/session`, {
      headers: { Origin: origin },
      data: { username, password: "skill-viewer-only-2026" },
    });
    expect(login.ok(), await login.text()).toBeTruthy();
    const session = await login.json();
    await p.goto(`${origin}/#book/${project.id}`);
    await p.getByRole("button", { name: "写作 Skills", exact: true }).click();
    const card = p
      .locator(".skill-card")
      .filter({ hasText: "concrete-writing-guide" });
    await expect(card).toBeVisible();
    await expect(
      p.getByRole("button", { name: "导入写作 Skill", exact: true }),
    ).toBeDisabled();
    await expect(
      card.getByRole("checkbox", {
        name: "启用 concrete-writing-guide",
        exact: true,
      }),
    ).toBeDisabled();
    await expect(
      card.getByRole("button", { name: "编辑", exact: true }),
    ).toBeDisabled();
    const denied = await p.request.put(
      `${origin}/api/projects/${project.id}/writing-skills/${skill.id}`,
      {
        headers: { Origin: origin, "X-CSRF-Token": session.csrfToken },
        data: { expectedVersion: 1, enabled: false },
      },
    );
    expect(denied.status()).toBe(403);
  } finally {
    await viewer.close();
  }
});
