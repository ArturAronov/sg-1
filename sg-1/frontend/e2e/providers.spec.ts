import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";

const token = process.env.SG1_DEV_TOKEN;
if (!token) throw new Error("Set SG1_DEV_TOKEN before running browser tests");

for (const [id, name] of [
  ["openai", "OpenAI"],
  ["anthropic", "Claude"],
  ["glm", "GLM"],
  ["kimi", "Kimi"],
]) {
  test(`${name}: save, replace, persist, and remove a private key`, async ({ page, baseURL }) => {
    await page.goto("/login");
    await page.getByLabel("Development token").fill(token!);
    await page.getByRole("button", { name: "Open workspace" }).click();
    await page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: "AI providers" })
      .click();
    await expect(page).toHaveURL(`${baseURL}/settings/providers`);
    const card = page.getByRole("region", { name, exact: true });
    await expect(card).toBeVisible();
    // Never overwrite or delete credentials someone configured outside the test.
    test.skip(
      await card.getByRole("button", { name: `Replace ${name} key` }).isVisible(),
      "This account already has a key; leave it untouched",
    );
    const key = `synthetic-browser-key-${id}-${randomUUID()}`;
    let saved = false;
    try {
      await card.getByLabel(`${name} API key`, { exact: true }).fill(key);
      const saving = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/api/settings/llm-providers/${id}`) && r.request().method() === "PUT",
      );
      await card.getByRole("button", { name: `Save ${name} key`, exact: true }).click();
      const response = await saving;
      expect(response.ok()).toBe(true);
      saved = true;
      expect(await response.text()).not.toContain(key);
      await expect(card.getByText("Key stored", { exact: true })).toBeVisible();
      await expect(card.getByRole("status")).toContainText("Not verified");
      await expect(card.getByLabel(`${name} API key`, { exact: true })).toHaveCount(0);
      expect(await page.content()).not.toContain(key);
      await page.reload();
      await expect(card.getByRole("button", { name: `Replace ${name} key` })).toBeVisible();
      await card.getByRole("button", { name: `Replace ${name} key` }).click();
      await expect(card.getByLabel(`${name} API key`, { exact: true })).toHaveValue("");
      await card.getByLabel(`${name} API key`, { exact: true }).fill(`${key}-replacement`);
      await card.getByRole("button", { name: `Save ${name} key`, exact: true }).click();
      await expect(card.getByText("Key stored", { exact: true })).toBeVisible();
      await card.getByRole("button", { name: `Remove ${name} key` }).click();
      await card.getByRole("button", { name: "Confirm removal" }).click();
      await expect(card.getByText("Not configured", { exact: true })).toBeVisible();
      saved = false;
      await page.reload();
      await expect(card.getByLabel(`${name} API key`, { exact: true })).toHaveValue("");
    } finally {
      if (saved)
        await page.request.delete(`/api/settings/llm-providers/${id}`, {
          headers: { Origin: baseURL! },
        });
    }
  });
}

test("provider settings require a session and reject foreign-origin changes", async ({
  page,
  request,
  baseURL,
}) => {
  await page.goto("/settings/providers");
  await expect(page).toHaveURL(/\/login$/);
  for (const method of ["PUT", "DELETE"]) {
    const response = await request.fetch("/api/settings/llm-providers/openai", {
      method,
      headers: { Origin: "https://untrusted.example" },
      data: { api_key: "synthetic-not-a-real-key" },
    });
    expect(response.status()).toBe(403);
    const anonymous = await request.fetch("/api/settings/llm-providers/openai", {
      method,
      headers: { Origin: baseURL! },
      data: { api_key: "synthetic-not-a-real-key" },
    });
    expect(anonymous.status()).toBe(401);
  }
});
