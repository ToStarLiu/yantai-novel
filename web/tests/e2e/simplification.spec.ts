import { expect, test, type Page } from "@playwright/test";
import { authenticate } from "./auth";
import { openBookSection, openWorkspaceAction } from "./navigation";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";

async function fixture(page: Page, count = 3) {
  const auth = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": auth.csrfToken };
  const projectResponse = await page.request.post("/api/projects", {
    headers,
    data: { title: "收敛交互验收 · " + Date.now(), genre: "悬疑" },
  });
  expect(projectResponse.ok(), await projectResponse.text()).toBeTruthy();
  const project = await projectResponse.json();
  const planned = await page.request.put(
    "/api/projects/" + project.id + "/studio/brief",
    {
      headers,
      data: {
        expectedVersion: 0,
        data: { targetWords: 100000, targetChapters: 30 },
      },
    },
  );
  expect(planned.ok(), await planned.text()).toBeTruthy();
  const chapters = [];
  for (let index = 0; index < count; index++) {
    const response = await page.request.post(
      "/api/projects/" + project.id + "/chapters",
      {
        headers,
        data: {
          title:
            ["第一章 雨夜来信", "第二章 渡口的约定", "第三章 没有署名的信"][
              index
            ] || "下一章",
          volume: "第一卷",
        },
      },
    );
    expect(response.ok()).toBeTruthy();
    const chapter = await response.json();
    if (index < 2) {
      const saved = await page.request.post(
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
                    { type: "text", text: "林川收好那封信，往渡口走去。" },
                  ],
                },
              ],
            },
          },
        },
      );
      expect(saved.ok(), await saved.text()).toBeTruthy();
      chapter.currentRevisionId = (await saved.json()).id;
    }
    chapters.push(chapter);
  }
  return { project, chapters, headers, userId: auth.user.id };
}

test("book overview resumes each book independently and counts only matching final drafts", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1500, height: 960 });
  const { project, chapters, headers } = await fixture(page);
  for (const chapter of chapters.slice(0, 2)) {
    const final = await page.request.post(
      "/api/chapters/" + chapter.id + "/finalize",
      { headers, data: { revisionId: chapter.currentRevisionId } },
    );
    expect(final.ok(), await final.text()).toBeTruthy();
  }
  const revised = await page.request.post(
    "/api/chapters/" + chapters[1].id + "/revisions",
    {
      headers,
      data: {
        expectedCurrentRevisionId: chapters[1].currentRevisionId,
        source: "author",
        document: {
          type: "doc",
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "林川没有赴约。他把信放回原处。" },
              ],
            },
          ],
        },
      },
    },
  );
  expect(revised.ok(), await revised.text()).toBeTruthy();
  const other = await fixture(page, 1);
  await page.goto("/#write/" + project.id + "/" + chapters[2].id);
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    chapters[2].title,
  );
  // A later visit to another book must not replace this book's continuation point.
  await page
    .getByLabel("选择作品", { exact: true })
    .selectOption(other.project.id);
  await expect(
    page.getByRole("heading", { name: other.project.title, exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "继续写作", exact: true }).click();
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    other.chapters[0].title,
  );
  await openWorkspaceAction(page, "返回我的书房");
  await page.locator(".book-card").filter({ hasText: project.title }).click();
  const nav = page.getByRole("navigation", { name: "作品导航", exact: true });
  await expect(nav.getByRole("button")).toHaveCount(3);
  await expect(
    page.getByRole("region", { name: "下一步写作", exact: true }),
  ).toContainText(chapters[2].title);
  await expect(
    page.getByRole("region", { name: "写作进度", exact: true }),
  ).toContainText("2 / 3 章已保存正文");
  await expect(
    page.getByRole("region", { name: "写作进度", exact: true }),
  ).toContainText("1 章当前稿已定稿");
  await expect(page.getByLabel("给创作伙伴的消息")).toHaveCount(0);
  await page.screenshot({
    path: "test-results/simplified-book-desktop.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "继续写作", exact: true }).click();
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    chapters[2].title,
  );
  await page
    .getByTestId("chapter-editor")
    .fill("这次回来的仍是第三章。未发送的修改先保存。");
  await page.getByRole("button", { name: "作品概览", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: project.title, exact: true }),
  ).toBeVisible();
  const saved = await (
    await page.request.get("/api/chapters/" + chapters[2].id)
  ).json();
  expect(saved.revision.plainText).toContain("仍是第三章");
});

test("book discussion is optional, restores its draft, and empty books keep one next action", async ({
  page,
}) => {
  const { project } = await fixture(page, 0);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#book/" + project.id);
  await expect(
    page.getByRole("navigation", { name: "作品导航" }).getByRole("button"),
  ).toHaveCount(3);
  await expect(
    page.getByRole("button", { name: "准备第一章", exact: true }),
  ).toHaveCount(1);
  await expect(page.getByLabel("给创作伙伴的消息")).toHaveCount(0);
  await page.screenshot({
    path: "test-results/simplified-book-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "和 AI 讨论", exact: true }).click();
  const input = page.getByLabel("给创作伙伴的消息", { exact: true });
  await expect(input).toBeFocused();
  await expect(input).toBeInViewport({ ratio: 1 });
  await input.fill("这是还没有发送的故事想法，不要丢掉。");
  await page
    .getByRole("button", { name: "收起全书创作伙伴", exact: true })
    .click();
  await expect(input).not.toBeVisible();
  await openBookSection(page, "人物");
  await page.getByRole("button", { name: "和 AI 讨论", exact: true }).click();
  await expect(input).toHaveValue("这是还没有发送的故事想法，不要丢掉。");
  await expect(input).toBeFocused();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
});

test("writing keeps three assistant destinations and secondary tools stay reachable by keyboard", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const { project, chapters } = await fixture(page);
  await page.setViewportSize({ width: 1500, height: 960 });
  await page.goto("/#write/" + project.id + "/" + chapters[1].id);
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  await expect(
    page.getByRole("navigation", { name: "创作导航" }).getByRole("button"),
  ).toHaveCount(4);
  const primary = page.getByRole("navigation", {
    name: "创作伙伴工具",
    exact: true,
  });
  await expect(primary.getByRole("button")).toHaveCount(3);
  const input = page.getByLabel("给创作伙伴的消息", { exact: true });
  await input.fill("保留这个未发送的章节问题。");
  await page.getByRole("button", { name: "本章资料", exact: true }).click();
  await page.getByRole("button", { name: "故事记忆", exact: true }).click();
  await page.getByRole("button", { name: "任务记录", exact: true }).click();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await expect(input).toHaveValue("保留这个未发送的章节问题。");
  const more = page.getByRole("button", { name: "更多操作", exact: true });
  await more.click();
  const actions = page.getByRole("dialog", { name: "作品与设置", exact: true });
  await expect(
    actions.getByRole("button", { name: "作品概览", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("End");
  await expect(
    actions.getByRole("button", { name: "退出登录", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(actions).toHaveCount(0);
  await expect(more).toBeFocused();
  await more.click();
  await actions.getByRole("button", { name: "作者记忆", exact: true }).click();
  const memory = page.getByRole("dialog", { name: "作者记忆", exact: true });
  await expect(memory).toBeVisible();
  await memory
    .getByRole("button", { name: "关闭作者记忆", exact: true })
    .click();
  await expect(input).toHaveValue("保留这个未发送的章节问题。");
  await page.screenshot({
    path: "test-results/simplified-workspace-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "收起创作伙伴", exact: true }).click();
  await more.click();
  await expect(
    actions.getByRole("button", { name: "AI 供应商", exact: true }),
  ).toBeInViewport({ ratio: 1 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.keyboard.press("Escape");
  await page.screenshot({
    path: "test-results/simplified-workspace-mobile.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("overview memory reminders open the relevant material without requiring navigation searches", async ({
  page,
}) => {
  const { project, chapters, headers } = await fixture(page, 3);
  const final = await page.request.post(
    "/api/chapters/" + chapters[0].id + "/finalize",
    { headers, data: { revisionId: chapters[0].currentRevisionId } },
  );
  expect(final.ok(), await final.text()).toBeTruthy();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/#write/" + project.id + "/" + chapters[2].id);
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    chapters[2].title,
  );
  await openWorkspaceAction(page, "作品概览");
  const attention = page.getByRole("region", {
    name: "待处理事项",
    exact: true,
  });
  await expect(attention).toContainText("记忆待确认");
  await attention
    .getByRole("button", { name: /有已定稿章节的记忆待确认/ })
    .click();
  const assistant = page.getByRole("complementary", {
    name: "章节创作伙伴",
    exact: true,
  });
  await expect(assistant).toBeVisible();
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    chapters[0].title,
  );
  await expect(
    assistant.getByRole("button", { name: "本章资料", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    assistant.getByRole("button", { name: "故事记忆", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});

test("unsaved planning is resolved before creating a chapter and its title is retained", async ({
  page,
}) => {
  const { project } = await fixture(page, 0);
  await page.goto("/#book/" + project.id);
  await openBookSection(page, "全书规划");
  await page.getByRole("button", { name: "编辑创作方向", exact: true }).click();
  await page
    .getByLabel("核心冲突", { exact: true })
    .fill("查明旧案，会伤害最信任的人。");
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await page.getByLabel("章节标题", { exact: true }).fill("第一章 未寄出的信");
  await page
    .getByRole("button", { name: "创建并进入写作", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "核心冲突", exact: true }),
  ).toHaveValue("查明旧案，会伤害最信任的人。");
  const before = await (
    await page.request.get("/api/projects/" + project.id)
  ).json();
  expect(before.chapters).toHaveLength(0);
  await page.getByRole("button", { name: "保存创作方向", exact: true }).click();
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    "第一章 未寄出的信",
  );
  await page
    .getByRole("button", { name: "创建并进入写作", exact: true })
    .click();
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    "第一章 未寄出的信",
  );
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  const after = await (
    await page.request.get("/api/projects/" + project.id)
  ).json();
  expect(after.chapters).toHaveLength(1);
});

test("cancelling a hidden voice draft confirmation creates no chapter", async ({
  page,
}) => {
  const { project } = await fixture(page, 0);
  await page.goto("/#book/" + project.id);
  await openBookSection(page, "文风与写法");
  const profile = page.getByRole("region", { name: "文风与写法", exact: true });
  await profile
    .getByRole("button", { name: "编辑本书写法", exact: true })
    .click();
  await profile
    .getByLabel("本书文风方向", { exact: true })
    .fill("这是尚未保存的本书写法。");
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await page
    .getByLabel("章节标题", { exact: true })
    .fill("第一章 留下来的约定");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page
    .getByRole("button", { name: "创建并进入写作", exact: true })
    .click();
  const unchanged = await (
    await page.request.get("/api/projects/" + project.id)
  ).json();
  expect(unchanged.chapters).toHaveLength(0);
  await openBookSection(page, "文风与写法");
  await expect(profile.getByLabel("本书文风方向", { exact: true })).toHaveValue(
    "这是尚未保存的本书写法。",
  );
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  let confirmations = 0;
  page.on("dialog", async (dialog) => {
    confirmations++;
    await dialog.accept();
  });
  await page
    .getByRole("button", { name: "创建并进入写作", exact: true })
    .click();
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  expect(confirmations).toBe(1);
  const created = await (
    await page.request.get("/api/projects/" + project.id)
  ).json();
  expect(created.chapters).toHaveLength(1);
});

test("rebuild reminders follow the invalidated chapter queue instead of the resume position", async ({
  page,
}) => {
  const { project, chapters, headers } = await fixture(page, 3);
  let response = await page.request.post(
    "/api/chapters/" + chapters[0].id + "/finalize",
    { headers, data: { revisionId: chapters[0].currentRevisionId } },
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  response = await page.request.post(
    "/api/chapters/" + chapters[0].id + "/revisions",
    {
      headers,
      data: {
        expectedCurrentRevisionId: chapters[0].currentRevisionId,
        source: "author",
        document: {
          type: "doc",
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "他把信交给了沈月，改变了先前的约定。" },
              ],
            },
          ],
        },
      },
    },
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  const revision = await response.json();
  response = await page.request.post(
    "/api/chapters/" + chapters[0].id + "/finalize",
    { headers, data: { revisionId: revision.id } },
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  await page.goto("/#write/" + project.id + "/" + chapters[2].id);
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    chapters[2].title,
  );
  await openWorkspaceAction(page, "作品概览");
  const reminder = page
    .getByRole("region", { name: "待处理事项", exact: true })
    .getByRole("button", { name: /故事记忆需要复核/ });
  await expect(reminder).toContainText(chapters[0].title);
  await reminder.click();
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    chapters[0].title,
  );
  await expect(
    page.getByRole("button", { name: "故事记忆", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
});
