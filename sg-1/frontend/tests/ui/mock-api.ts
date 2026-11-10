import { createServer } from "node:http";
import type { Project, LLMSettings } from "../../src/lib/types";

const projects: Project[] = [
  {
    id: "theme-property",
    department_id: "property",
    department_name: "Property management",
    name: "Property hub",
    slug: "property-hub",
    description: "A shared home for properties, maintenance, and the people who make it work.",
    status: "ready",
    access_mode: "password",
    role: "admin",
    created_at: "2026-11-09T12:00:00Z",
  },
  {
    id: "theme-requests",
    department_id: "operations",
    department_name: "Operations",
    name: "Team requests",
    slug: "team-requests",
    description: "Less back-and-forth. A simpler way to keep team requests moving.",
    status: "provisioning",
    access_mode: "password",
    role: "admin",
    created_at: "2026-11-09T12:00:00Z",
  },
  {
    id: "theme-inventory",
    department_id: "operations",
    department_name: "Operations",
    name: "Office inventory",
    slug: "office-inventory",
    description: "Keep track of the things your team needs, all in one place.",
    status: "failed",
    access_mode: "public",
    role: "admin",
    created_at: "2026-11-09T12:00:00Z",
  },
];
const settings: LLMSettings = {
  providers: [
    { id: "openai", name: "OpenAI", description: "Build with OpenAI models." },
    { id: "anthropic", name: "Anthropic", description: "Build with Claude models." },
    { id: "glm", name: "GLM", description: "Build with GLM models." },
    { id: "kimi", name: "Kimi", description: "Build with Kimi models." },
  ],
  credentials: [{ provider: "openai", configured: true, updated_at: "2026-11-09T12:00:00Z" }],
};

createServer((request, response) => {
  const path = new URL(request.url!, "http://127.0.0.1:3108").pathname;
  response.setHeader("Content-Type", "application/json");
  const send = (value: unknown) => response.end(JSON.stringify(value));
  if (path === "/healthz") return send({ ok: true });
  if (path === "/public/projects/members-app" || path === "/public/projects/denied-app") {
    return send({ url: "http://127.0.0.1:3108/published-fixture", mode: "members" });
  }
  if (path === "/public/projects/property-hub") {
    if (request.headers.authorization || request.headers.cookie) {
      response.statusCode = 400;
      return send({ error: { message: "Viewer must not forward employee credentials" } });
    }
    return send({ url: "http://127.0.0.1:3108/published-fixture" });
  }
  if (path.startsWith("/public/projects/")) {
    response.statusCode = 404;
    return send({ error: { message: "No published application" } });
  }
  if (path === "/published-fixture") {
    response.setHeader("Content-Type", "text/html; charset=utf-8");
    return response.end("<!doctype html><html><body><h1>Published fixture app</h1></body></html>");
  }
  if (request.headers.authorization !== "Bearer theme-test-only-not-a-real-secret-00000000") {
    response.statusCode = 401;
    return send({ error: { message: "Test authentication required" } });
  }
  if (path === "/v1/projects/members-app/member-session")
    return send({ url: "http://127.0.0.1:3108/published-fixture" });
  if (path === "/v1/projects/denied-app/member-session") {
    response.statusCode = 403;
    return send({ error: { message: "Project membership required" } });
  }
  if (path === "/v1/me")
    return send({
      user: {
        id: "theme-user",
        name: "Local developer",
        email: "developer@localhost",
        platform_admin: true,
      },
      authentication: "development",
      capabilities: { provisioning: true, ai_builds: true, sso: false },
    });
  if (path === "/v1/departments")
    return send({
      departments: [
        { id: "property", name: "Property management", slug: "property", role: "admin" },
        { id: "operations", name: "Operations", slug: "operations", role: "admin" },
      ],
    });
  if (path === "/v1/projects") return send({ projects });
  if (path === "/v1/settings/llm-providers") return send(settings);
  if (path.endsWith("/runtime"))
    return send({
      project: projects[0],
      messages: [
        {
          id: "1",
          role: "system",
          content: "Your project is running. Describe what you would like to build.",
          created_at: "2026-11-09T12:00:00Z",
        },
        {
          id: "2",
          role: "user",
          content: "A shared workspace for the property team.",
          created_at: "2026-11-09T12:01:00Z",
        },
      ],
      builds: [],
      deployments: [
        { id: "starter", summary: "Starter application", created_at: "2026-11-09T12:00:00Z" },
      ],
      current_deployment_id: "starter",
      last_error: null,
      password_set: true,
    });
  if (path.endsWith("/files")) return send({ files: [] });
  if (path.endsWith("/preview")) return send({ url: "about:blank" });
  response.statusCode = 404;
  send({ error: { message: "Not implemented by the presentation fixture" } });
}).listen(3108, "127.0.0.1");
