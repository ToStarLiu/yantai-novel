import { expect, test, type Page } from "@playwright/test";
import { authenticate } from "./auth";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
const privatePhrase =
  "PRIVATE-TASK-VOICE: 用简短动作承载情绪，不在段尾重复解释。";
let headers: Record<string, string>;
let createdMemory = "";
let originalSettings: {
  version: number;
  useMemory: boolean;
  suggestMemories: boolean;
};
test.beforeEach(async ({ page }) => {
  const session = await authenticate(page);
  headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  originalSettings = await (
    await page.request.get("/api/author-memory-settings")
  ).json();
  const updated = await page.request.put("/api/author-memory-settings", {
    headers,
    data: {
      expectedVersion: originalSettings.version,
      useMemory: true,
      suggestMemories: originalSettings.suggestMemories,
    },
  });
  expect(updated.ok(), await updated.text()).toBeTruthy();
  const memory = await page.request.post("/api/author-memories", {
    headers,
    data: {
      scope: "author",
      kind: "voice",
      title: "单次任务验收 · 私人文风",
      content: privatePhrase,
      pinned: true,
    },
  });
  expect(memory.ok(), await memory.text()).toBeTruthy();
  createdMemory = (await memory.json()).id;
});
test.afterEach(async ({ page }) => {
  const memories = await (
    await page.request.get("/api/author-memories")
  ).json();
  const memory = memories.items.find((item: any) => item.id === createdMemory);
  if (memory)
    await page.request.delete(
      `/api/author-memories/${memory.id}?expectedVersion=${memory.version}`,
      { headers },
    );
  const settings = await (
    await page.request.get("/api/author-memory-settings")
  ).json();
  await page.request.put("/api/author-memory-settings", {
    headers,
    data: {
      expectedVersion: settings.version,
      useMemory: originalSettings.useMemory,
      suggestMemories: originalSettings.suggestMemories,
    },
  });
});
async function setup(page: Page) {
  const projectResponse = await page.request.post("/api/projects", {
    headers,
    data: { title: "单次任务作者记忆 · " + Date.now(), genre: "悬疑" },
  });
  expect(projectResponse.ok(), await projectResponse.text()).toBeTruthy();
  const project = await projectResponse.json();
  const chapterResponse = await page.request.post(
    `/api/projects/${project.id}/chapters`,
    { headers, data: { title: "桥上的来客", volume: "第一卷" } },
  );
  const chapter = await chapterResponse.json();
  const revision = await page.request.post(
    `/api/chapters/${chapter.id}/revisions`,
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
                  text: "雨落在断桥。林川将铜印放回内袋，左臂的伤口仍在疼痛。",
                },
              ],
            },
          ],
        },
      },
    },
  );
  expect(revision.ok(), await revision.text()).toBeTruthy();
  const outline = await page.request.put(`/api/chapters/${chapter.id}`, {
    headers,
    data: {
      title: chapter.title,
      volume: chapter.volume,
      blueprint: "林川在桥头追查铜印来历，保留伤势，不揭示幕后人的身份。",
    },
  });
  expect(outline.ok(), await outline.text()).toBeTruthy();
  await page.goto(`/#write/${project.id}/${chapter.id}`);
  await expect(page.getByTestId("chapter-editor")).toContainText(
    "林川将铜印放回内袋",
  );
  return { project, chapter };
}
async function latest(page: Page, pid: string, kind: string, before = "") {
  let run: any;
  await expect
    .poll(
      async () => {
        const runs = await (
          await page.request.get(`/api/projects/${pid}/runs`)
        ).json();
        run = runs.find(
          (item: any) => item.type === kind && item.id !== before,
        );
        return run?.status;
      },
      { timeout: 30000 },
    )
    .toBe("Succeeded");
  return run;
}
async function expandUsage(usage: import("@playwright/test").Locator) {
  await expect(usage).toBeVisible();
  if ((await usage.getAttribute("open")) === null)
    await usage.locator(":scope > summary").click();
  return usage;
}
async function inspect(page: Page, run: any) {
  const card = page
    .locator(`[data-run-id="${run.id}"]`)
    .filter({ has: page.getByText("本任务的作者记忆", { exact: true }) });
  return expandUsage(card.locator(".task-memory-usage"));
}
test("single writing tasks disclose private preferences, honor opt-out and keep evidence review unpersonalized", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1500, height: 1000 });
  const { project } = await setup(page);
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await page.locator(".copilot-writing-tools > summary").click();
  const control = page.getByLabel("单次写作：使用作者记忆", { exact: true });
  await expect(control).toBeChecked();
  await page.getByRole("button", { name: "依章纲起草", exact: false }).click();
  const first = await latest(page, project.id, "draft");
  expect(first.input.ownerId).toBeTruthy();
  expect(JSON.stringify(first.input)).not.toContain(privatePhrase);
  const use = await inspect(page, first);
  await expect(use).toContainText(privatePhrase);
  expect(
    (
      await (
        await page.request.get(`/api/runs/${first.id}/author-context`)
      ).json()
    ).memory.items.some((item: any) => item.id === createdMemory),
  ).toBeTruthy();
  await control.uncheck();
  await page.getByRole("button", { name: "依章纲起草", exact: false }).click();
  const second = await latest(page, project.id, "draft", first.id);
  expect(second.input.ignoreMemory).toBe(true);
  const skipped = await inspect(page, second);
  await expect(skipped).toContainText("本任务未使用作者记忆");
  await page.getByRole("button", { name: "一致性审稿", exact: true }).click();
  const review = await latest(page, project.id, "review");
  const evidence = await inspect(page, review);
  await expect(evidence).toContainText("不加载作者偏好");
  await expect(evidence).not.toContainText(privatePhrase);
  await page.getByRole("button", { name: "故事记忆", exact: true }).click();
  await expect(page.locator(".task-memory-exclusion")).toContainText(
    "事实提取不会加载作者偏好",
  );
  await page.getByRole("button", { name: "本章章纲", exact: true }).click();
  const planning = page.getByLabel("章纲规划：使用作者记忆", { exact: true });
  await expect(planning).not.toBeChecked();
  await planning.check();
  await page.getByRole("button", { name: "规划建议", exact: true }).click();
  const plan = await latest(page, project.id, "planning");
  await page.getByRole("button", { name: "本章章纲", exact: true }).click();
  await expandUsage(
    page.locator(".task-memory-usage").filter({ visible: true }).first(),
  );
  await expect(
    page.locator(".task-memory-usage").filter({ visible: true }).first(),
  ).toContainText(privatePhrase);
  expect(plan.input.ignoreMemory).toBeFalsy();
  await page.setViewportSize({ width: 390, height: 844 });
  const open = page.getByRole("button", { name: "打开助手", exact: true });
  if (await open.isVisible()) await open.click();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/task-memory-mobile.png",
    fullPage: true,
  });
});
test("production and candidate generation preserve the chosen author memory setting", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1500, height: 1000 });
  const { project, chapter } = await setup(page);
  const flowResponse = await page.request.post("/api/workflows", {
    headers,
    data: {
      name: "记忆制作验收 · " + Date.now(),
      version: 1,
      steps: [
        {
          id: "plan",
          type: "planning",
          promptTemplateId: "",
          agentId: "",
          dependsOn: [],
          condition: "always",
          humanGate: false,
          maxCost: null,
          inputSelection: "current",
        },
      ],
    },
  });
  expect(flowResponse.ok(), await flowResponse.text()).toBeTruthy();
  const flow = await flowResponse.json();
  await page.getByRole("button", { name: "创作工作室", exact: true }).click();
  await page.getByRole("button", { name: "批量制作", exact: false }).click();
  await page
    .getByRole("combobox", { name: "制作流程", exact: true })
    .selectOption(flow.id);
  await page.getByLabel("结束章", { exact: true }).fill("1");
  const control = page.getByLabel("批量制作：使用作者记忆", { exact: true });
  await expect(control).toBeChecked();
  await page.getByRole("button", { name: "创建批量制作", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "批准此步", exact: true }).first(),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "批准此步", exact: true })
    .first()
    .click();
  let batch: any;
  await expect
    .poll(
      async () => {
        const list = await (
          await page.request.get(`/api/projects/${project.id}/productions`)
        ).json();
        batch = list[0];
        return batch?.steps?.[0]?.taskRunId;
      },
      { timeout: 30000 },
    )
    .toBeTruthy();
  const run = await latest(page, project.id, "planning");
  const context = await (
    await page.request.get(`/api/runs/${run.id}/author-context`)
  ).json();
  expect(
    context.memory.items.some((item: any) => item.id === createdMemory),
  ).toBeTruthy();
  await expandUsage(
    page.locator(".production-step .task-memory-usage").first(),
  );
  await expect(
    page.locator(".production-step .task-memory-usage").first(),
  ).toContainText(privatePhrase);
  await page.getByRole("button", { name: "出版与声音", exact: false }).click();
  await page.getByRole("button", { name: "高质量多候选", exact: true }).click();
  await page.getByLabel("多候选创作：使用作者记忆", { exact: true }).uncheck();
  await page
    .getByRole("button", { name: "生成并评价候选", exact: true })
    .click();
  let group: any;
  await expect
    .poll(
      async () => {
        const list = await (
          await page.request.get(`/api/projects/${project.id}/candidates`)
        ).json();
        group = list[0];
        return group?.status;
      },
      { timeout: 30000 },
    )
    .toBe("Succeeded");
  expect(group.chapterId).toBe(chapter.id);
  for (const rid of group.runIds) {
    const current = await (
      await page.request.get(`/api/runs/${rid}/author-context`)
    ).json();
    expect(current.ignoreMemory).toBe(true);
    expect(current.memory.items).toHaveLength(0);
  }
  await page
    .getByRole("button", { name: /候选 [a-z0-9]/ })
    .first()
    .click();
  await expandUsage(
    page.locator(".task-memory-usage").filter({ visible: true }).first(),
  );
  await expect(
    page.locator(".task-memory-usage").filter({ visible: true }).first(),
  ).toContainText("本任务未使用作者记忆");
  await page.screenshot({
    path: "test-results/task-memory-production.png",
    fullPage: true,
  });
});

test("a custom writing role can read author preferences only with its memory capability", async ({
  page,
}) => {
  const roles = [];
  for (const allowed of [false, true]) {
    const created = await page.request.post("/api/agents", {
      headers,
      data: {
        name: "记忆权限角色 · " + String(allowed) + " · " + Date.now(),
        taskType: "draft",
        promptTemplateId: "",
        tools: [
          "read-settings",
          "read-history",
          "create-candidate",
          ...(allowed ? ["query-memory"] : []),
        ],
        maxOutputTokens: 1000,
      },
    });
    expect(created.ok(), await created.text()).toBeTruthy();
    roles.push(await created.json());
  }
  const { project } = await setup(page);
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await page.locator(".copilot-writing-tools > summary").click();
  await page.locator(".generation-settings > summary").click();
  await page
    .getByRole("combobox", { name: "Agent 职责", exact: true })
    .selectOption(roles[0].id);
  await expect(page.locator(".task-memory-exclusion")).toContainText(
    "未获记忆读取权限",
  );
  await page.getByRole("button", { name: "依章纲起草", exact: false }).click();
  const blocked = await latest(page, project.id, "draft");
  const blockedContext = await (
    await page.request.get(`/api/runs/${blocked.id}/author-context`)
  ).json();
  expect(blockedContext.excludedReason).toContain("Agent");
  expect(blockedContext.memory.items).toHaveLength(0);
  await page
    .getByRole("combobox", { name: "Agent 职责", exact: true })
    .selectOption(roles[1].id);
  await expect(
    page.getByLabel("单次写作：使用作者记忆", { exact: true }),
  ).toBeChecked();
  await page.getByRole("button", { name: "依章纲起草", exact: false }).click();
  const allowed = await latest(page, project.id, "draft", blocked.id);
  const use = await inspect(page, allowed);
  await expect(use).toContainText(privatePhrase);
});
