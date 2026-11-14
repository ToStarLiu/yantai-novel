import { openChapterMaterial, openWorkspaceAction } from "./navigation";
import { test, expect } from "@playwright/test";
test("Chinese writing, revisions, AI candidates, memory, search and export round trip", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await page
    .getByLabel("作者账户")
    .fill(process.env.E2E_USERNAME || "e2e-author");
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.E2E_PASSWORD || "test-only-novel-2026");
  await page.getByRole("button", { name: "进入书房" }).click();
  await page.getByRole("button", { name: "规划一本新书", exact: true }).click();
  await page.getByLabel("作品名").fill("回声与铜印 · 浏览器验收");
  await page
    .getByLabel("故事的第一个想法")
    .fill("一部用于验证保存、改稿和故事记忆的原创短篇。");
  await page.getByRole("button", { name: "下一步 · 创作方向" }).click();
  await page.getByRole("button", { name: "保存并进入全书规划" }).click();
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await page.getByLabel("章节标题", { exact: true }).fill("第一章 雨中旧桥");
  await page.getByRole("button", { name: "创建并进入写作" }).click();
  const editor = page.getByTestId("chapter-editor");
  await editor.fill(
    "林川左臂受伤，他把唯一的铜印放进胸前内袋。\n\n沈月来到旧桥北岸的磨坊。",
  );
  await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  await page.reload();
  await expect(editor).toContainText("林川左臂受伤");
  await openChapterMaterial(page, "本章章纲");
  await page
    .getByLabel("章纲", { exact: true })
    .fill(
      "保持林川左臂受伤，铜印只有一枚。二人在磨坊查看未开启的木箱，结束在破封之前。",
    );
  await page.getByRole("button", { name: "保存章纲", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("章纲已保存");
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  if (
    (await page.locator(".copilot-writing-tools").getAttribute("open")) === null
  )
    await page.locator(".copilot-writing-tools > summary").click();
  await page.getByRole("button", { name: "依章纲起草" }).click();
  await expect(page.getByText("独立候选 · 尚未采用")).toBeVisible();
  await expect(editor).toContainText("林川左臂受伤");
  await page.getByRole("button", { name: "比较并选择修订" }).click();
  await page.getByRole("button", { name: "全选", exact: true }).click();
  await page.getByRole("button", { name: "保存选中的修订" }).click();
  await expect(page.getByRole("status")).toContainText("新版本");
  await page.getByRole("button", { name: "历史", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByRole("button", { name: /作者编辑/ }).first(),
  ).toBeVisible();
  await page
    .getByRole("button", { name: /作者编辑/ })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "恢复为新版本" }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "恢复为新版本" }).click();
  await expect(page.getByRole("status")).toContainText("历史正文已恢复");
  await page.getByRole("button", { name: "确认定稿" }).click();
  await expect(page.getByText("正文已定稿", { exact: true })).toBeVisible();
  await openChapterMaterial(page, "故事记忆");
  await page.getByRole("button", { name: "提取本章记忆变更" }).click();
  await expect(
    page.getByRole("button", { name: "保存审核决定" }),
  ).toBeVisible();
  const all = page.getByRole("button", { name: "接受全部", exact: true });
  if (await all.isVisible()) await all.click();
  await page.getByRole("button", { name: "保存审核决定" }).click();
  await expect(page.getByRole("status")).toContainText("审核决定已保存");
  await page.getByRole("button", { name: "提交已审核记忆" }).click();
  await expect(page.getByRole("status")).toContainText("记忆变更已提交");
  const downloadPromise = page.waitForEvent("download");
  await openWorkspaceAction(page, "导出完整作品");
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toMatch(/\.json$/);
  await download.saveAs("test-results/browser-project-export.json");
  await page.screenshot({
    path: "test-results/workbench-desktop.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});
test("mobile layout fits the viewport and keeps navigation reachable", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");
  await expect(page.getByRole("button", { name: "进入书房" })).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/login-mobile.png",
    fullPage: true,
  });
});
