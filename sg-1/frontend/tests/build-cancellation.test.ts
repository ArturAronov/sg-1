import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import test from "node:test";
import { LiveBuilder, type RuntimeState } from "../src/components/live-builder";
import type { LLMSettings } from "../src/lib/types";

const settings: LLMSettings = { providers: [], credentials: [] };

function render(status: string, role: "read" | "write" | "admin" = "write") {
  const state: RuntimeState = {
    project: {
      id: "project-id",
      name: "Test project",
      slug: "test-project",
      description: "",
      department_id: "department-id",
      department_name: "Property management",
      status: "ready",
      access_mode: "password",
      role,
      created_at: "2026-11-09T10:00:00Z",
    },
    messages: [],
    builds: [
      { id: "build-id", status, message: "Build status", provider: "openai", model: "test" },
    ],
    deployments: [],
    current_deployment_id: "deployment-id",
    last_error: null,
    password_set: false,
  };
  return renderToStaticMarkup(createElement(LiveBuilder, { initial: state, settings }));
}

test("editors can cancel queued and running builds", () => {
  for (const status of ["queued", "generating", "building", "validating"]) {
    assert.match(render(status), />Cancel build<\/button>/);
  }
});

test("cancelling builds remain active with a disabled stop control", () => {
  assert.match(render("cancelling"), /<button[^>]*disabled=""[^>]*>Stopping…<\/button>/);
  assert.match(render("cancelling"), /aria-label="Model ID"[^>]*disabled=""/);
});

test("terminal builds do not block model selection or show cancellation", () => {
  for (const status of ["cancelled", "succeeded", "failed"]) {
    const html = render(status);
    assert.doesNotMatch(html, />Cancel build<|>Stopping…</);
    assert.doesNotMatch(html, /aria-label="Model ID"[^>]*disabled=""/);
  }
});

test("read-only members never see a cancel control", () => {
  assert.doesNotMatch(render("building", "read"), />Cancel build</);
});
