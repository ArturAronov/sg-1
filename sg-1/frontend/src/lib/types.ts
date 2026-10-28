export type Role = "read" | "write" | "admin";

export type User = { id: string; email: string; name: string; platform_admin: boolean };
export type Department = { id: string; name: string; slug: string; role: Role };
export type Project = {
  id: string;
  department_id: string;
  department_name: string;
  name: string;
  slug: string;
  description: string;
  status: "provisioning" | "ready" | "failed" | "archiving" | "archived";
  access_mode: "password" | "public";
  role: Role;
  created_at: string;
};
export type Me = {
  user: User;
  authentication: "development";
  capabilities: { provisioning: boolean; ai_builds: boolean; sso: boolean };
};
