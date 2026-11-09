import { Brand } from "@/components/brand";

export default function SignedOutPage() {
  return (
    <main className="standalone-state">
      <Brand />
      <h1>You have signed out.</h1>
      <p>Your identity provider may keep its own session active.</p>
      <form action="/" method="get">
        <button className="button primary">Sign in again</button>
      </form>
    </main>
  );
}
