import { expect, test } from "@playwright/test";

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

test("create a persistent protected project and open its builder", async ({ page, baseURL }) => {
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
  await expect(page.getByText("Password protection enabled")).toBeVisible();
  await expect(
    page.getByText("No container is running for this project yet.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Build request" })).toBeDisabled();
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
