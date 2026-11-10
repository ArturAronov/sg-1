import { expect, test, type Page } from "@playwright/test";

test("sibling apps cannot overwrite host-prefixed authentication cookies", async ({ browser }) => {
  const context = await browser.newContext();
  try {
    await context.route("https://**/*", (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<!doctype html><html><body>Cookie isolation fixture</body></html>",
      }),
    );
    const names = [
      "__Host-SG1AuthSession",
      ...[0, 1, 2, 3].map((i) => `__Host-SG1AuthSession-${i}`),
    ];
    await context.addCookies(
      names.map((name) => ({
        name,
        value: "trusted-session",
        url: "https://sg1.chickensoup.dev",
        secure: true,
        httpOnly: true,
      })),
    );
    await context.addCookies([
      {
        name: "__Host-sg1_app",
        value: "trusted-app-ticket",
        url: "https://one.apps.chickensoup.dev",
        secure: true,
        httpOnly: true,
      },
    ]);
    const app = await context.newPage();
    await app.goto("https://evil.apps.chickensoup.dev");
    await app.evaluate((names) => {
      for (const name of names) {
        document.cookie = `${name}=forged; Domain=chickensoup.dev; Path=/; Secure`;
        document.cookie = `${name}=forged; Domain=sg1.chickensoup.dev; Path=/; Secure`;
      }
      document.cookie = "__Host-sg1_app=forged; Domain=apps.chickensoup.dev; Path=/; Secure";
    }, names);
    expect(await app.evaluate(() => document.cookie)).not.toContain("trusted-session");
    const platformCookies = await context.cookies("https://sg1.chickensoup.dev");
    expect(platformCookies).toHaveLength(5);
    expect(
      platformCookies.every(
        (cookie) => cookie.value === "trusted-session" && cookie.domain === "sg1.chickensoup.dev",
      ),
    ).toBe(true);
    const otherAppCookies = await context.cookies("https://one.apps.chickensoup.dev");
    expect(otherAppCookies).toHaveLength(1);
    expect(otherAppCookies[0].value).toBe("trusted-app-ticket");
  } finally {
    await context.close();
  }
});

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

test("member-only viewers require sign-in and deny non-members", async ({ page }) => {
  await page.goto("/projects/members-app");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByTitle("Published application", { exact: true })).toHaveCount(0);
  await signIn(page);
  await page.goto("/projects/members-app");
  await expect(page).toHaveURL(/\/app-access\/members-app$/);
  await expect(page.getByTitle("Published application", { exact: true })).toBeVisible();
  await page.goto("/projects/denied-app");
  await expect(page.getByRole("heading", { name: "Project membership required." })).toBeVisible();
  await expect(page.getByTitle("Published application", { exact: true })).toHaveCount(0);
});

test("project admins can select member-only app visibility", async ({ page }) => {
  await signIn(page);
  await page.goto("/build/property-hub");
  await page.getByText("Sharing & passwords", { exact: true }).click();
  await page.getByLabel("Application access", { exact: true }).selectOption("members");
  await expect(page.getByLabel("New application password")).toHaveCount(0);
  await expect(
    page.getByText("Only active employees explicitly assigned", { exact: false }),
  ).toBeVisible();
  await page.route("**/api/projects/property-hub/access", async (route) => {
    expect(route.request().method()).toBe("PUT");
    expect(route.request().postDataJSON().mode).toBe("members");
    await route.fulfill({ json: { status: "saved" } });
  });
  await page.getByRole("button", { name: "Save access settings", exact: true }).click();
  await expect(page.getByText("Access settings saved.", { exact: false })).toBeVisible();
});

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
  await expect(page.getByRole("link", { name: "Open app", exact: true })).toHaveAttribute(
    "href",
    "/projects/property-hub",
  );
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

test("published project paths work anonymously and keep generated code off-origin", async ({
  page,
}) => {
  await page.goto("/projects/property-hub");
  await expect(page).toHaveURL(/\/projects\/property-hub$/);
  const viewer = page.getByTitle("Published application", { exact: true });
  await expect(viewer).toHaveAttribute("src", "http://127.0.0.1:3108/published-fixture");
  await expect(viewer).toHaveAttribute(
    "sandbox",
    "allow-scripts allow-forms allow-same-origin allow-downloads",
  );
  await expect(
    page
      .frameLocator('iframe[title="Published application"]')
      .getByRole("heading", { name: "Published fixture app" }),
  ).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Main navigation" })).toHaveCount(0);

  await page.goto("/property-hub");
  await expect(page).toHaveURL(/\/projects\/property-hub$/);
  await expect(page.getByTitle("Published application", { exact: true })).toBeVisible();
});

test("project viewers fail closed for unpublished and invalid slugs", async ({ page }) => {
  for (const path of ["/projects/private-project", "/projects/INVALID", "/projects/bad--slug"]) {
    const response = await page.goto(path);
    expect(response?.status()).toBe(404);
    await expect(page.getByRole("heading", { name: "Nothing published here yet." })).toBeVisible();
    await expect(page.getByTitle("Published application", { exact: true })).toHaveCount(0);
    await expect(page.getByText("Local developer", { exact: true })).toHaveCount(0);
  }
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
