import { expect, test, type Page, type Locator } from "@playwright/test";
import { authenticate } from "./auth";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
let headers: Record<string, string>;
let oldMemory: Set<string>;
test.beforeEach(async ({ page }) => {
  const session = await authenticate(page);
  headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  const list = await (await page.request.get("/api/author-memories")).json();
  oldMemory = new Set(list.items.map((x: any) => x.id));
});
test.afterEach(async ({ page }) => {
  const list = await (await page.request.get("/api/author-memories")).json();
  for (const item of list.items)
    if (!oldMemory.has(item.id))
      await page.request.delete(
        "/api/author-memories/" + item.id + "?expectedVersion=" + item.version,
        { headers },
      );
});
async function fixture(page: Page) {
  const p = await (
    await page.request.post("/api/projects", {
      headers,
      data: { title: "持续写作验收 · " + Date.now(), genre: "悬疑" },
    })
  ).json();
  const c = await (
    await page.request.post("/api/projects/" + p.id + "/chapters", {
      headers,
      data: { title: "断桥下的来客", volume: "第一卷" },
    })
  ).json();
  const r = await page.request.post("/api/chapters/" + c.id + "/revisions", {
    headers,
    data: {
      expectedCurrentRevisionId: c.currentRevisionId,
      source: "author",
      document: {
        type: "doc",
        content: [
          "雨停时，林川已经到了断桥。",
          "他将铜印放回内袋，左臂的伤口没有痊愈。",
          "沈月望着河面，没有回答。",
        ].map((text) => ({
          type: "paragraph",
          content: [{ type: "text", text }],
        })),
      },
    },
  });
  expect(r.ok(), await r.text()).toBeTruthy();
  return { p, c };
}
async function write(page: Page, f: Awaited<ReturnType<typeof fixture>>) {
  await page.goto("/#write/" + f.p.id + "/" + f.c.id);
  await expect(page.getByTestId("chapter-editor")).toContainText("雨停时");
  const open = page.getByRole("button", { name: "打开助手", exact: true });
  if (await open.isVisible()) await open.click();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
}
async function send(page: Page, text: string, count: number) {
  await page.getByLabel("给创作伙伴的消息").fill(text);
  await page.getByRole("button", { name: "发送消息", exact: true }).click();
  await expect(page.getByTestId("copilot-turn")).toHaveCount(count);
  const turn = page.getByTestId("copilot-turn").nth(count - 1);
  await expect(turn.locator(".copilot-speaker")).toContainText("已完成", {
    timeout: 30000,
  });
  return turn;
}
const candidate = (turn: Locator) =>
  turn
    .locator(
      '[data-testid="copilot-artifact"][data-artifact-kind="revision-candidate"]',
    )
    .first();
test("book voice combines private defaults, book instructions, original samples and Skills with conflict protection", async ({
  page,
}) => {
  const f = await fixture(page);
  await page.goto("/#book/" + f.p.id);
  await page.getByRole("button", { name: "文风与写法", exact: true }).click();
  const profile = page.getByRole("region", { name: "文风与写法", exact: true });
  await expect(profile).toContainText("本次明确要求 → 本书设定与写法");
  await profile
    .getByRole("button", { name: "编辑本书写法", exact: true })
    .click();
  await profile.getByLabel("本书文风方向").fill("叙述克制，情绪藏在动作里。");
  await profile
    .getByLabel("本书节奏与句式")
    .fill("短句推进追逐，关系戏留出停顿。");
  await profile.getByLabel("本书避免的写法").fill("不用段尾总结解释人物情绪。");
  await profile
    .getByRole("button", { name: "保存本书写法", exact: true })
    .click();
  await expect(profile).toContainText("叙述克制，情绪藏在动作里。");
  await profile
    .getByRole("button", { name: "添加我的文字样例", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "作者记忆", exact: true });
  await expect(dialog.getByLabel("记忆适用范围", { exact: true })).toHaveValue(
    "book",
  );
  await dialog
    .getByLabel("记忆内容", { exact: true })
    .fill("样例：她把杯子推远了一寸。保留这种用动作表达拒绝的方式。");
  await dialog.getByRole("button", { name: "保存记忆", exact: true }).click();
  await dialog.getByRole("button", { name: "关闭作者记忆" }).click();
  await expect(profile).toContainText("她把杯子推远了一寸");
  await profile
    .getByRole("button", { name: "编辑本书写法", exact: true })
    .click();
  await profile.getByLabel("本书文风方向").fill("未保存的本书文风草稿");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("button", { name: "书房", exact: true }).click();
  await expect(profile.getByLabel("本书文风方向")).toHaveValue(
    "未保存的本书文风草稿",
  );
  await expect(
    page.getByRole("heading", { name: f.p.title, exact: true }),
  ).toBeVisible();
  const studio = await (
    await page.request.get("/api/projects/" + f.p.id + "/studio")
  ).json();
  await page.request.put("/api/projects/" + f.p.id + "/studio/brief", {
    headers,
    data: {
      expectedVersion: studio.brief.version,
      data: { ...studio.brief.data, style: "另一个窗口已更新本书风格" },
    },
  });
  await profile
    .getByRole("button", { name: "保存本书写法", exact: true })
    .click();
  await expect(profile.getByLabel("本书文风方向")).toHaveValue(
    "未保存的本书文风草稿",
  );
  await expect(profile).toContainText("作品规划已在别处更新");
  await profile
    .getByRole("button", { name: "载入最新写法（替换草稿）", exact: true })
    .click();
  await expect(profile.getByLabel("本书文风方向")).toHaveValue(
    "另一个窗口已更新本书风格",
  );
  page.once("dialog", (d) => d.accept());
  await profile.getByRole("button", { name: "取消编辑", exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/writing-profile-mobile.png",
    fullPage: true,
  });
});
test("explicit candidate references produce a revision chain, preserve previous drafts and compare safely", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const f = await fixture(page);
  await write(page, f);
  const first = await send(page, "请改写本章，保持事实和人物口吻", 1);
  const initial = candidate(first);
  await expect(initial).toBeVisible();
  const original = await page.getByTestId("chapter-editor").textContent();
  const firstRun = await initial.getAttribute("data-artifact-run");
  await initial.getByRole("button", { name: "少解释", exact: true }).click();
  await expect(page.getByLabel("本轮修改的候选")).toContainText(
    firstRun!.slice(0, 8),
  );
  await expect(page.getByTestId("copilot-turn")).toHaveCount(1);
  const second = await send(
    page,
    "请继续打磨候选，删掉重复解释，保留有力的动作",
    2,
  );
  const revised = candidate(second);
  await expect(revised).toBeVisible();
  await expect(revised).toContainText("上一稿保留");
  await expect(
    second
      .locator(".copilot-tool")
      .filter({ hasText: "读取待打磨候选" })
      .first(),
  ).toBeVisible();
  await expect(page.getByTestId("chapter-editor")).toHaveText(original);
  const nextRun = await revised.getAttribute("data-artifact-run");
  expect(nextRun).not.toBe(firstRun);
  const run = await (await page.request.get("/api/runs/" + nextRun)).json();
  expect(run.input.candidateRunId).toBe(firstRun);
  await revised
    .getByRole("button", { name: "与上一稿比较", exact: true })
    .click();
  await expect(revised.getByLabel("候选差异")).toContainText(
    "对比上一稿（只读）",
  );
  await expect(
    revised.getByRole("button", { name: "保存选中的修订", exact: true }),
  ).toBeDisabled();
  await revised.getByRole("button", { name: "查看 Diff", exact: true }).click();
  await revised.getByRole("button", { name: "全选修订", exact: true }).click();
  await page.screenshot({
    path: "test-results/candidate-iteration.png",
    fullPage: true,
  });
  await revised
    .getByRole("button", { name: "保存选中的修订", exact: true })
    .click();
  await expect(revised.locator(".pill")).toContainText("已采用");
  await expect(page.getByTestId("chapter-editor")).not.toHaveText(original);
  await expect(
    initial.getByRole("button", { name: "采用完整候选", exact: true }),
  ).toBeDisabled();
});
test("candidate selection targets only that draft and adoption failures never mark it adopted", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const f = await fixture(page);
  await write(page, f);
  const first = await send(page, "请改写本章", 1);
  const initial = candidate(first);
  const preview = initial.locator(".copilot-artifact-preview");
  await preview.selectText();
  await preview.dispatchEvent("mouseup");
  await initial
    .getByRole("button", { name: "只打磨这段", exact: true })
    .click();
  await expect(page.getByLabel("本轮修改的候选")).toContainText(
    "只修改候选中的选段",
  );
  const second = await send(page, "请把引用的候选选段改写得更含蓄", 2);
  const revised = candidate(second);
  await expect(revised).toBeVisible();
  await page.route("**/api/chapters/" + f.c.id + "/revisions", (route) =>
    route.request().method() === "POST"
      ? route.fulfill({
          status: 409,
          contentType: "application/json",
          body: JSON.stringify({ error: "采用冲突测试：正文已更新" }),
        })
      : route.continue(),
  );
  await revised
    .getByRole("button", { name: "采用完整候选", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("采用冲突测试");
  await expect(revised.locator(".pill")).toHaveText("尚未采用");
  await expect(page.getByTestId("chapter-editor")).toContainText("雨停时");
});

test("a single passage rewrite continues as a complete candidate and preserves the surrounding manuscript", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1600, height: 1000 });
  const f = await fixture(page);
  const chapter = await (
    await page.request.get("/api/chapters/" + f.c.id)
  ).json();
  const created = await page.request.post("/api/chapters/" + f.c.id + "/runs", {
    headers,
    data: {
      intentId: "rewrite-continue-" + Date.now(),
      taskType: "rewrite",
      inputRevisionId: chapter.currentRevisionId,
      selection: "他将铜印放回内袋，左臂的伤口没有痊愈。",
      operation: "polish",
      ignoreMemory: true,
    },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const partial = await created.json();
  await expect
    .poll(
      async () =>
        (await (await page.request.get("/api/runs/" + partial.id)).json())
          .status,
    )
    .toBe("Succeeded");
  await write(page, f);
  await page.locator(".copilot-writing-tools > summary").click();
  await page
    .getByRole("button", { name: "和创作伙伴继续打磨", exact: true })
    .click();
  await expect(page.getByLabel("本轮修改的候选")).toContainText(
    partial.id.slice(0, 8),
  );
  const next = await send(page, "请继续打磨这份选段候选，保留其他段落。", 1);
  const complete = candidate(next);
  await expect(complete).toBeVisible();
  await expect(complete.locator(".copilot-artifact-preview")).toContainText(
    "雨停时，林川已经到了断桥。",
  );
  await expect(complete.locator(".copilot-artifact-preview")).toContainText(
    "沈月望着河面，没有回答。",
  );
  await complete
    .getByRole("button", { name: "与上一稿比较", exact: true })
    .click();
  await expect(complete.getByLabel("候选差异")).toContainText(
    "对比上一稿（只读）",
  );
  const removed = await complete.locator(".copilot-hunk del").allTextContents();
  expect(removed.join("")).not.toContain("雨停时，林川已经到了断桥。");
  expect(removed.join("")).not.toContain("沈月望着河面，没有回答。");
  await expect(page.getByTestId("chapter-editor")).toContainText(
    "他将铜印放回内袋，左臂的伤口没有痊愈。",
  );
});
