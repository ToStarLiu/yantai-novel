import { authenticate } from "./auth";
import { openWorkspaceAction } from "./navigation";
import { test, expect, type Page } from "@playwright/test";
import { createServer, type Server } from "node:http";

const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
async function setup(page: Page) {
  const session = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  const post = async (path: string, data: unknown) => {
    const r = await page.request.post("/api" + path, { headers, data });
    expect(r.ok(), await r.text()).toBeTruthy();
    return r.json();
  };
  const project = await post("/projects", {
    title: "联网与协作验收 · " + Date.now(),
    description: "独立浏览器测试作品",
    genre: "悬疑",
  });
  const chapter = await post(`/projects/${project.id}/chapters`, {
    title: "断桥",
    volume: "第一卷",
  });
  const revision = await post(`/chapters/${chapter.id}/revisions`, {
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
              text: "林川在断桥边找到一枚铜印。他的左臂仍在疼痛，雨水顺着桥栏落下。",
            },
          ],
        },
      ],
    },
  });
  await page.goto(`/#write/${project.id}/${chapter.id}`);
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  return { headers, post, project, chapter, revision };
}
async function studio(page: Page, label: string) {
  await openWorkspaceAction(page, "创作工作室");
  await page.getByRole("button", { name: label, exact: false }).click();
}
test("canvas-specific scene proposals, causal explanation, hook reports and voice adoption remain explicit", async ({
  page,
}) => {
  const { project, chapter, revision } = await setup(page);
  await studio(page, "剧情画布");
  await page.getByRole("button", { name: "添加节点", exact: true }).click();
  await page.getByLabel("节点标题").fill("铜印发出细响");
  await page
    .getByLabel("节点内容")
    .fill("林川在不打开木箱的前提下，核对铜印与箱盖的刻纹。");
  await page.getByRole("button", { name: "由这个节点生成场景" }).click();
  await expect(
    page.getByRole("button", { name: "确认加入场景序列" }),
  ).toBeEnabled({ timeout: 20000 });
  await page.getByLabel("采用场景名").fill("箱前核印");
  await page.getByRole("button", { name: "确认加入场景序列" }).click();
  await expect(page.getByRole("status")).toContainText("加入场景序列");
  const scenes = (
    await (await page.request.get(`/api/projects/${project.id}/studio`)).json()
  ).scenes.data.scenes;
  expect(scenes[0].title).toBe("箱前核印");
  expect(scenes[0].nodeIds.length).toBe(1);
  expect(scenes[0].boardVersion).toBeGreaterThan(0);
  await page.getByRole("button", { name: "解释节点因果关系" }).click();
  await expect(
    page.getByRole("heading", { name: "因果解释候选", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "出版与声音", exact: false }).click();
  await page.getByRole("button", { name: "虚拟编辑部", exact: true }).click();
  await page
    .getByRole("button", { name: "作者声音 提炼声音与表达边界" })
    .click();
  await expect(
    page.getByRole("button", { name: "采用为作者声音档案" }),
  ).toBeEnabled({ timeout: 20000 });
  await page.getByLabel("声音档案名称").fill("铜印之声");
  await page.getByRole("button", { name: "采用为作者声音档案" }).click();
  await expect(page.getByRole("status")).toContainText("作品文风档案");
  const detail = await (
    await page.request.get(`/api/projects/${project.id}`)
  ).json();
  expect(
    detail.settings.some(
      (s: any) => s.kind === "style" && s.name === "铜印之声",
    ),
  ).toBe(true);
  await page
    .getByRole("button", { name: "章末钩子 悬念、回报与继续阅读动力" })
    .click();
  await expect
    .poll(async () => {
      const runs = await (
        await page.request.get(`/api/projects/${project.id}/runs`)
      ).json();
      return runs.find((r: any) => r.input.feature === "hook")?.status;
    })
    .toBe("Succeeded");
  expect(
    (await (await page.request.get(`/api/chapters/${chapter.id}`)).json())
      .currentRevisionId,
  ).toBe(revision.id);
  await page.screenshot({
    path: "test-results/studio-editorial.png",
    fullPage: true,
  });
});

test("book planning adoption, immutable planning history, isolated overlap and compressed context", async ({
  page,
}) => {
  const { project, revision } = await setup(page);
  await studio(page, "故事规划");
  await page.getByLabel("规划层级").selectOption("book");
  const runResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/runs") &&
      response.request().method() === "POST",
    { timeout: 10000 },
  );
  await page
    .getByRole("button", { name: "生成全书规划候选", exact: true })
    .click();
  const queued = await runResponse;
  expect(queued.ok(), await queued.text()).toBeTruthy();
  await expect
    .poll(async () => {
      const runs = await (
        await page.request.get(`/api/projects/${project.id}/runs`)
      ).json();
      return runs.some(
        (r: any) =>
          r.type === "planning" &&
          r.status === "Succeeded" &&
          r.input.scope === "book",
      );
    })
    .toBe(true);
  await expect(page.getByLabel("已有规划候选").locator("option")).toHaveCount(
    2,
    { timeout: 15000 },
  );
  await page.getByLabel("已有规划候选").selectOption({ index: 1 });
  await page
    .getByRole("button", { name: "采用为全书计划", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("作者采用");
  const doc = await (
    await page.request.get(`/api/projects/${project.id}/studio`)
  ).json();
  expect(doc.brief.data.plotPlan).toBeTruthy();
  await page.getByRole("button", { name: "规划历史", exact: true }).click();
  await expect(page.locator(".studio-comments")).toContainText("版本");
  await page
    .locator(".studio-comments")
    .getByRole("button", { name: "恢复为新版本" })
    .last()
    .click();
  await expect(page.getByRole("status")).toContainText("恢复为新版本");
  await page.locator(".studio-comments .icon-button").click();
  await page.getByRole("button", { name: "参考资料", exact: false }).click();
  await page.getByLabel("参考资料名").fill("原文隔离证据");
  await page
    .getByLabel("参考资料正文")
    .fill("林川在断桥边找到一枚铜印。他的左臂仍在疼痛，雨水顺着桥栏落下。");
  await page
    .getByRole("button", { name: "分析并保存资料", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("参考资料");
  await page.getByLabel("研究主题").fill("公开资料抓取验收");
  await page
    .getByLabel("公开 URL（最多 5 条，每行一条）")
    .fill("https://example.com");
  const researchRequest = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/projects/${project.id}/research`) &&
      response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "抓取选定公开网页" }).click();
  const researchResponse = await researchRequest;
  expect(researchResponse.ok(), await researchResponse.text()).toBeTruthy();
  const research = await researchResponse.json();
  expect(research.sources).toHaveLength(1);
  expect(research.sources[0].url).toBe("https://example.com");
  expect(
    !!research.sources[0].referenceId || !!research.sources[0].error,
  ).toBeTruthy();
  await page.getByRole("button", { name: "检查当前稿与参考原文重合" }).click();
  await expect(page.locator(".studio-result").last()).toContainText(
    "原文隔离证据",
  );
  await page.getByRole("button", { name: "模型与检索", exact: false }).click();
  await page
    .getByRole("button", { name: "语义检索与压缩", exact: true })
    .click();
  await page.getByLabel("检索问题").fill("铜印");
  await page.getByLabel("检索模式").selectOption("keyword");
  const searchRequest = page.waitForResponse((response) =>
    response.url().endsWith("/semantic-search"),
  );
  await page.getByRole("button", { name: "对照召回结果", exact: true }).click();
  const searchResponse = await searchRequest;
  expect(searchResponse.ok(), await searchResponse.text()).toBeTruthy();
  expect(Array.isArray(await searchResponse.json())).toBe(true);
  const compression = page.waitForResponse((response) =>
    response.url().endsWith("/context-compression"),
  );
  await page.getByRole("button", { name: "生成带来源的压缩上下文" }).click();
  const compressed = await compression;
  expect(compressed.ok(), await compressed.text()).toBeTruthy();
  const summary = await compressed.json();
  expect(summary.derived).toBe(true);
  expect(summary.projectId).toBe(project.id);
  expect(revision.id).toBeTruthy();
  await page.screenshot({
    path: "test-results/studio-research.png",
    fullPage: true,
  });
});

test("invitation joins an isolated viewer with real permission enforcement and shared presence", async ({
  page,
  browser,
}) => {
  const { post, project, chapter } = await setup(page);
  await studio(page, "协作与权限");
  await expect(page.getByText("你的权限：所有者")).toBeVisible();
  await page.getByLabel("邀请权限").selectOption("viewer");
  await page.getByRole("button", { name: "创建邀请", exact: true }).click();
  const token = await page.locator(".invitation-token code").innerText();
  expect(token.length).toBeGreaterThan(20);
  const viewer = await browser.newContext();
  const p = await viewer.newPage();
  await p.goto(origin);
  await p.getByRole("button", { name: "使用邀请加入作品" }).click();
  await p.getByLabel("邀请令牌").fill(token);
  await p.getByLabel("显示名称").fill("只读协作者");
  const username = "viewer-" + Date.now();
  await p.getByLabel("作者账户").fill(username);
  await p.getByLabel("密码", { exact: true }).fill("browser-viewer-only-2026");
  await p.getByRole("button", { name: "接受邀请并进入" }).click();
  await expect(
    p.getByRole("heading", { name: "每个念头，都值得一本书。" }),
  ).toBeVisible();
  await p.locator(".book-card").filter({ hasText: project.title }).click();
  await expect(
    p.getByRole("heading", { name: project.title, exact: true }),
  ).toBeVisible();
  await expect(p.getByTestId("chapter-editor")).toHaveCount(0);
  await p.getByRole("button", { name: "继续写作", exact: true }).click();
  await expect(p.getByTestId("chapter-editor")).toBeVisible();
  await expect(p.getByTestId("chapter-editor")).toHaveAttribute(
    "contenteditable",
    "false",
  );
  const s = await (await p.request.get("/api/session")).json();
  const denial = await p.request.post(`/api/chapters/${chapter.id}/revisions`, {
    headers: { Origin: origin, "X-CSRF-Token": s.csrfToken },
    data: {
      expectedCurrentRevisionId: "",
      source: "author",
      document: { type: "doc", content: [] },
    },
  });
  expect(denial.status()).toBe(403);
  await expect
    .poll(async () => {
      const presence = await (
        await page.request.get(`/api/projects/${project.id}/presence`)
      ).json();
      return presence.some(
        (person: { username: string; chapterId: string }) =>
          person.username === username && person.chapterId === chapter.id,
      );
    })
    .toBe(true);
  await openWorkspaceAction(p, "创作工作室");
  await p.getByRole("button", { name: "协作与权限", exact: false }).click();
  await expect(p.getByText("你的权限：只读者")).toBeVisible();
  await expect(
    p.getByRole("button", { name: "创建邀请", exact: true }),
  ).toHaveCount(0);
  await expect
    .poll(async () => {
      const members = await (
        await page.request.get(`/api/projects/${project.id}/members`)
      ).json();
      return members.some(
        (m: any) => m.username === username && m.role === "viewer",
      );
    })
    .toBe(true);
  await page.getByRole("button", { name: "剧情画布", exact: false }).click();
  await page.getByRole("button", { name: "协作与权限", exact: false }).click();
  await page.screenshot({
    path: "test-results/studio-collaboration.png",
    fullPage: true,
  });
  await viewer.close();
});

test("MCP discovers typed tools and executes only an explicitly permitted read-only tool", async ({
  page,
}) => {
  let server: Server | undefined;
  let port = 0;
  let savedConfigID = "";
  const calls: string[] = [];
  server = createServer((request, response) => {
    let body = "";
    request.on("data", (c) => (body += c));
    request.on("end", () => {
      const msg = JSON.parse(body);
      calls.push(msg.method);
      if (msg.method.startsWith("notifications/")) {
        response.writeHead(202);
        response.end();
        return;
      }
      let result: unknown = {};
      if (msg.method === "initialize")
        result = {
          protocolVersion: "2025-11-25",
          capabilities: { tools: {} },
          serverInfo: { name: "browser-fixture", version: "1" },
        };
      if (msg.method === "tools/list")
        result = {
          tools: [
            {
              name: "read_clock",
              description: "只读时钟证据",
              inputSchema: {
                type: "object",
                properties: { label: { type: "string" } },
                required: ["label"],
              },
              annotations: { readOnlyHint: true, destructiveHint: false },
            },
            {
              name: "delete_all",
              description: "禁止的写操作",
              inputSchema: { type: "object" },
              annotations: { readOnlyHint: false, destructiveHint: true },
            },
          ],
        };
      if (msg.method === "tools/call")
        result = {
          content: [
            { type: "text", text: "只读结果：" + msg.params.arguments.label },
          ],
          isError: false,
        };
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ jsonrpc: "2.0", id: msg.id, result }));
    });
  });
  await new Promise<void>((resolve) =>
    server!.listen(0, "127.0.0.1", () => {
      port = (server!.address() as any).port;
      resolve();
    }),
  );
  try {
    await setup(page);
    await studio(page, "模型与检索");
    await page.getByRole("button", { name: "MCP 工具", exact: true }).click();
    await page
      .getByLabel("名称", { exact: true })
      .fill("浏览器只读工具 " + Date.now());
    await page.getByLabel("服务 URL").fill(`http://127.0.0.1:${port}/mcp`);
    await page.getByLabel("允许工具名").fill("read_clock");
    await page.getByLabel("明确允许本机回环 HTTP 服务").check();
    const configRequest = page.waitForResponse(
      (response) =>
        response.url().endsWith("/mcp-configs") &&
        response.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "保存 MCP 服务", exact: true })
      .click();
    savedConfigID = (await (await configRequest).json()).id;
    await expect(page.getByRole("status")).toContainText("MCP 服务配置已保存");
    await page.getByRole("button", { name: "发现工具与类型定义" }).click();
    await expect(page.getByText("只读时钟证据")).toBeVisible();
    await expect(page.getByText("禁止的写操作")).toBeVisible();
    await page.getByRole("button", { name: "选择此授权工具" }).first().click();
    await page.getByLabel("类型化参数（JSON）").fill('{"label":"雨夜线索"}');
    await page.getByRole("button", { name: "执行授权只读调用" }).click();
    await expect(page.locator(".studio-main")).toContainText(
      "只读结果：雨夜线索",
    );
    expect(calls).toContain("tools/list");
    expect(calls).toContain("tools/call");
    await page.screenshot({
      path: "test-results/studio-mcp.png",
      fullPage: true,
    });
  } finally {
    if (savedConfigID) {
      const current = await (await page.request.get("/api/session")).json();
      await page.request.put(`/api/mcp-configs/${savedConfigID}/enabled`, {
        headers: { Origin: origin, "X-CSRF-Token": current.csrfToken },
        data: { enabled: false },
      });
    }
    await new Promise<void>((resolve) => server!.close(() => resolve()));
  }
});
