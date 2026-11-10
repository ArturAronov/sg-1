import { redirect } from "next/navigation";
import { Brand } from "@/components/brand";
import { Icon } from "@/components/icon";
import { developmentEnabled, sessionToken } from "@/lib/auth";

export const dynamic = "force-dynamic";

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string; expired?: string }>;
}) {
  const params = await searchParams;
  if (!params.expired && (await sessionToken())) redirect("/");
  const development = developmentEnabled();
  return (
    <main className="login-page">
      <div className="login-story">
        <Brand />
        <div>
          <span className="eyebrow">A LITTLE LESS BUSYWORK.</span>
          <h1>
            Your next good idea
            <br />
            deserves a <em>workspace.</em>
          </h1>
          <p>
            Useful tools, built around your people.
            <br />
            One conversation at a time.
          </p>
        </div>
        <span className="tiny-label">SG-1 / INTERNAL APPLICATION STUDIO</span>
      </div>
      <div className="login-form-wrap">
        <div className="login-form">
          <span className="project-icon color-0">
            <Icon name="shield" size={23} />
          </span>
          <h2>{development ? "Welcome to the studio." : "Employee sign-in required."}</h2>
          <p>
            {development
              ? "Use your local development token to open this workspace."
              : "Use your company’s configured sign-in provider to open this workspace."}
          </p>
          {development && (
            <form action="/api/session" method="post">
              <label>
                Development token
                <input
                  name="token"
                  type="password"
                  autoComplete="off"
                  required
                  minLength={32}
                  maxLength={512}
                  placeholder="Your SG1_DEV_TOKEN"
                  aria-invalid={!!params.error}
                  aria-describedby={params.error ? "login-error" : undefined}
                />
              </label>
              {params.error && (
                <p role="alert" className="form-error" id="login-error">
                  Unable to sign in. Check your token and try again.
                </p>
              )}
              {params.expired && (
                <p role="status" className="form-error">
                  The backend session is unavailable. Check API configuration and seed-dev.
                </p>
              )}
              <button className="button primary full-width">
                Open workspace
                <Icon name="arrow" size={16} />
              </button>
            </form>
          )}
          {!development && (
            <form action="/" method="get">
              <button className="button primary full-width">Continue with company sign-in</button>
            </form>
          )}
          <div className="info-box">
            <Icon name="code" size={17} />
            <span>
              {development
                ? "Local development only. This is not employee SSO and must not be exposed publicly."
                : "Employee access is protected by your company identity provider."}
            </span>
          </div>
        </div>
      </div>
    </main>
  );
}
