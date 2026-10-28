import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

const token = process.env.SG1_DEV_TOKEN;
if (!token || token.length < 32)
  throw new Error("Set SG1_DEV_TOKEN or run the root setup helper before browser tests");

test("anonymous visitors cannot enter the builder", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "Welcome to the studio." })).toBeVisible();
  await page.goto("/build/secret-project");
  await expect(page).toHaveURL(/\/login$/);
});

test("rejects incorrect tokens and cross-origin mutations", async ({ page, request, baseURL }) => {
  await page.goto("/login");
  await page.getByLabel("Development token").fill("an-incorrect-token-with-at-least-32-characters");
  await page.getByRole("button", { name: "Open workspace" }).click();
  await expect(page).toHaveURL(/error=1/);
  const response = await request.post(`${baseURL}/api/projects`, {
    headers: { Origin: "https://untrusted.example" },
    data: {},
  });
  expect(response.status()).toBe(403);
});

test("create a real protected container and open its builder", async ({ page, baseURL }) => {
  test.setTimeout(180000);
  await page.goto("/login");
  await page.getByLabel("Development token").fill(token!);
  await page.getByRole("button", { name: "Open workspace" }).click();
  await expect(
    page.getByRole("heading", { name: "Tools built around the way you work." }),
  ).toBeVisible();
  expect(await page.content()).not.toContain(token!);
  const name = `Browser test ${Date.now()}`;
  const slug = name.toLowerCase().replaceAll(" ", "-");
  await page.getByRole("button", { name: "New project", exact: true }).click();
  await page.getByLabel("Project name", { exact: true }).fill(name);
  await expect(page.getByLabel("Project URL", { exact: true })).toHaveValue(slug);
  await page.getByRole("button", { name: "Create workspace", exact: true }).click();
  await expect(page).toHaveURL(`${baseURL}/build/${slug}`);
  await expect(page.getByRole("heading", { name })).toBeVisible();
  await expect(page.getByText("Container running", { exact: true })).toBeVisible({
    timeout: 150000,
  });
  await expect(page.getByRole("textbox", { name: "Build request" })).toBeEnabled();
  await expect(
    page
      .frameLocator('iframe[title="Application preview"]')
      .getByRole("heading", { name: "Your project is running." }),
  ).toBeVisible({ timeout: 15000 });
  const source = await page.request.get(`/api/projects/${slug}/source`);
  expect(source.status()).toBe(200);
  expect((await source.body()).subarray(0, 2).toString()).toBe("PK");
  await page.getByText("Private project files (0)", { exact: true }).click();
  await page.getByLabel("Project file", { exact: true }).setInputFiles({
    name: "example.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("persistent project attachment"),
  });
  await page.getByRole("button", { name: "Upload file", exact: true }).click();
  await expect(page.getByText("example.txt", { exact: true })).toBeVisible();
  const downloading = page.waitForEvent("download");
  await page.getByRole("link", { name: "Download", exact: true }).click();
  const download = await downloading;
  expect(await readFile((await download.path())!, "utf8")).toBe("persistent project attachment");
  await page.getByText("Sharing & passwords", { exact: true }).click();
  await page.getByLabel("New application password").fill("browser-test-password-123");
  await page.getByRole("button", { name: "Save access settings" }).click();
  await expect(page.getByText("Access settings saved.", { exact: false })).toBeVisible();
  const visitor = await page.context().browser()!.newContext();
  try {
    const app = await visitor.newPage();
    await app.goto(`${baseURL}/${slug}`);
    const frame = app.frameLocator('iframe[title="Published application"]');
    await expect(frame.getByRole("heading", { name: "Password required" })).toBeVisible();
    await frame.getByLabel("Application password").fill("browser-test-password-123");
    await frame.getByRole("button", { name: "Open application" }).click();
    await expect(frame.getByRole("heading", { name: "Your project is running." })).toBeVisible();
    await page.getByLabel("Application access", { exact: true }).selectOption("public");
    await page.getByRole("checkbox").check();
    const sharing = page.waitForResponse(
      (r) => r.url().endsWith(`/api/projects/${slug}/access`) && r.request().method() === "PUT",
    );
    await page.getByRole("button", { name: "Save access settings" }).click();
    expect((await sharing).ok()).toBe(true);
    const anonymous = await page.context().browser()!.newContext();
    try {
      const publicPage = await anonymous.newPage();
      await publicPage.goto(`${baseURL}/${slug}`);
      const publicFrame = publicPage.frameLocator('iframe[title="Published application"]');
      await expect(
        publicFrame.getByRole("heading", { name: "Your project is running." }),
      ).toBeVisible();
      await page.getByLabel("Application access", { exact: true }).selectOption("password");
      const protecting = page.waitForResponse(
        (r) => r.url().endsWith(`/api/projects/${slug}/access`) && r.request().method() === "PUT",
      );
      await page.getByRole("button", { name: "Save access settings" }).click();
      expect((await protecting).ok()).toBe(true);
      await publicPage.reload();
      await expect(publicFrame.getByRole("heading", { name: "Password required" })).toBeVisible();
    } finally {
      await anonymous.close();
    }
  } finally {
    await visitor.close();
  }
  await page.reload();
  await expect(page.getByRole("heading", { name })).toBeVisible();
  await page.getByRole("link", { name: "All projects" }).click();
  await page.getByRole("textbox", { name: "Search projects" }).fill(name);
  await expect(page.getByRole("heading", { name })).toBeVisible();
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("viewer URLs fail closed instead of publishing project data", async ({ page }) => {
  const response = await page.goto("/private-project");
  expect(response?.status()).toBe(404);
  await expect(page.getByRole("heading", { name: "Nothing published here yet." })).toBeVisible();
  await expect(page.getByText("Local developer", { exact: true })).toHaveCount(0);
});
