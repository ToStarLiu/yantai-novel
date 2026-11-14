import { authenticate } from "./auth";
import { openWorkspaceAction } from "./navigation";
import { test, expect } from "@playwright/test";
async function setup(page: import("@playwright/test").Page) {
  const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
  const session = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  const post = async (path: string, data: unknown) => {
    const r = await page.request.post("/api" + path, { headers, data });
    expect(r.ok(), await r.text()).toBeTruthy();
    return r.json();
  };
  const project = await post("/projects", {
    title: "Studio 全流程 · " + Date.now(),
    description: "高级创作工作室验收",
    genre: "悬疑",
  });
  const character = await post(`/projects/${project.id}/settings`, {
    kind: "character",
    name: "林川",
    content: "身负秘密的旅人。",
  });
  const chapter = await post(`/projects/${project.id}/chapters`, {
    title: "第一章 断桥",
    volume: "第一卷",
  });
  const revision = await post(`/chapters/${chapter.id}/revisions`, {
    expectedCurrentRevisionId: chapter.currentRevisionId || "",
    document: {
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            {
              type: "text",
              text: "林川在断桥边找到一枚铜印。他的左臂仍在疼痛。",
            },
          ],
        },
      ],
    },
    source: "author",
  });
  await page.request.put(`/api/chapters/${chapter.id}`, {
    headers,
    data: {
      title: chapter.title,
      volume: "第一卷",
      blueprint: "追查铜印来历，保持人物知识边界，结束在开箱前。",
    },
  });
  await page.goto(`/#write/${project.id}/${chapter.id}`);
  await expect(page.getByTestId("chapter-editor")).toBeVisible();
  await openWorkspaceAction(page, "创作工作室");
  await expect(page.getByRole("dialog", { name: "创作工作室" })).toBeVisible();
  return { headers, post, project, chapter, character, revision };
}
test("plot canvas persists dragged nodes and causal connections, plans and scenes", async ({
  page,
}) => {
  const { project } = await setup(page);
  await page.getByRole("button", { name: "添加节点", exact: true }).click();
  await page.getByLabel("节点标题").fill("发现铜印");
  await page.getByRole("button", { name: "添加节点", exact: true }).click();
  await page.getByLabel("节点标题").fill("追问来历");
  await page.getByLabel("前置情节").selectOption({ label: "发现铜印" });
  await page.getByRole("button", { name: "保存画布", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("画布已保存");
  const nodes = page.locator(".react-flow__node");
  await expect(nodes).toHaveCount(2);
  const first = nodes.filter({ hasText: "发现铜印" });
  const box = await first.boundingBox();
  if (box) {
    await page.mouse.move(box.x + 30, box.y + 20);
    await page.mouse.down();
    await page.mouse.move(box.x + 140, box.y + 80, { steps: 10 });
    await page.mouse.up();
  }
  const source = first.locator(".react-flow__handle-bottom");
  const target = nodes
    .filter({ hasText: "追问来历" })
    .locator(".react-flow__handle-top");
  if ((await source.count()) && (await target.count()))
    await source.dragTo(target);
  await page.getByRole("button", { name: "保存画布", exact: true }).click();
  await page.getByRole("button", { name: "故事规划", exact: false }).click();
  await page.getByLabel("题材", { exact: true }).fill("东方悬疑");
  await page.getByLabel("目标读者", { exact: true }).fill("喜爱线索推理的读者");
  await page.getByRole("button", { name: "下一层", exact: false }).click();
  await page.getByLabel("无法回避的核心冲突").fill("真相与保护同伴之间的抉择");
  await page.getByLabel("失败必须付出什么代价").fill("失去同伴信任");
  await page.getByRole("button", { name: "下一层", exact: false }).click();
  await page.getByRole("button", { name: "保存完整规划" }).click();
  await expect(page.getByRole("status")).toContainText("故事规划已保存");
  await page.getByRole("button", { name: "场景导演", exact: false }).click();
  await page.getByRole("button", { name: "新场景", exact: true }).click();
  await page.getByLabel("场景名").fill("雨中发现");
  await page.getByLabel("目标", { exact: true }).fill("查看铜印");
  await page.getByLabel("冲突", { exact: true }).fill("伤口影响行动");
  await page.getByRole("button", { name: "保存场景序列" }).click();
  await expect(page.getByRole("status")).toContainText("场景与交接状态已保存");
  const studio = await (
    await page.request.get(`/api/projects/${project.id}/studio`)
  ).json();
  expect(studio.board.data.nodes).toHaveLength(2);
  expect(studio.board.data.nodes[1].prerequisiteIds).toHaveLength(1);
  expect(studio.brief.data.coreConflict).toContain("同伴");
  expect(studio.scenes.data.scenes[0].title).toBe("雨中发现");
  await page.getByRole("button", { name: "剧情画布", exact: false }).click();
  await page.screenshot({
    path: "test-results/studio-canvas.png",
    fullPage: true,
  });
});
test("isolated reference, character arc, comments, sandbox checkpoints and branches work", async ({
  page,
}) => {
  const { project, character, revision } = await setup(page);
  await page.getByRole("button", { name: "人物弧光", exact: false }).click();
  await page.getByLabel("人物弧：从哪里走向哪里").fill("从孤立戒备到信任同伴");
  await page.getByLabel("此刻最想得到什么").fill("弄清铜印来历");
  await page.getByRole("button", { name: "保存人物弧" }).click();
  await expect(page.getByRole("status")).toContainText("人物弧");
  await page.getByRole("button", { name: "参考资料", exact: false }).click();
  await page.getByLabel("参考资料名").fill("自有短篇样例");
  await page
    .getByLabel("参考资料正文")
    .fill(
      "# 雨中\n\n林川站在桥下，听见远处传来铃声。\n\n“谁在那里？”他问。\n\n# 次日\n\n铜印留下的刻痕指向王都。",
    );
  await page.getByRole("button", { name: "分析并保存资料" }).click();
  await expect(page.getByRole("status")).toContainText("隔离导入");
  await expect(page.getByRole("heading", { name: "文风画像" })).toBeVisible();
  await page.getByRole("button", { name: "本章批注", exact: true }).click();
  await page.getByLabel("引用正文", { exact: true }).fill("他的左臂仍在疼痛。");
  await page.getByLabel("批注", { exact: true }).fill("后续搬箱应保持伤势。");
  await page.getByRole("button", { name: "添加批注" }).click();
  await expect(page.getByRole("status")).toContainText("批注已保存");
  await page.getByRole("button", { name: "标记解决" }).click();
  await expect(page.getByRole("status")).toContainText("批注状态已更新");
  await page.getByRole("button", { name: "关闭批注" }).click();
  await page.getByRole("button", { name: "世界沙盘", exact: false }).click();
  await page.getByLabel("实验名称").fill("磨坊对峙预演");
  await page.getByLabel("参与实体").selectOption(character.id);
  await page.getByRole("button", { name: "建立沙盘", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("世界实验已建立");
  await page.getByRole("button", { name: "采用沙盘候选" }).click();
  await expect(page.getByRole("status")).toContainText("已采用到沙盘");
  await page.getByRole("button", { name: "冻结", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("沙盘已冻结");
  await page.getByRole("button", { name: "回滚", exact: true }).first().click();
  await expect(page.getByRole("status")).toContainText("沙盘已回滚");
  await page.getByRole("button", { name: "故事分支", exact: false }).click();
  await page.getByLabel("分支名").fill("如果没有开箱");
  await page.getByRole("button", { name: "从当前版本分叉" }).click();
  await expect(page.getByRole("status")).toContainText("分支已创建");
  await expect(
    page.getByRole("button", { name: "切换到这个分支" }),
  ).toBeVisible();
  const studio = await (
    await page.request.get(`/api/projects/${project.id}/studio`)
  ).json();
  expect(studio.arcs.data.characters[0].arc).toContain("信任");
  expect(studio.references).toHaveLength(1);
  expect(studio.comments[0].resolved).toBe(true);
  expect(studio.sandboxes[0].frozen).toBe(true);
  expect(studio.sandboxes[0].history.length).toBeGreaterThan(1);
  await page.screenshot({
    path: "test-results/studio-branches.png",
    fullPage: true,
  });
});

test("workflow approvals, provider configuration, candidates and publication downloads are usable", async ({
  page,
}) => {
  const { project } = await setup(page);
  const suffix = Date.now();
  await page.getByRole("button", { name: "流程与团队", exact: false }).click();
  await page
    .getByLabel("流程名", { exact: true })
    .fill("作者审批审稿流程" + suffix);
  await page.getByRole("button", { name: "添加步骤", exact: true }).click();
  await page.getByRole("button", { name: "保存流程", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("流程定义");
  await page.getByRole("button", { name: "Prompt 版本", exact: true }).click();
  await page.getByLabel("模板名").fill("有证据的读者建议" + suffix);
  await page
    .getByLabel("系统提示")
    .fill("只针对当前正文提出可核对建议，引用原文，不修改正文。");
  await page.getByRole("button", { name: "保存新版本", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Prompt");
  await page.getByRole("button", { name: "Agent 职责", exact: true }).click();
  await page.getByLabel("角色名").fill("证据审稿编辑" + suffix);
  await page.getByRole("button", { name: "保存角色", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Agent");
  await page.getByRole("button", { name: "批量制作", exact: false }).click();
  await page
    .getByLabel("制作流程")
    .selectOption({ label: "作者审批审稿流程" + suffix });
  await page.getByLabel("结束章", { exact: true }).fill("1");
  await page.getByRole("button", { name: "创建批量制作" }).click();
  await expect(
    page.getByRole("button", { name: "批准此步" }).first(),
  ).toBeVisible();
  await page.getByRole("button", { name: "批准此步" }).first().click();
  await expect(page.getByRole("status")).toContainText("作者批准");
  await page.getByRole("button", { name: "模型与检索", exact: false }).click();
  await page
    .getByLabel("名称", { exact: true })
    .fill("禁用的兼容服务配置" + suffix);
  await page
    .getByLabel("API Base URL", { exact: true })
    .fill("http://127.0.0.1:1/v1");
  await page.getByLabel("模型 ID").fill("test-local-model");
  await page.getByLabel("启用服务", { exact: false }).uncheck();
  await page.getByLabel("计价币种").fill("CNY");
  await page.getByLabel("输入价格 / 百万 Token").fill("1.2");
  await page.getByLabel("输出价格 / 百万 Token").fill("3.4");
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Provider");
  const prices = await (await page.request.get("/api/provider-configs")).json();
  const saved = prices.find(
    (p: any) => p.name === "禁用的兼容服务配置" + suffix,
  );
  expect(saved.currency).toBe("CNY");
  expect(saved.inputPricePerMillion).toBe(1.2);
  expect(saved.outputPricePerMillion).toBe(3.4);
  await page.getByRole("button", { name: "出版与声音", exact: false }).click();
  await page.getByLabel("优先使用已定稿正文").uncheck();
  for (const format of [
    "Word 文稿",
    "EPUB 电子书",
    "PDF 阅读版",
    "HTML 网页",
  ]) {
    const download = page.waitForEvent("download");
    await page.getByRole("button", { name: format, exact: false }).click();
    const downloaded = await download;
    expect(downloaded.suggestedFilename()).toMatch(/.(docx|epub|pdf|html)$/);
    const stream = await downloaded.createReadStream();
    const chunks: Buffer[] = [];
    for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
    const bytes = Buffer.concat(chunks);
    expect(bytes.length).toBeGreaterThan(100);
    if (format.startsWith("PDF"))
      expect(bytes.subarray(0, 4).toString()).toBe("%PDF");
    if (format.startsWith("Word") || format.startsWith("EPUB"))
      expect(bytes.subarray(0, 2).toString()).toBe("PK");
    if (format.startsWith("HTML")) expect(bytes.toString()).toContain("<html");
  }
  await page.getByRole("button", { name: "高质量多候选", exact: true }).click();
  await page.getByRole("button", { name: "生成并评价候选" }).click();
  await expect(page.getByRole("status")).toContainText("多候选任务");
  await expect
    .poll(
      async () => {
        const groups = await (
          await page.request.get(`/api/projects/${project.id}/candidates`)
        ).json();
        return groups[0]?.status;
      },
      { timeout: 30000 },
    )
    .toBe("Succeeded");
  await page
    .getByRole("button", { name: /候选 [a-z0-9]/ })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "采用这份候选为新版本" }),
  ).toBeEnabled();
  await page.screenshot({
    path: "test-results/studio-publishing.png",
    fullPage: true,
  });
});
