import { expect, test, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { authenticate } from "./auth";
import { openBookSection, openWorkspaceAction } from "./navigation";

const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";

test("an author plans a book before creating chapters and keeps writing when returning", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await authenticate(page);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "每个念头，都值得一本书。" }),
  ).toBeVisible();
  await expect(page.getByTestId("chapter-editor")).toHaveCount(0);
  await page.getByRole("button", { name: "规划一本新书", exact: true }).click();
  const title = `作者旅程 · ${Date.now()}`;
  await page.getByLabel("作品名", { exact: true }).fill(title);
  await page
    .getByLabel("故事的第一个想法")
    .fill("失忆制印师发现自己参与了十年前的旧案。");
  await page.getByLabel("希望写给谁看").fill("喜欢人物推理的读者");
  await page.getByRole("button", { name: "下一步 · 创作方向" }).click();
  await page
    .getByLabel("核心冲突", { exact: true })
    .fill("找回记忆会使最信任的人失去自由。");
  await page.getByLabel("故事主题", { exact: true }).fill("记忆与责任");
  await page.getByRole("button", { name: "保存并进入全书规划" }).click();
  await expect(
    page.getByRole("heading", { name: title, exact: true }),
  ).toBeVisible();
  const pid = await page.evaluate(() => location.hash.split("/")[1]);
  const before = await (await page.request.get(`/api/projects/${pid}`)).json();
  expect(before.chapters).toHaveLength(0);
  const brief = await (
    await page.request.get(`/api/projects/${pid}/studio`)
  ).json();
  expect(brief.brief.data.coreConflict).toContain("找回记忆");
  expect(brief.brief.data.audience).toContain("人物推理");
  await openBookSection(page, "全书规划");
  await expect(page.locator(".book-foundation")).toContainText("找回记忆");
  await page.getByRole("button", { name: "编辑创作方向", exact: true }).click();
  await page.getByRole("button", { name: "和 AI 讨论", exact: true }).click();
  await page
    .getByLabel("给创作伙伴的消息")
    .fill("帮我规划这本书的主线，并提出可采用的全书方案");
  await page.getByRole("button", { name: "发送消息" }).click();
  await expect(
    page.getByRole("button", { name: "采用全书方案", exact: true }),
  ).toBeEnabled({ timeout: 30000 });
  await expect(page.locator(".book-plot-plan")).not.toContainText(
    "全书规划候选",
  );
  const candidate = page.locator('[data-artifact-kind="book-plan-proposal"]');
  const candidateText = await candidate.locator("pre").innerText();
  await candidate
    .getByRole("button", { name: "采用全书方案", exact: true })
    .click();
  await expect(page.locator(".book-plot-plan")).toContainText(
    candidateText.slice(0, 60),
  );
  await expect(
    page.getByRole("button", { name: "保存创作方向", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByText(
      "全书规划刚刚有了更新。请收起后重新编辑，保留最新采用的方案。",
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "收起编辑", exact: true }).click();
  await page.screenshot({
    path: "test-results/author-book-planning.png",
    fullPage: true,
  });
  await page.reload();
  await page.getByRole("button", { name: "和 AI 讨论", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "已采用全书方案", exact: true }),
  ).toBeDisabled();
  const after = await (await page.request.get(`/api/projects/${pid}`)).json();
  expect(after.chapters).toHaveLength(0);
  await openBookSection(page, "人物");
  await page.getByRole("button", { name: "添加人物", exact: true }).click();
  await page.getByLabel("名称", { exact: true }).fill("林川");
  await page
    .getByLabel("人物的欲望、性格与关系")
    .fill("失忆的制印师，渴望查清过去，却害怕伤害养父。");
  await page.getByRole("button", { name: "保存设定", exact: true }).click();
  await expect(page.locator(".book-settings-grid")).toContainText("林川");
  await openBookSection(page, "世界与规则");
  await page.getByRole("button", { name: "添加世界设定", exact: true }).click();
  await page.getByLabel("名称", { exact: true }).fill("记忆铜印");
  await page
    .getByLabel("这个世界如何运作")
    .fill("铜印只能保存亲历者的记忆，不能窥见别人的想法。");
  await page.getByRole("button", { name: "保存设定", exact: true }).click();
  await expect(page.locator(".book-settings-grid")).toContainText("记忆铜印");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/author-book-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "和 AI 讨论", exact: true }).click();
  await expect(page.getByLabel("给创作伙伴的消息")).toBeFocused();
  await expect(page.getByLabel("给创作伙伴的消息")).toBeInViewport({
    ratio: 1,
  });
  await page.screenshot({
    path: "test-results/author-book-mobile-chat.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await page.getByLabel("章节标题", { exact: true }).fill("第一章 旧案");
  await page.getByRole("button", { name: "创建并进入写作" }).click();
  const editor = page.getByTestId("chapter-editor");
  await expect(editor).toBeVisible();
  await editor.fill("林川握住铜印，一段本不属于今天的雨声忽然响起。");
  await page.getByRole("button", { name: "作品概览", exact: true }).click();
  await expect(editor).toHaveCount(0);
  await page.getByRole("button", { name: "继续写作", exact: true }).click();
  await expect(editor).toContainText("本不属于今天的雨声");
  await page.reload();
  await expect(editor).toContainText("本不属于今天的雨声");
  await page.getByRole("button", { name: "返回我的书房", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "每个念头，都值得一本书。" }),
  ).toBeVisible();
  await page.getByLabel("搜索书房作品").fill(title);
  await expect(page.locator(".book-card")).toHaveCount(1);
  await page.screenshot({
    path: "test-results/author-library.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "继续上次写作", exact: true }).click();
  await expect(editor).toContainText("本不属于今天的雨声");
  expect(errors).toEqual([]);
});

test("AI provider settings discover models and keep the API key write only", async ({
  page,
}) => {
  const server = createServer((req, res) => {
    if (
      req.url === "/v1/models" &&
      req.headers.authorization === "Bearer ui-test-only"
    ) {
      res.setHeader("Content-Type", "application/json");
      res.end(
        JSON.stringify({
          data: [{ id: "author-ui-model" }, { id: "author-ui-small" }],
        }),
      );
    } else {
      res.statusCode = 401;
      res.end("test credential missing");
    }
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address() as { port: number };
  const name = `页面供应商验收 · ${Date.now()}`;
  const session = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  try {
    await page.goto("/");
    await openWorkspaceAction(page, "AI 供应商");
    const dialog = page.getByRole("dialog", { name: "AI 供应商设置" });
    await dialog
      .getByRole("button", { name: "添加 AI 供应商", exact: true })
      .click();
    await dialog.getByLabel("供应商名称", { exact: true }).fill(name);
    await dialog
      .getByLabel("API 地址", { exact: true })
      .fill(`http://127.0.0.1:${address.port}/v1`);
    const key = dialog.getByLabel("API 密钥", { exact: true });
    await expect(key).toHaveAttribute("type", "password");
    await key.fill("ui-test-only");
    await dialog
      .getByRole("button", { name: "测试连接并获取模型", exact: true })
      .click();
    await expect(dialog.locator('[role="status"]')).toContainText("2");
    await dialog
      .getByLabel("使用的模型", { exact: true })
      .fill("author-ui-model");
    await dialog.getByLabel("设为默认写作模型").uncheck();
    await dialog
      .getByRole("button", { name: "保存供应商", exact: true })
      .click();
    const item = dialog.locator(".provider-item").filter({ hasText: name });
    await expect(item).toContainText("密钥已配置");
    const response = await page.request.get("/api/provider-settings");
    const raw = await response.text();
    expect(raw).not.toContain("ui-test-only");
    const publicProvider = JSON.parse(raw).providers.find(
      (p: { name: string }) => p.name === name,
    );
    expect(publicProvider.hasKey).toBe(true);
    expect(publicProvider.credentialSource).toBe("stored");
    await item.getByRole("button", { name: "编辑", exact: true }).click();
    await expect(key).toHaveValue("");
    await dialog
      .getByRole("button", { name: "测试连接并获取模型", exact: true })
      .click();
    await expect(dialog.locator('[role="status"]')).toContainText("2");
    await dialog
      .getByLabel("使用的模型", { exact: true })
      .fill("author-ui-small");
    await dialog
      .getByRole("button", { name: "保存供应商", exact: true })
      .click();
    await expect(item).toHaveCount(1);
    await expect(item).toContainText("author-ui-small");
    const replaced = await (
      await page.request.get("/api/provider-settings")
    ).json();
    const replacement = replaced.providers.find(
      (p: { name: string }) => p.name === name,
    );
    expect(replacement.id).not.toBe(publicProvider.id);
    expect(replacement.hasKey).toBe(true);
    await item.getByRole("button", { name: "编辑", exact: true }).click();
    await expect(key).toHaveValue("");
    await page.screenshot({
      path: "test-results/author-provider-settings.png",
      fullPage: true,
    });
    await item.getByRole("button", { name: "停用", exact: true }).click();
    await expect(item).toContainText("已停用");
  } finally {
    const all = await (await page.request.get("/api/provider-settings")).json();
    for (const p of all.providers.filter(
      (p: { name: string }) => p.name === name,
    ))
      await page.request.put(`/api/provider-settings/${p.id}/enabled`, {
        headers,
        data: { enabled: false },
      });
    await new Promise<void>((resolve, reject) =>
      server.close((e) => (e ? reject(e) : resolve())),
    );
  }
});
