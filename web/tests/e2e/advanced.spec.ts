import { openChapterMaterial } from "./navigation";
import { test, expect } from "@playwright/test";
test("planning, rewrite, handoff, style profiles and budgets remain author controlled", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page
    .getByLabel("作者账户")
    .fill(process.env.E2E_USERNAME || "e2e-author");
  await page
    .getByLabel("密码", { exact: true })
    .fill(process.env.E2E_PASSWORD || "test-only-novel-2026");
  await page.getByRole("button", { name: "进入书房" }).click();
  await page.getByRole("button", { name: "规划一本新书", exact: true }).click();
  await page.getByLabel("作品名").fill("写作工具 · " + Date.now());
  await page.getByRole("button", { name: "下一步 · 创作方向" }).click();
  await page.getByRole("button", { name: "保存并进入全书规划" }).click();
  await page.getByRole("button", { name: "卷章目录", exact: true }).click();
  await page.getByLabel("章节标题", { exact: true }).fill("第一章 雨声");
  await page.getByRole("button", { name: "创建并进入写作" }).click();
  const editor = page.getByTestId("chapter-editor");
  await editor.fill("林川望向旧桥，雨水正沿着铜印滑落。");
  await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  await openChapterMaterial(page, "本章章纲");
  await page.getByRole("button", { name: "规划建议" }).click();
  await openChapterMaterial(page, "本章章纲");
  await expect(
    page.getByRole("button", { name: "填入章纲编辑区" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "填入章纲编辑区" }).click();
  await page.getByRole("button", { name: "保存章纲", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("章纲已保存");
  await openChapterMaterial(page, "人物设定");
  await page.getByRole("button", { name: "文风", exact: true }).click();
  await page.getByLabel("文风模板").selectOption({ index: 1 });
  await expect(page.getByLabel("设定名称")).not.toHaveValue("");
  await page.getByRole("button", { name: "添加设定", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("设定已保存");
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  if (
    (await page.locator(".copilot-writing-tools").getAttribute("open")) === null
  )
    await page.locator(".copilot-writing-tools > summary").click();
  await page.getByRole("button", { name: "本地规则检查 · 不调用模型" }).click();
  await expect(page.getByText(/规则检查 · \d+ 条提示/)).toBeVisible();
  await editor.selectText();
  await page.getByRole("button", { name: "生成候选", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "比较并选择修订" }),
  ).toBeVisible();
  await expect(editor).toContainText("林川望向旧桥");
  await page.getByRole("button", { name: "生成场景交接摘要" }).click();
  await expect(
    page.locator(".run-card").filter({ hasText: "场景交接" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "比较并选择修订" }),
  ).toHaveCount(0);
  await expect(editor).toContainText("林川望向旧桥");
  await page.getByRole("button", { name: "任务记录", exact: true }).click();
  await page.getByRole("button", { name: "预算设置" }).click();
  await page.getByLabel("币种", { exact: true }).fill("CNY");
  await page.getByLabel("单次输出 Token 上限").fill("1800");
  await page.getByRole("button", { name: "保存预算", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("预算设置已保存");
  await page.screenshot({
    path: "test-results/advanced-writing-tools.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});
