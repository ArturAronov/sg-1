import assert from "node:assert/strict";
import test from "node:test";
import { isSameOrigin, readLimitedBody } from "../src/lib/security";
import { suggestSlug } from "../src/lib/slug";

test("slug suggestions normalize names and stay DNS-safe", () => {
  assert.equal(suggestSlug("  Maintenance & Répairs! "), "maintenance-repairs");
  assert.equal(suggestSlug("a".repeat(47) + " something"), "a".repeat(47));
  assert.equal(suggestSlug("../api/../../"), "api"); // Server rejects reserved names.
});

test("mutation origin checks reject missing, malicious, and sibling origins", () => {
  assert.equal(isSameOrigin("http://localhost:3000", "http://localhost:3000"), true);
  for (const value of [
    null,
    "null",
    "https://evil.example",
    "http://localhost:3000.evil.example",
    "http://localhost:3000/extra",
    "http://localhost:3001",
    "https://app.ourcompany.net",
  ]) {
    assert.equal(isSameOrigin(value, "http://localhost:3000"), false);
  }
});

test("body limit checks bytes, not just characters or content-length", async () => {
  const request = new Request("http://localhost", { method: "POST", body: "hello" });
  assert.equal(await readLimitedBody(request, 5), "hello");
  await assert.rejects(
    readLimitedBody(new Request("http://localhost", { method: "POST", body: "ééé" }), 5),
    /too large/,
  );
  await assert.rejects(
    readLimitedBody(new Request("http://localhost", { method: "POST", body: "x".repeat(17_000) })),
    /too large/,
  );
});
