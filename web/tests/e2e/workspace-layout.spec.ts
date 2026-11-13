import { expect, test, type Page } from "@playwright/test";
import { authenticate } from "./auth";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
const paragraphs = [
  "雨停的时候，林川走到了断桥下。远处传来船笛声，沈月却没有回头。",
  "他把铜印放回内袋，左臂的伤口牵扯着旧衬衣。他没出声，只将右手搭在桥栏上。",
  "“你说，那封信是谁寄来的？”沈月问。",
  "林川看着桥墩上的水痕。潮水退了一寸，最下面的一道刻痕终于露出来。",
  "他忽然明白，有人早在他们之前来过这里。",
];
async function fixture(page: Page) {
  const auth = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": auth.csrfToken };
  const p = await page.request.post("/api/projects", {
    headers,
    data: {
      title: "工作台交互验收 · " + Date.now(),
      genre: "悬疑",
      description: "原创布局验收",
    },
  });
  expect(p.ok(), await p.text()).toBeTruthy();
  const project = await p.json();
  const chapters = [];
  for (let i = 0; i < 5; i++) {
    const r = await page.request.post(
      "/api/projects/" + project.id + "/chapters",
      {
        headers,
        data: {
          title: [
            "雨中的来客",
            "没有署名的信",
            "断桥下",
            "无人知晓的约定",
            "铜印的回声",
          ][i],
          volume: i < 3 ? "第一卷 · 旧城来信" : "第二卷 · 潮水回声",
        },
      },
    );
    expect(r.ok()).toBeTruthy();
    chapters.push(await r.json());
  }
  const c = chapters[2];
  const r = await page.request.post("/api/chapters/" + c.id + "/revisions", {
    headers,
    data: {
      expectedCurrentRevisionId: c.currentRevisionId || "",
      source: "author",
      document: {
        type: "doc",
        content: paragraphs.map((text) => ({
          type: "paragraph",
          content: [{ type: "text", text }],
        })),
      },
    },
  });
  expect(r.ok(), await r.text()).toBeTruthy();
  await page.goto("/#write/" + project.id + "/" + c.id);
  await expect(page.getByTestId("chapter-editor")).toContainText("雨停的时候");
  return { project, chapters };
}
test("workspace navigation, resizable panels, focus and reading preferences retain the manuscript", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.setViewportSize({ width: 1600, height: 1000 });
  await fixture(page);
  const directory = page.getByRole("complementary", { name: "章节目录" });
  await expect(
    page.getByRole("navigation", { name: "创作导航" }),
  ).toBeVisible();
  const search = page.getByLabel("搜索章节", { exact: true });
  await search.fill("铜印");
  await expect(
    directory.getByRole("button", { name: /铜印的回声/ }),
  ).toBeVisible();
  await expect(
    directory.getByRole("button", { name: /雨中的来客/ }),
  ).toHaveCount(0);
  await search.fill("");
  const volume = directory.getByRole("button", { name: /第一卷 · 旧城来信/ });
  await volume.click();
  await expect(
    directory.getByRole("button", { name: /雨中的来客/ }),
  ).toHaveCount(0);
  await volume.click();
  await expect(
    directory.getByRole("button", { name: /雨中的来客/ }),
  ).toBeVisible();
  const divider = page.getByRole("separator", { name: "调整章节目录宽度" });
  const before = Number(await divider.getAttribute("aria-valuenow"));
  await divider.focus();
  await divider.press("ArrowRight");
  await expect(divider).toHaveAttribute("aria-valuenow", String(before + 16));
  const right = page.getByRole("separator", { name: "调整创作伙伴宽度" });
  await right.focus();
  await right.press("ArrowLeft");
  const width = await right.getAttribute("aria-valuenow");
  await page.getByRole("button", { name: "正文排版", exact: true }).click();
  await page.getByLabel("正文字号", { exact: true }).fill("21");
  await page.getByLabel("正文行距", { exact: true }).fill("2.4");
  await page.getByLabel("正文字体", { exact: true }).selectOption("sans");
  await page.getByRole("button", { name: "关闭正文排版" }).click();
  await expect(page.getByTestId("chapter-editor")).toHaveCSS(
    "font-size",
    "21px",
  );
  await page.getByRole("button", { name: "收起章节目录", exact: true }).click();
  await expect(directory).not.toBeVisible();
  await page.getByRole("button", { name: "展开正文目录", exact: true }).click();
  await expect(directory).toBeVisible();
  await page.getByLabel("给创作伙伴的消息").fill("保留这段尚未发送的作者想法");
  await page.getByRole("button", { name: "收起助手", exact: true }).click();
  await page.getByRole("button", { name: "打开助手", exact: true }).click();
  await expect(page.getByLabel("给创作伙伴的消息")).toHaveValue(
    "保留这段尚未发送的作者想法",
  );
  await page.getByRole("button", { name: "专注写作", exact: true }).click();
  await expect(directory).not.toBeVisible();
  await expect(
    page.getByRole("complementary", { name: "章节创作伙伴" }),
  ).not.toBeVisible();
  await expect(page.getByTestId("chapter-editor")).toContainText(paragraphs[0]);
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: "专注写作", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(page.getByTestId("chapter-editor")).toContainText(paragraphs[0]);
  await expect(page.getByTestId("chapter-editor")).toHaveCSS(
    "font-size",
    "21px",
  );
  await expect(
    page.getByRole("separator", { name: "调整创作伙伴宽度" }),
  ).toHaveAttribute("aria-valuenow", width!);
  await page.screenshot({
    path: "test-results/workspace-desktop.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
});
test("selection actions prepare a message and quick commands preserve failed-save drafts", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1500, height: 960 });
  const { chapters } = await fixture(page);
  const editor = page.getByTestId("chapter-editor");
  await editor.selectText();
  await expect(
    page.getByRole("toolbar", { name: "选段创作操作" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "润色当前选段", exact: true }).click();
  const input = page.getByLabel("给创作伙伴的消息");
  await expect(input).toHaveValue(/润色当前选段/);
  await expect(page.getByTestId("copilot-turn")).toHaveCount(0);
  await expect(page.locator(".copilot-selection")).toBeVisible();
  await page.getByRole("button", { name: "移除本次选段", exact: true }).click();
  await editor.focus();
  await editor.selectText();
  await page.getByRole("button", { name: "扩写当前选段", exact: true }).click();
  await expect(page.locator(".copilot-selection")).toBeVisible();
  await expect(input).toHaveValue(/润色当前选段[\s\S]*扩写当前选段/);
  await page.getByRole("button", { name: "本章章纲", exact: true }).click();
  await page.getByRole("button", { name: "灵感助手", exact: true }).click();
  await expect(input).toHaveValue(/润色当前选段/);
  const opener = page.getByRole("button", { name: "快捷操作", exact: true });
  await opener.click();
  const box = page.getByRole("dialog", { name: "快捷操作" });
  await expect(
    page.getByRole("combobox", { name: "搜索快捷操作" }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(opener).toBeFocused();
  await page.keyboard.press("Control+k");
  await page
    .getByRole("combobox", { name: "搜索快捷操作" })
    .fill("管理写作 Skills");
  await page.keyboard.press("Enter");
  const skillsDialog = page.getByRole("dialog", {
    name: "写作 Skills 管理",
    exact: true,
  });
  await expect(skillsDialog).toBeVisible();
  await expect(skillsDialog.locator("button:focus")).toHaveCount(1);
  await skillsDialog
    .getByRole("button", { name: "关闭写作 Skills", exact: true })
    .click();
  await page.keyboard.press("Control+k");
  await page.getByRole("combobox", { name: "搜索快捷操作" }).fill("铜印的回声");
  await page.keyboard.press("Enter");
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    "铜印的回声",
  );
  await expect(box).not.toBeVisible();
  const current = chapters[4];
  await page.route("**/api/chapters/" + current.id + "/revisions", (route) =>
    route.request().method() === "POST"
      ? route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "验收保存失败" }),
        })
      : route.continue(),
  );
  await editor.fill("这段内容保存失败也必须留在原章。");
  await opener.click();
  await page
    .getByRole("combobox", { name: "搜索快捷操作" })
    .fill("返回我的书房");
  await page.keyboard.press("Enter");
  await expect(box.getByRole("alert")).toBeVisible();
  await expect(editor).toContainText("必须留在原章");
  await expect(page.getByLabel("章节标题", { exact: true })).toHaveValue(
    "铜印的回声",
  );
  await page.keyboard.press("Escape");
  await page.unroute("**/api/chapters/" + current.id + "/revisions");
  await page.keyboard.press("Control+s");
  await expect(page.locator(".save-status")).toContainText("已保存");
});
test("workspace stays usable on a narrow desktop and mobile", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 850 });
  await fixture(page);
  await page.getByRole("button", { name: "打开助手", exact: true }).click();
  await expect(page.getByLabel("给创作伙伴的消息")).toBeVisible();
  await page.getByRole("button", { name: "收起创作伙伴", exact: true }).click();
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "打开章节导航", exact: true }).click();
  await expect(page.getByLabel("搜索章节", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "收起目录", exact: true }).click();
  await page.getByRole("button", { name: "打开助手", exact: true }).click();
  await expect(page.getByLabel("给创作伙伴的消息")).toBeVisible();
  await page.screenshot({
    path: "test-results/workspace-mobile.png",
    fullPage: true,
  });
  await page.getByRole("button", { name: "收起创作伙伴", exact: true }).click();
  await page.getByRole("button", { name: "快捷操作", exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "搜索快捷操作" }),
  ).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(390);
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 1280, height: 850 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth),
  ).toBeLessThanOrEqual(1280);
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
});
