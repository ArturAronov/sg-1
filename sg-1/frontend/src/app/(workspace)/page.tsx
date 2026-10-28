import { ProjectDashboard } from "@/components/project-dashboard";
import { apiGet } from "@/lib/api";
import type { Department, Project } from "@/lib/types";

export default async function ProjectsPage() {
  const [{ projects }, { departments }] = await Promise.all([
    apiGet<{ projects: Project[] }>("/v1/projects"),
    apiGet<{ departments: Department[] }>("/v1/departments"),
  ]);
  return <ProjectDashboard projects={projects} departments={departments} />;
}
