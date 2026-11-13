import { test, expect } from "@playwright/test";
test("installable offline shell never stores manuscript or session API responses", async ({
  page,
  context,
}) => {
  await page.goto("/");
  await expect
    .poll(
      () =>
        page.evaluate(() =>
          navigator.serviceWorker.getRegistration().then((r) => !!r),
        ),
      { timeout: 15000 },
    )
    .toBe(true);
  await page.evaluate(() => navigator.serviceWorker.ready);
  const manifest = await (
    await page.request.get("/manifest.webmanifest")
  ).json();
  expect(manifest.display).toBe("standalone");
  expect(manifest.name).toContain("砚台");
  await page.request.get("/api/session");
  const cached = await page.evaluate(async () => {
    const urls: string[] = [];
    for (const name of await caches.keys()) {
      const cache = await caches.open(name);
      for (const request of await cache.keys()) urls.push(request.url);
    }
    return urls;
  });
  expect(cached.some((url) => new URL(url).pathname.startsWith("/api/"))).toBe(
    false,
  );
  expect(cached.some((url) => new URL(url).pathname === "/")).toBe(true);
  await context.setOffline(true);
  await page.reload();
  await expect(page.getByRole("button", { name: "进入书房" })).toBeVisible();
  expect(await page.locator("body").innerText()).not.toContain("林川");
  await context.setOffline(false);
});
