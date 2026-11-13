import { authenticate } from "./auth";
import { test, expect } from "@playwright/test";
test("nonempty memory is edited, confirmed, cited and invalidated after an old chapter changes", async ({
  page,
}) => {
  const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
  const session = await authenticate(page);
  const headers = { Origin: origin, "X-CSRF-Token": session.csrfToken };
  const post = async (path: string, data: unknown) => {
    const r = await page.request.post("/api" + path, { headers, data });
    expect(r.ok(), await r.text()).toBeTruthy();
    return r.json();
  };
  const project = await post("/projects", {
    title: "记忆非空闭环 · " + Date.now(),
    description: "证据、作者决定与旧稿失效",
    genre: "测试",
  });
  const lin = await post(`/projects/${project.id}/settings`, {
    kind: "character",
    name: "林川",
    content: "一名在桥边受伤的旅人。",
  });
  const chapter = await post(`/projects/${project.id}/chapters`, {
    title: "第一章 断桥",
    volume: "第一卷",
  });
  const text = "林川的左臂受伤，仍然疼痛。林川留在北岸磨坊，决定暂不离开。";
  const revision = await post(`/chapters/${chapter.id}/revisions`, {
    expectedCurrentRevisionId: chapter.currentRevisionId || "",
    document: {
      type: "doc",
      content: [{ type: "paragraph", content: [{ type: "text", text }] }],
    },
    source: "author",
  });
  await post(`/chapters/${chapter.id}/finalize`, { revisionId: revision.id });
  const detail = await (
    await page.request.get(`/api/projects/${project.id}`)
  ).json();
  const evidence = (quote: string) => ({
    revisionId: revision.id,
    plainTextHash: revision.hash,
    quote,
    prefix: "",
    suffix: "",
  });
  const proposal = await post(`/chapters/${chapter.id}/memory-proposals`, {
    sourceRevisionId: revision.id,
    baseStateId: detail.project.stateVersionId || "",
    epoch: detail.project.epoch,
    changes: [
      {
        id: "",
        kind: "Injury",
        subject: lin.id,
        predicate: "左臂",
        object: "受伤",
        characterId: "",
        truthStatus: "True",
        storyTime: null,
        evidence: evidence("林川的左臂受伤，仍然疼痛。"),
        decision: "Pending",
      },
      {
        id: "",
        kind: "Location",
        subject: lin.id,
        predicate: "位置",
        object: "北岸磨坊",
        characterId: "",
        truthStatus: "True",
        storyTime: null,
        evidence: evidence("林川留在北岸磨坊，决定暂不离开。"),
        decision: "Pending",
      },
    ],
  });
  await page.goto(`/#write/${project.id}/${chapter.id}`);
  await expect(page.getByTestId("chapter-editor")).toContainText(
    "林川的左臂受伤",
  );
  await page.getByRole("button", { name: "故事记忆", exact: true }).click();
  await page.getByLabel("选择记忆提案").selectOption(proposal.id);
  const changes = page.locator(".memory-change");
  await expect(changes).toHaveCount(2);
  await changes
    .first()
    .getByRole("button", { name: "编辑", exact: true })
    .click();
  await page.getByLabel("对象或状态内容").fill("受伤，仍然疼痛");
  await page.getByRole("button", { name: "保存这项编辑" }).click();
  await expect(page.getByRole("status")).toContainText("变更编辑已保存");
  await changes
    .nth(1)
    .getByRole("button", { name: "拒绝", exact: true })
    .click();
  await page.getByRole("button", { name: "保存审核决定" }).click();
  await expect(page.getByRole("status")).toContainText("审核决定已保存");
  await page.getByRole("button", { name: "提交已审核记忆" }).click();
  await expect(page.getByRole("status")).toContainText("记忆变更已提交");
  await page.getByText("当前已确认的记忆", { exact: false }).click();
  await expect(page.locator(".fact-card")).toHaveCount(1);
  await expect(page.locator(".fact-card")).toContainText("受伤，仍然疼痛");
  await page
    .locator(".fact-card")
    .getByRole("button", { name: "查看来源正文" })
    .click();
  await expect(page.locator(".fact-card mark")).toContainText("林川的左臂受伤");
  await page.getByText("故事时间线与人物关系", { exact: false }).click();
  await expect(page.locator(".timeline-list")).toContainText("受伤，仍然疼痛");
  const second = await post(`/projects/${project.id}/chapters`, {
    title: "第二章 启程",
    volume: "第一卷",
  });
  await page.reload();
  await page.getByRole("button", { name: "02 第二章 启程" }).click();
  await page.getByRole("button", { name: "历史检索", exact: true }).click();
  await page.getByLabel("搜索故事历史").fill("左臂");
  await page.getByRole("button", { name: "执行历史检索" }).click();
  await expect(page.locator(".citation-card").first()).toContainText("左臂");
  await page
    .locator(".citation-card")
    .first()
    .getByRole("button", { name: "查看原文" })
    .click();
  await expect(page.locator(".source-preview")).toContainText("林川的左臂受伤");
  await page.getByRole("button", { name: "01 第一章 断桥" }).click();
  await page
    .getByTestId("chapter-editor")
    .fill("林川的右臂受伤，仍然疼痛。林川留在北岸磨坊，决定暂不离开。");
  await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  page.once("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "确认定稿" }).click();
  await expect(
    page.getByText("记忆待重建", { exact: true }).first(),
  ).toBeVisible();
  await page.getByRole("button", { name: "故事记忆", exact: true }).click();
  await expect(page.getByText(/旧稿发生了变化/)).toBeVisible();
  await page.getByRole("button", { name: "改稿影响记录" }).click();
  await expect(page.getByRole("dialog")).toContainText("相关状态需要重建");
  await page.getByRole("button", { name: "关闭弹窗" }).click();
  await page.screenshot({
    path: "test-results/memory-invalidation.png",
    fullPage: true,
  });
});
