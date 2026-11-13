import { expect, test, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { authenticate } from "./auth";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
let headers: Record<string, string>;
let previousIds: Set<string>;
test.beforeEach(async ({ page }) => {
  const session = await authenticate(page);
  headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  const list = await (await page.request.get("/api/author-memories")).json();
  previousIds = new Set(list.items.map((x: any) => x.id));
  await page.request.put("/api/author-memory-settings", {
    headers,
    data: {
      expectedVersion: list.settings.version,
      useMemory: true,
      suggestMemories: true,
    },
  });
});
test.afterEach(async ({ page }) => {
  const list = await (await page.request.get("/api/author-memories")).json();
  for (const item of list.items)
    if (!previousIds.has(item.id))
      await page.request.delete(
        "/api/author-memories/" + item.id + "?expectedVersion=" + item.version,
        { headers },
      );
  const settings = await (
    await page.request.get("/api/author-memory-settings")
  ).json();
  await page.request.put("/api/author-memory-settings", {
    headers,
    data: {
      expectedVersion: settings.version,
      useMemory: true,
      suggestMemories: true,
    },
  });
});
async function book(page: Page, label: string) {
  const pr = await page.request.post("/api/projects", {
    headers,
    data: {
      title: "作者记忆验收 · " + label + " · " + Date.now(),
      genre: "悬疑",
    },
  });
  expect(pr.ok(), await pr.text()).toBeTruthy();
  const project = await pr.json();
  const cr = await page.request.post(
    "/api/projects/" + project.id + "/chapters",
    { headers, data: { title: "雨夜的来客", volume: "第一卷" } },
  );
  const chapter = await cr.json();
  const save = await page.request.post(
    "/api/chapters/" + chapter.id + "/revisions",
    {
      headers,
      data: {
        expectedCurrentRevisionId: chapter.currentRevisionId || "",
        source: "author",
        document: {
          type: "doc",
          content: [
            {
              type: "paragraph",
              content: [
                {
                  type: "text",
                  text: "雨落在断桥，林川将铜印放回内袋，左臂的伤口尚未痊愈。",
                },
              ],
            },
          ],
        },
      },
    },
  );
  expect(save.ok(), await save.text()).toBeTruthy();
  return { project, chapter };
}
async function openBook(page: Page, b: Awaited<ReturnType<typeof book>>) {
  await page.goto("/#write/" + b.project.id + "/" + b.chapter.id);
  await expect(page.getByTestId("chapter-editor")).toContainText("雨落在断桥");
  const open = page.getByRole("button", { name: "打开助手", exact: true });
  if (await open.isVisible()) await open.click();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
}
async function send(page: Page, text: string, count: number) {
  await page.getByLabel("给创作伙伴的消息").fill(text);
  await page.getByRole("button", { name: "发送消息", exact: true }).click();
  await expect(page.getByTestId("copilot-turn")).toHaveCount(count, {
    timeout: 30000,
  });
  await expect(
    page.getByRole("button", { name: "停止回答", exact: true }),
  ).not.toBeVisible({ timeout: 30000 });
  const last = page.getByTestId("copilot-turn").last();
  await expect(last.locator(".copilot-speaker")).toContainText("已完成", {
    timeout: 30000,
  });
  return last;
}
async function openMemory(page: Page) {
  await page.getByRole("button", { name: "作者记忆", exact: true }).click();
  return page.getByRole("dialog", { name: "作者记忆", exact: true });
}
test("author memory supports manual voice, editing, export/import, settings and mobile", async ({
  page,
}) => {
  await page.goto("/#library");
  const dialog = await openMemory(page);
  await dialog.getByRole("button", { name: "记下一条", exact: true }).click();
  await dialog.getByLabel("记忆类别", { exact: true }).selectOption("voice");
  await dialog
    .getByLabel("记忆标题", { exact: true })
    .fill("记忆验收 · 克制的对白");
  await dialog
    .getByLabel("记忆内容", { exact: true })
    .fill("用简短对白和具体动作表现犹豫，减少段尾总结。");
  await dialog.getByRole("button", { name: "保存记忆", exact: true }).click();
  const card = dialog
    .locator(".author-memory-card")
    .filter({ hasText: "记忆验收 · 克制的对白" });
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "编辑", exact: true }).click();
  await dialog
    .getByLabel("记忆内容", { exact: true })
    .fill("用简短对白和具体动作表现犹豫，保留作者原有措辞。");
  await dialog.getByRole("button", { name: "保存记忆", exact: true }).click();
  await expect(card).toContainText("保留作者原有措辞");
  const downloading = page.waitForEvent("download");
  await dialog.getByRole("link", { name: "导出记忆", exact: true }).click();
  const download = await downloading;
  const exported = JSON.parse(await readFile((await download.path())!, "utf8"));
  expect(exported.formatVersion).toBe(1);
  expect(
    exported.items.some((x: any) => x.content.includes("保留作者原有措辞")),
  ).toBeTruthy();
  expect(
    exported.items.every(
      (x: any) => !x.ownerId && !x.sourceTurnId && !x.sourceQuote,
    ),
  ).toBeTruthy();
  await dialog.getByLabel("选择作者记忆文件").setInputFiles({
    name: "memories.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify(exported)),
  });
  await expect(dialog.getByLabel("作者记忆导入预览")).toBeVisible();
  await dialog
    .getByRole("button", { name: "确认导入记忆", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("跳过");
  await dialog.getByRole("button", { name: "记忆设置", exact: true }).click();
  await dialog.getByLabel("在创作对话中使用作者记忆").uncheck();
  await dialog
    .getByRole("button", { name: "保存记忆设置", exact: true })
    .click();
  await expect(dialog).toContainText("已暂停使用记忆");
  await page.screenshot({
    path: "test-results/author-memory-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await expect(
    dialog.getByRole("button", { name: "关闭作者记忆" }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/author-memory-mobile.png",
    fullPage: true,
  });
  await dialog.getByRole("button", { name: "关闭作者记忆" }).click();
});
test("confirmed author preference follows across books, book-only memory stays scoped, and round opt-out clears it", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const first = await book(page, "一"),
    second = await book(page, "二");
  await openBook(page, first);
  await send(
    page,
    "我喜欢简短的对话，让动作承载潜台词，不要解释人物已知的信息。",
    1,
  );
  const suggestion = page.getByTestId("author-memory-proposal").first();
  await expect(suggestion).toContainText("值得以后记住");
  const remember = suggestion.getByRole("button", {
    name: "记住这条",
    exact: true,
  });
  await expect(remember).toBeEnabled();
  await remember.click();
  await expect(suggestion).toContainText("已记住");
  const created = await page.request.post("/api/author-memories", {
    headers,
    data: {
      scope: "book",
      projectId: first.project.id,
      kind: "voice",
      title: "记忆验收 · 本书独有语气",
      content: "仅这本书采用潮汐意象，另一部作品不沿用。",
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const local = await created.json();
  const turn = await send(page, "请检查本章人物的可知信息", 2);
  await expect(turn.locator(".copilot-turn-memory")).toContainText(
    "本轮已加载",
  );
  await expect(
    turn.locator(".copilot-tool").filter({ hasText: "查阅作者记忆" }).first(),
  ).toBeVisible();
  await page
    .getByLabel("选择作品", { exact: true })
    .selectOption(second.project.id);
  await expect(
    page.getByRole("heading", { name: second.project.title, exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "继续写作", exact: true }).click();
  await expect(page.getByTestId("chapter-editor")).toContainText("雨落在断桥");
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  const across = await send(page, "请检查本章人物的可知信息", 1);
  await across.locator(".copilot-turn-memory summary").click();
  await expect(across.locator(".copilot-turn-memory")).toContainText(
    "动作承载潜台词",
  );
  await expect(across.locator(".copilot-turn-memory")).not.toContainText(
    local.content,
  );
  await page.getByLabel("本轮不使用记忆", { exact: true }).check();
  const ignored = await send(page, "请检查本章人物的可知信息", 2);
  await expect(ignored.locator(".copilot-turn-memory")).toContainText(
    "本轮未使用作者记忆",
  );
  await expect(
    ignored.locator(".copilot-tool").filter({ hasText: "查阅作者记忆" }),
  ).toHaveCount(0);
  await page.getByLabel("本轮不使用记忆", { exact: true }).uncheck();
  await page.screenshot({
    path: "test-results/author-memory-conversation.png",
    fullPage: true,
  });
});
test("manual conversation bookmark preserves exact evidence and forgetting invalidates generated candidates", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const b = await book(page, "对话记忆");
  await openBook(page, b);
  const first = await send(
    page,
    "我们先比较两个雨夜重逢的方向，再决定下一步。",
    1,
  );
  await first.getByRole("button", { name: "记住这句话", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "作者记忆", exact: true });
  await expect(dialog.getByLabel("记忆内容", { exact: true })).toHaveValue(
    "我们先比较两个雨夜重逢的方向，再决定下一步。",
  );
  await dialog
    .getByLabel("记忆标题", { exact: true })
    .fill("记忆验收 · 雨夜讨论");
  await dialog.getByRole("button", { name: "保存记忆", exact: true }).click();
  await dialog.getByRole("button", { name: "关闭作者记忆" }).click();
  const turn = await send(page, "请继续雨夜重逢，改写得更含蓄", 2);
  const candidate = turn
    .getByTestId("copilot-artifact")
    .filter({
      has: page.getByRole("button", { name: "采用完整候选", exact: true }),
    })
    .first();
  await expect(candidate).toBeVisible();
  await candidate.getByRole("button", { name: "少解释", exact: true }).click();
  await expect(page.getByLabel("给创作伙伴的消息")).toHaveValue(/删去重复解释/);
  await expect(page.getByTestId("copilot-turn")).toHaveCount(2);
  const manage = await openMemory(page);
  const card = manage
    .locator(".author-memory-card")
    .filter({ hasText: "记忆验收 · 雨夜讨论" });
  await card.getByRole("button", { name: "忘记", exact: true }).click();
  await card.getByRole("button", { name: "确认忘记", exact: true }).click();
  await expect(card).toHaveCount(0);
  await manage.getByRole("button", { name: "关闭作者记忆" }).click();
  await page.reload();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "采用完整候选", exact: true }),
  ).toBeDisabled();
  const next = await send(page, "雨夜重逢有什么其他推进方式？", 3);
  await expect(next.locator(".copilot-turn-memory")).toContainText(
    "本轮已加载 0 条作者记忆",
  );
});

test("memory conflict keeps the author's draft and reloads the current version explicitly", async ({
  page,
}) => {
  const created = await page.request.post("/api/author-memories", {
    headers,
    data: {
      scope: "author",
      kind: "workflow",
      title: "记忆验收 · 协作习惯",
      content: "先给两个方向，再展开。",
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const memory = await created.json();
  await page.goto("/#library");
  const dialog = await openMemory(page);
  const card = dialog
    .locator(".author-memory-card")
    .filter({ hasText: "记忆验收 · 协作习惯" });
  await card.getByRole("button", { name: "编辑", exact: true }).click();
  await dialog
    .getByLabel("记忆内容", { exact: true })
    .fill("我的未保存草稿：先说明取舍，再给方向。");
  const changed = await page.request.put("/api/author-memories/" + memory.id, {
    headers,
    data: {
      expectedVersion: memory.version,
      content: "另一个窗口：先问目标，再展开。",
    },
  });
  expect(changed.ok(), await changed.text()).toBeTruthy();
  await dialog.getByRole("button", { name: "保存记忆", exact: true }).click();
  await expect(dialog.locator(".author-memory-conflict")).toContainText(
    "草稿仍保留",
  );
  await expect(dialog.getByLabel("记忆内容", { exact: true })).toHaveValue(
    "我的未保存草稿：先说明取舍，再给方向。",
  );
  await expect(
    dialog.getByRole("button", { name: "保存记忆", exact: true }),
  ).toBeDisabled();
  await dialog
    .getByRole("button", { name: "重新载入最新版本（替换草稿）", exact: true })
    .click();
  await expect(dialog.getByLabel("记忆内容", { exact: true })).toHaveValue(
    "另一个窗口：先问目标，再展开。",
  );
  await dialog
    .getByRole("button", { name: "收起记忆编辑", exact: true })
    .click();
  await card.getByRole("button", { name: "暂时停用", exact: true }).click();
  await dialog
    .getByRole("button", { name: "已停用与未采用", exact: true })
    .click();
  await expect(card).toContainText("已停用");
  await card.getByRole("button", { name: "重新启用", exact: true }).click();
  await dialog.getByRole("button", { name: /已记住/ }).click();
  await expect(card).toBeVisible();
  await dialog
    .getByRole("button", { name: "关闭作者记忆", exact: true })
    .click();
});
