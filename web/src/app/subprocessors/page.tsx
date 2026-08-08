import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Subprocessors | Sender API",
  description: "The service providers Sender API uses to operate transactional email infrastructure.",
};

const providers = [
  { name: "OVHcloud", role: "Hosting", details: "Hosts the application services and operational infrastructure used to run Sender API." },
  { name: "Amazon SES", role: "Email delivery", details: "Transmits requested email and returns delivery, bounce, complaint, and other provider events." },
  { name: "Supabase", role: "Authentication and managed database (when enabled)", details: "Provides authentication for user accounts. Managed PostgreSQL storage is used only when the deployment is configured to use Supabase Database; the current Compose profile keeps PostgreSQL on the application host." },
  { name: "Stripe", role: "Billing", details: "Processes subscriptions, checkout, customer portal, and payment status when paid plans are enabled." },
];

export default function SubprocessorsPage() {
  return (
    <main className="legal-shell">
      <div className="legal-container">
        <Link href="/" className="legal-back">&lt;- Sender API</Link>
        <p className="legal-kicker">Legal / 03</p>
        <h1>Subprocessors</h1>
        <p className="legal-lead">Sender API uses a small set of infrastructure providers to host the product, authenticate users, store operational data, deliver email, and process billing.</p>
        <div className="legal-meta">
          <span>Last updated: 8 August 2026</span>
          <span>Contact: support@chydo.lol</span>
        </div>

        <section className="legal-section">
          <h2>Current provider list</h2>
          <p>Each provider processes data only for the service described below and under its own terms and security practices. The list may change as the service evolves; material changes will be reflected on this page.</p>
          <div className="legal-provider-list">
            {providers.map((provider) => (
              <article className="legal-provider" key={provider.name}>
                <h3>{provider.name}<br /><span>{provider.role}</span></h3>
                <p>{provider.details}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="legal-section">
          <h2>What this means for your data</h2>
          <p>Depending on the feature you use, providers may process account identifiers, sender and recipient addresses, message content, delivery metadata, authentication data, billing identifiers, and technical logs. The <Link href="/privacy">Privacy Policy</Link> explains the purposes and retention approach.</p>
        </section>

        <section className="legal-section">
          <h2>Questions and objections</h2>
          <p>For provider questions, privacy requests, or a current list confirmation, contact <a href="mailto:support@chydo.lol">support@chydo.lol</a>.</p>
        </section>

        <div className="legal-links" aria-label="Legal pages">
          <Link href="/privacy">Privacy</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/refunds">Refunds</Link>
        </div>
      </div>
    </main>
  );
}
