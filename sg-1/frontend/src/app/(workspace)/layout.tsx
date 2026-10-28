import { Shell } from "@/components/shell";
import { getMe } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function WorkspaceLayout({ children }: { children: React.ReactNode }) {
  const { user } = await getMe();
  return <Shell user={user}>{children}</Shell>;
}
