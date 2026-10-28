import Link from "next/link";
import { Icon } from "@/components/icon";

export default function NotFound() {
  return (
    <main className="standalone-state">
      <span className="project-icon color-0">
        <Icon name="layers" size={26} />
      </span>
      <span className="tiny-label">NOT AVAILABLE</span>
      <h1>Nothing published here yet.</h1>
      <p>
        This address has no available application. Application serving is not connected in this
        foundation build.
      </p>
      <Link href="/" className="button primary">
        Go to workspace
        <Icon name="arrow" size={16} />
      </Link>
    </main>
  );
}
