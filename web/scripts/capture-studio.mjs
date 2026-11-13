import { chromium } from "@playwright/test";
import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
const origin = process.env.E2E_BASE_URL || "http://127.0.0.1:8089";
const browser = await chromium.launch({
  channel: process.env.PLAYWRIGHT_CHANNEL || "chrome",
  headless: true,
});
const context = await browser.newContext({
  viewport: { width: 1600, height: 1100 },
});
const page = await context.newPage();
try {
  const response = await page.request.post(origin + "/api/session", {
    headers: { Origin: origin },
    data: {
      username: process.env.E2E_USERNAME || "e2e-author",
      password: process.env.E2E_PASSWORD || "test-only-novel-2026",
    },
  });
  if (!response.ok()) throw new Error("Screenshot account could not sign in");
  const projects = await (
    await page.request.get(origin + "/api/projects")
  ).json();
  let studioProject;
  for (const project of projects.filter((p) =>
    p.title.startsWith("Studio 全流程"),
  )) {
    const state = await (
      await page.request.get(`${origin}/api/projects/${project.id}/studio`)
    ).json();
    if (state.board.data.nodes.length > 0) {
      studioProject = project;
      break;
    }
  }
  const connected = projects.find((p) => p.title.startsWith("联网与协作验收"));
  if (!studioProject || !connected)
    throw new Error(
      "Run the dedicated browser fixtures before capturing screenshots",
    );
  const directory = resolve("../docs/release/screenshots");
  await mkdir(directory, { recursive: true });
  async function openProject(projectID) {
    await page.goto(origin);
    await page.evaluate(
      (id) => localStorage.setItem("novel-project", id),
      projectID,
    );
    await page.reload();
    await page.getByTestId("chapter-editor").waitFor();
    await page.getByRole("button", { name: "创作工作室", exact: true }).click();
  }
  async function capture(name, tab) {
    await page.getByRole("button", { name: tab, exact: false }).click();
    await page.locator(".studio-main").evaluate((el) => (el.scrollTop = 0));
    await page.locator(".studio-nav").evaluate((el) => (el.scrollTop = 0));
    await page.waitForTimeout(350);
    await page.screenshot({ path: resolve(directory, name), fullPage: true });
  }
  await openProject(studioProject.id);
  await capture("studio-canvas.png", "剧情画布");
  await capture("studio-planning.png", "故事规划");
  await capture("studio-scenes.png", "场景导演");
  await capture("studio-publishing.png", "出版与声音");
  const memberships = await Promise.all(
    projects
      .filter((p) => p.title.startsWith("联网与协作验收"))
      .map(async (p) => ({
        p,
        m: await (
          await page.request.get(`${origin}/api/projects/${p.id}/members`)
        ).json(),
      })),
  );
  const collab =
    memberships.find((v) => v.m.some((m) => m.username.startsWith("viewer-")))
      ?.p || connected;
  await page.getByRole("button", { name: "返回写作", exact: true }).click();
  await openProject(collab.id);
  await capture("studio-collaboration.png", "协作与权限");
  await page.setViewportSize({width:390,height:844});
  await page.getByRole('button',{name:'故事规划',exact:false}).click();
  await page.locator('.studio-main').evaluate(el=>el.scrollTop=0);
  await page.screenshot({path:resolve(directory,'studio-mobile.png'),fullPage:true});
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>window.innerWidth);
  if(overflow)throw new Error('Mobile Studio has horizontal overflow');
  console.log("Captured five Studio screenshots in docs/release/screenshots");
} finally {
  await browser.close();
}
