import { expect, test } from "@playwright/test";
import { authenticate } from "./auth";

const doc = (text: string) => ({
  type: "doc",
  content: [{ type: "paragraph", content: [{ type: "text", text }] }],
});
async function seed(page: import("@playwright/test").Page) {
  const session = await authenticate(page);
  const headers = {
    "X-CSRF-Token": session.csrfToken,
    Origin: process.env.E2E_BASE_URL || "http://127.0.0.1:8089",
  };
  const project = await (
    await page.request.post("/api/projects", {
      headers,
      data: {
        title: "Copilot 对话验收 · " + Date.now(),
        description: "",
        genre: "悬疑",
      },
    })
  ).json();
  const chapter = await (
    await page.request.post(`/api/projects/${project.id}/chapters`, {
      headers,
      data: { title: "第一章 旧桥", volume: "第一卷" },
    })
  ).json();
  const saved = await page.request.post(
    `/api/chapters/${chapter.id}/revisions`,
    {
      headers,
      data: {
        expectedCurrentRevisionId: chapter.currentRevisionId || "",
        document: doc("林川左臂受伤，他把铜印放进胸前内袋。沈月站在旧桥北岸。"),
        source: "author",
      },
    },
  );
  expect(saved.ok(), await saved.text()).toBeTruthy();
  await page.goto(`/#write/${project.id}/${chapter.id}`);
  await expect(page.getByTestId("chapter-editor")).toContainText(
    "林川左臂受伤",
  );
  return { project, chapter };
}
async function send(
  page: import("@playwright/test").Page,
  text: string,
  count: number,
) {
  const input = page.getByLabel("给创作伙伴的消息");
  await expect(page.getByRole("button", { name: "发送消息" })).toBeVisible();
  await input.fill(text);
  await input.press("Enter");
  await expect(page.getByTestId("copilot-turn")).toHaveCount(count, {
    timeout: 30000,
  });
  await expect(page.getByRole("button", { name: "停止回答" })).not.toBeVisible({
    timeout: 30000,
  });
}
test("Copilot keeps tool conversation, candidate diff and replay after refresh", async ({
  page,
}) => {
  await seed(page);
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await send(page, "请检查本章人物的可知信息", 1);
  await expect(page.locator(".copilot-tool").first()).toBeVisible();
  await send(page, "请改写得更含蓄，但保持原来的事实", 2);
  const candidate = page
    .getByTestId("copilot-artifact")
    .filter({
      has: page.getByRole("button", { name: "查看 Diff", exact: true }),
    })
    .first();
  await expect(candidate).toBeVisible();
  await candidate
    .getByRole("button", { name: "查看 Diff", exact: true })
    .click();
  await expect(candidate.getByLabel("候选差异")).toBeVisible();
  await page.screenshot({
    path: "test-results/copilot-diff.png",
    fullPage: true,
  });
  await page.reload();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await expect(page.getByTestId("copilot-turn")).toHaveCount(2);
  await expect(page.locator(".copilot-tool").first()).toBeVisible();
  await expect(page.getByTestId("chapter-editor")).toContainText(
    "林川左臂受伤",
  );
  await page.screenshot({
    path: "test-results/copilot-conversation.png",
    fullPage: true,
  });
  const restoredCandidate = page
    .getByTestId("copilot-artifact")
    .filter({
      has: page.getByRole("button", { name: "查看 Diff", exact: true }),
    })
    .first();
  const artifactRun = await restoredCandidate.getAttribute("data-artifact-run");
  await restoredCandidate
    .getByRole("button", { name: "查看 Diff", exact: true })
    .click();
  await restoredCandidate
    .getByRole("button", { name: "全选修订", exact: true })
    .click();
  await restoredCandidate
    .getByRole("button", { name: "保存选中的修订", exact: true })
    .click();
  await expect(
    page.locator(
      `[data-artifact-run="${artifactRun}"][data-artifact-kind="revision-candidate"] .pill`,
    ),
  ).toContainText("已采用");
  await expect(page.getByRole("status")).toContainText("新版本");
});
test("Copilot preserves selection, cancelled output, blueprint and compaction", async ({
  page,
}) => {
  await seed(page);
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await page.getByTestId("chapter-editor").selectText();
  await expect(page.locator(".copilot-selection")).toBeVisible();
  const input = page.getByLabel("给创作伙伴的消息");
  await input.fill("请慢速讨论这段文字");
  await input.press("Enter");
  await expect(page.getByRole("button", { name: "停止回答" })).toBeVisible();
  await page.reload();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await expect(page.getByRole("button", { name: "停止回答" })).toBeVisible();
  await page.getByRole("button", { name: "停止回答" }).click();
  await expect(
    page.getByText("已停止，以上内容和调用记录已保留。"),
  ).toBeVisible({ timeout: 20000 });
  await send(page, "请提出本章章纲", 2);
  await expect(
    page.getByRole("button", { name: "采用章纲建议", exact: true }),
  ).toBeEnabled();
  await send(page, "继续比较刚才的两个发展方向", 3);
  await page.getByRole("button", { name: "压缩对话上下文" }).click();
  await expect(page.getByText("上下文已整理，原对话仍保留。")).toBeVisible({
    timeout: 30000,
  });
  await page.screenshot({
    path: "test-results/copilot-compaction.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "采用章纲建议", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("章纲建议已采用");
});
test("Copilot remains usable at mobile width", async ({ page }) => {
  await seed(page);
  await page.setViewportSize({ width: 390, height: 844 });
  const open = page.getByRole("button", { name: "打开助手", exact: true });
  if (await open.isVisible()) await open.click();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await expect(page.getByLabel("给创作伙伴的消息")).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/copilot-mobile.png",
    fullPage: true,
  });
});
