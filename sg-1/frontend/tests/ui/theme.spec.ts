import { expect, test, type Page } from "@playwright/test";

async function signIn(page: Page) {
  await page.context().addCookies([
    {
      name: "sg1_development_session",
      value: "theme-test-only-not-a-real-secret-00000000",
      url: "http://127.0.0.1:3107",
    },
  ]);
  await page.goto("/");
}

async function noOverflow(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

test("sign-in keeps its content, validation, and accessible focus", async ({ page }) => {
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Welcome to the studio." })).toBeVisible();
  await page.getByLabel("Development token").focus();
  await expect(page.getByLabel("Development token")).toHaveCSS("outline-style", "solid");
  await noOverflow(page);
  await page.goto("/login?error=1");
  await expect(page.getByLabel("Development token")).toHaveAttribute("aria-invalid", "true");
  await expect(page.locator("#login-error")).toBeVisible();
});

test("project directory preserves search, filtering, and keyboard dialog controls", async ({
  page,
}, testInfo) => {
  await signIn(page);
  await expect(
    page.getByRole("heading", { name: "Tools built around the way you work." }),
  ).toBeVisible();
  await expect(page.locator(".project-card")).toHaveCount(3);
  await page.evaluate(() => window.scrollTo(0, 0));
  await noOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("desktop-theme.png"),
    fullPage: true,
    style: "nextjs-portal { display: none; }",
  });
  await page.getByRole("textbox", { name: "Search projects" }).fill("Property");
  await expect(page.locator(".project-card")).toHaveCount(1);
  await page.getByRole("textbox", { name: "Search projects" }).fill("no such project");
  await expect(page.getByRole("heading", { name: "No projects match your search." })).toBeVisible();
  await page.getByRole("textbox", { name: "Search projects" }).fill("");
  await page.getByLabel("Filter department").selectOption("operations");
  await expect(page.locator(".project-card")).toHaveCount(2);
  await page.getByRole("button", { name: "New project", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByLabel("Project name", { exact: true })).toBeFocused();
  await page.getByLabel("Project name", { exact: true }).fill("New useful tool");
  await expect(page.getByLabel("Project URL", { exact: true })).toHaveValue("new-useful-tool");
  await page.getByLabel("Close dialog").focus();
  await page.keyboard.press("Shift+Tab");
  await expect(page.getByRole("button", { name: "Create workspace", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "New project", exact: true })).toBeFocused();
});

test("navigation, provider forms, and builder controls work at every breakpoint", async ({
  page,
}, testInfo) => {
  await signIn(page);
  const nav = page.getByRole("navigation", { name: "Main navigation" });
  await nav.getByRole("link", { name: "AI providers", exact: true }).click();
  await expect(nav.getByRole("link", { name: "AI providers", exact: true })).toHaveAttribute(
    "aria-current",
    "page",
  );
  await expect(page.locator(".provider-card")).toHaveCount(4);
  const save = page.getByRole("button", { name: "Save Anthropic key" });
  await expect(save).toBeDisabled();
  await expect(save).toHaveCSS("background-color", "rgb(238, 241, 245)");
  await page.getByLabel("Anthropic API key").fill("test-only-key-not-submitted");
  await expect(save).toBeEnabled();
  await noOverflow(page);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("providers-theme.png"),
    fullPage: true,
    style: "nextjs-portal { display: none; }",
  });
  await nav.getByRole("link", { name: "Projects", exact: true }).click();
  await page.locator(".project-card").filter({ hasText: "Property hub" }).click();
  await expect(page.getByRole("heading", { name: "Property hub" })).toBeVisible();
  await expect(page.getByLabel("Build request")).toBeEnabled();
  await expect(page.getByTitle("Application preview", { exact: true })).toBeVisible();
  await page.getByText("Sharing & passwords", { exact: true }).click();
  await expect(page.getByLabel("New application password")).toBeVisible();
  await page.getByText("Private project files (0)", { exact: true }).click();
  await expect(page.getByLabel("Project file", { exact: true })).toBeVisible();
  await noOverflow(page);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("builder-theme.png"),
    fullPage: true,
    style: "nextjs-portal { display: none; }",
  });
});

test("small-phone and tablet layouts never lose navigation or overflow", async ({ page }) => {
  await signIn(page);
  for (const width of [320, 580, 768, 950, 1024]) {
    await page.setViewportSize({ width, height: 900 });
    await noOverflow(page);
    await expect(page.getByRole("navigation", { name: "Main navigation" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  }
});

test("theme contrast, pressed states, and reduced motion", async ({ page }) => {
  await signIn(page);
  const contrastPairs = await page.evaluate(() => {
    const styles = getComputedStyle(document.documentElement);
    const color = (name: string) => styles.getPropertyValue(name).trim();
    return ["--paper", "--desktop", "--accent-soft"]
      .map((name) => [color("--ink"), color(name)])
      .concat([
        [color("--muted"), color("--paper")],
        [color("--muted"), color("--desktop")],
        [color("--muted"), color("--accent-soft")],
        [color("--paper"), color("--accent")],
        [color("--paper"), color("--accent-hover")],
        [color("--accent"), color("--accent-soft")],
        [color("--success"), color("--success-soft")],
        [color("--warning"), color("--warning-soft")],
        [color("--error"), color("--error-soft")],
      ]);
  });
  const luminance = (hex: string) => {
    const digits = hex.slice(1);
    const normalized =
      digits.length === 3
        ? digits
            .split("")
            .map((c) => c + c)
            .join("")
        : digits;
    const channels = normalized.match(/../g)!.map((value) => {
      const c = parseInt(value, 16) / 255;
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
    });
    return channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
  };
  for (const [foreground, background] of contrastPairs) {
    const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
    expect((values[0] + 0.05) / (values[1] + 0.05)).toBeGreaterThanOrEqual(4.5);
  }
  await page.emulateMedia({ reducedMotion: "reduce" });
  const button = page.getByRole("button", { name: "New project", exact: true });
  await expect(button).toHaveCSS("transition-duration", "0s");
  await button.hover();
  await expect(button).toHaveCSS("background-color", "rgb(41, 73, 143)");
  await page.mouse.down();
  await expect(button).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 1)");
  await page.mouse.up();
});
