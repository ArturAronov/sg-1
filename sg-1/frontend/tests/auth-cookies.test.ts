import assert from "node:assert/strict";
import test from "node:test";
import {
  ALB_SESSION_COOKIE,
  ALB_SESSION_COOKIES,
  hasAmbiguousALBSessionCookies,
} from "../src/lib/auth-cookies";

test("ALB cookies use host-only prefix for the base and all shards", () => {
  assert.equal(ALB_SESSION_COOKIE, "__Host-SG1AuthSession");
  assert.equal(ALB_SESSION_COOKIES.length, 5);
  assert.ok(ALB_SESSION_COOKIES.every((name) => name.startsWith("__Host-")));
});

test("ambiguous employee session cookies fail closed", () => {
  for (const header of [
    null,
    "theme=light",
    `${ALB_SESSION_COOKIE}=one`,
    `${ALB_SESSION_COOKIE}-0=one; ${ALB_SESSION_COOKIE}-1=two`,
  ]) {
    assert.equal(hasAmbiguousALBSessionCookies(header), false);
  }
  for (const header of [
    `${ALB_SESSION_COOKIE}=one; ${ALB_SESSION_COOKIE}=two`,
    `${ALB_SESSION_COOKIE}-0=one; ${ALB_SESSION_COOKIE}-0=two`,
    `${ALB_SESSION_COOKIE}=one; ${ALB_SESSION_COOKIE}-0=two`,
  ]) {
    assert.equal(hasAmbiguousALBSessionCookies(header), true);
  }
});
