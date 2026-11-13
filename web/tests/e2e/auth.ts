import { expect, type Page, type Cookie } from "@playwright/test";
let authorCookies: Cookie[] = [];
export async function authenticate(page: Page) {
  if (authorCookies.length) {
    await page.context().addCookies(authorCookies);
    const existing = await (await page.request.get("/api/session")).json();
    if (existing.authenticated) return existing;
  }
  const response = await page.request.post("/api/session", {
    headers: { Origin: process.env.E2E_BASE_URL || "http://127.0.0.1:8089" },
    data: {
      username: process.env.E2E_USERNAME || "e2e-author",
      password: process.env.E2E_PASSWORD || "test-only-novel-2026",
    },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  authorCookies = await page.context().cookies();
  return response.json();
}
