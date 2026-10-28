import { notFound } from "next/navigation";

// Fail closed until the real published-app gateway and access policy are wired.
// Never reveal project metadata or serve generated HTML on the SG-1 origin.
export default function ApplicationViewer() {
  notFound();
}
