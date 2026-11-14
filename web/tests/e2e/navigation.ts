import { expect, type Page } from "@playwright/test";

export async function openWorkspaceAction(page: Page, label: string) {
  const action = page
    .getByRole("button", { name: label, exact: true })
    .filter({ visible: true });
  if (await action.first().isVisible()) {
    await action.first().click();
    return;
  }
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page
    .getByRole("dialog", { name: "作品与设置", exact: true })
    .getByRole("button", { name: label, exact: true })
    .click();
}

export async function openChapterMaterial(page: Page, label: string) {
  const material = page.getByRole("button", { name: label, exact: true });
  if (!(await material.isVisible()))
    await page.getByRole("button", { name: "本章资料", exact: true }).click();
  await material.click();
}

export async function openBookSection(page: Page, label: string) {
  const section = page.getByRole("button", { name: label, exact: true });
  if (!(await section.isVisible()))
    await page.getByRole("button", { name: "设定与规划", exact: true }).click();
  await section.click();
}

export async function openBookSkills(page: Page) {
  await openBookSection(page, "文风与写法");
  const manage = page.getByRole("button", {
    name: /^(选择与管理 Skills|收起 Skills 管理)$/,
  });
  await expect(manage).toBeVisible();
  if ((await manage.getAttribute("aria-expanded")) !== "true")
    await manage.click();
  await expect(manage).toHaveAttribute("aria-expanded", "true");
  await expect(
    page.getByRole("heading", { name: "写作 Skills", exact: true }),
  ).toBeVisible();
}
