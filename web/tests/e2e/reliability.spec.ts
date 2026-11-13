import { authenticate } from "./auth";
import { test, expect } from "@playwright/test";
const doc = (text: string) => ({
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text }] }],
});
async function seed(page: import("@playwright/test").Page) {
  const session = await authenticate(page);
  const headers = {
    "X-CSRF-Token": session.csrfToken,
    Origin: "http://127.0.0.1:8089",
  };
  const response = await page.request.post("/api/projects", {
    headers,
    data: {
      title: "版本与输入隔离 · " + Date.now(),
      description: "可靠性浏览器测试",
      genre: "测试",
    },
  });
  const project = await response.json();
  const chapter = await (
    await page.request.post(`/api/projects/${project.id}/chapters`, {
      headers,
      data: { title: "第一章 旧桥", volume: "第一卷" },
    })
  ).json();
  await page.request.post(`/api/chapters/${chapter.id}/revisions`, {
    headers,
    data: {
      expectedCurrentRevisionId: chapter.currentRevisionId || "",
      document: doc("最初的正文。"),
      source: "author",
    },
  });
  await page.goto(`/#write/${project.id}/${chapter.id}`);
  await expect(page.getByTestId("chapter-editor")).toContainText(
    "最初的正文。",
  );
  return { headers, project, chapter };
}
test("failed saves retain local text and can retry without losing the buffer", async ({
  page,
}) => {
  const { chapter } = await seed(page);
  let fail = true;
  await page.route(`**/api/chapters/${chapter.id}/revisions`, async (route) => {
    if (route.request().method() === "POST" && fail)
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "模拟网络保存失败" }),
      });
    else await route.continue();
  });
  const editor = page.getByTestId("chapter-editor");
  await editor.fill("保存失败也必须留住的中文段落。");
  await expect(page.getByText("保存失败", { exact: true })).toBeVisible();
  await expect(editor).toContainText("保存失败也必须留住");
  expect(
    await page.evaluate(
      (id) => localStorage.getItem("novel-draft-" + id),
      chapter.id,
    ),
  ).toContain("保存失败也必须留住");
  fail = false;
  await page.getByRole("button", { name: "重试保存" }).click();
  await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  await page.reload();
  await expect(editor).toContainText("保存失败也必须留住");
});
test("concurrent editing returns a visible conflict and never replaces the other version", async ({
  page,
}) => {
  const { headers, chapter } = await seed(page);
  const current = await (
    await page.request.get(`/api/chapters/${chapter.id}`)
  ).json();
  const external = await page.request.post(
    `/api/chapters/${chapter.id}/revisions`,
    {
      headers,
      data: {
        expectedCurrentRevisionId: current.currentRevisionId,
        document: doc("另一个页面已经保存的正文。"),
        source: "author",
      },
    },
  );
  expect(external.ok()).toBeTruthy();
  const editor = page.getByTestId("chapter-editor");
  await editor.fill("当前页面尚未合并的编辑。");
  await expect(page.getByText("版本冲突", { exact: true })).toBeVisible();
  await expect(editor).toContainText("当前页面尚未合并");
  const after = await (
    await page.request.get(`/api/chapters/${chapter.id}`)
  ).json();
  expect(after.revision.plainText).toContain("另一个页面已经保存");
  await page.screenshot({
    path: "test-results/conflict-buffer.png",
    fullPage: true,
  });
});
test("text import previews chapters and full project export restores as a new project", async ({
  page,
}) => {
  const { project } = await seed(page);
  await page.getByRole("button", { name: "导入作品" }).click();
  await page
    .getByPlaceholder("也可以直接粘贴已有正文…")
    .fill(
      "# 第二章 晨光\n\n沈月从磨坊出发。\n\n# 第三章 渡口\n\n林川仍然带着铜印。",
    );
  await page.getByRole("button", { name: "预览分章", exact: true }).click();
  await expect(page.getByLabel("导入第1章标题")).toBeVisible();
  await page.getByRole("button", { name: "确认导入到当前作品" }).click();
  await expect(page.getByRole("status")).toContainText("文字已导入");
  const detail = await (
    await page.request.get(`/api/projects/${project.id}`)
  ).json();
  expect(detail.chapters.length).toBe(3);
  const exportJSON = await (
    await page.request.get(`/api/projects/${project.id}/export`)
  ).json();
  await page.getByRole("button", { name: "导入作品" }).click();
  const importedResponse = page.waitForResponse(
    (r) => r.url().endsWith("/api/import") && r.request().method() === "POST",
  );
  await page.locator("input[type=file]").setInputFiles({
    name: "project.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify(exportJSON)),
  });
  await expect(page.getByRole("status")).toContainText("文字已导入");
  const response = await importedResponse;
  expect(response.ok()).toBeTruthy();
  const restored = (await response.json()).project;
  expect(restored.id).not.toBe(project.id);
  const restoredDetail = await (
    await page.request.get(`/api/projects/${restored.id}`)
  ).json();
  expect(restoredDetail.chapters.length).toBe(3);
  const restoredChapter = await (
    await page.request.get(`/api/chapters/${restoredDetail.chapters[0].id}`)
  ).json();
  expect(restoredChapter.revision.plainText).toContain("最初的正文。");
});

test("an AI candidate cannot replace a newer author revision", async ({
  page,
}) => {
  const { headers, project, chapter } = await seed(page);
  await page.request.put(`/api/chapters/${chapter.id}`, {
    headers,
    data: {
      title: chapter.title,
      volume: chapter.volume,
      blueprint: "在旧桥边发现线索，结尾停在破封之前。",
    },
  });
  await page.reload();
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  if (
    (await page.locator(".copilot-writing-tools").getAttribute("open")) === null
  )
    await page.locator(".copilot-writing-tools > summary").click();
  await page.getByRole("button", { name: "依章纲起草" }).click();
  await expect(
    page.getByRole("button", { name: "比较并选择修订" }),
  ).toBeEnabled();
  const runs = await (
    await page.request.get(`/api/projects/${project.id}/runs`)
  ).json();
  const run = runs[0];
  await page.getByTestId("chapter-editor").fill("作者在生成之后写下的新版本。");
  await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "比较并选择修订" }),
  ).toBeDisabled();
  const latest = await (
    await page.request.get(`/api/chapters/${chapter.id}`)
  ).json();
  const attempt = await page.request.post(
    `/api/chapters/${chapter.id}/revisions`,
    {
      headers,
      data: {
        expectedCurrentRevisionId: latest.currentRevisionId,
        document: doc(run.output.text),
        source: "ai",
        runId: run.id,
      },
    },
  );
  expect(attempt.status()).toBe(409);
  const after = await (
    await page.request.get(`/api/chapters/${chapter.id}`)
  ).json();
  expect(after.revision.plainText).toContain("作者在生成之后写下的新版本。");
});

test("volume editing, readable export and explicit project deletion work", async ({
  page,
}) => {
  const { project } = await seed(page);
  page.once("dialog", (d) => d.accept("第二卷 归途"));
  await page.getByRole("button", { name: "编辑卷名" }).click();
  await expect(page.getByRole("status")).toContainText("卷名已保存");
  await page.getByRole("button", { name: "作品设置" }).click();
  let download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Markdown", exact: true }).click();
  expect((await download).suggestedFilename()).toMatch(/\.md$/);
  download = page.waitForEvent("download");
  await page.getByRole("button", { name: "TXT", exact: true }).click();
  expect((await download).suggestedFilename()).toMatch(/\.txt$/);
  page.once("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "删除这部作品" }).click();
  await expect(page.getByRole("status")).toContainText("作品已删除");
  expect((await page.request.get(`/api/projects/${project.id}`)).status()).toBe(
    404,
  );
});

test("mobile author can open and close the assistant and reach the chapter tree", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await seed(page);
  await expect(page.getByRole("button", { name: "打开助手" })).toBeVisible();
  await page.getByRole("button", { name: "打开助手" }).click();
  await expect(page.getByRole("heading", { name: "创作伙伴" })).toBeVisible();
  await page.getByRole("button", { name: "收起创作伙伴" }).click();
  await expect(page.getByRole("heading", { name: "创作伙伴" })).toHaveCount(0);
  await page.getByRole("button", { name: "打开章节导航" }).click();
  await expect(
    page.getByRole("button", { name: "01 第一章 旧桥" }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/workbench-mobile.png",
    fullPage: true,
  });
});
