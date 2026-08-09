import type { Metadata } from "next";
import Link from "next/link";
import { BrandMark } from "@/components/brand-mark";

export const metadata: Metadata = {
  title: "API Docs | Sender API",
  description: "Send transactional email with the Sender API.",
};

function CodeLine({ children }: { children: React.ReactNode }) {
  return <span className="block">{children}</span>;
}

export default function DocsPage() {
  return (
    <main className="docs-shell">
      <nav className="docs-nav" aria-label="Documentation navigation">
        <Link href="/" className="landing-brand" aria-label="Sender API home">
          <BrandMark />
          <span>Sender API</span>
        </Link>
        <div className="docs-nav-links">
          <Link href="/pricing">Plans</Link>
          <Link href="/signup" className="landing-button landing-button-small">Create an account</Link>
        </div>
      </nav>

      <div className="docs-container">
        <p className="landing-eyebrow">API reference / 01</p>
        <h1>Send your first<br /><em>transactional email.</em></h1>
        <p className="docs-lead">The shortest path from a product event to a message you can follow. Authenticate with a workspace API key, send through one endpoint, and see what happens next.</p>

        <section className="docs-section">
          <div className="docs-section-heading">
            <div><span className="docs-method">POST</span><code>/api/v1/emails</code></div>
            <p>Queue one message for delivery.</p>
          </div>
          <div className="docs-code-panel">
            <pre aria-label="Send an email with curl"><CodeLine><span className="code-muted">curl</span> https://chydo.lol/api/v1/emails \</CodeLine><CodeLine>  <span className="code-muted">-H</span> <span className="code-string">&quot;Authorization: Bearer re_live_...&quot;</span> \</CodeLine><CodeLine>  <span className="code-muted">-H</span> <span className="code-string">&quot;Idempotency-Key: welcome-user-42&quot;</span> \</CodeLine><CodeLine>  <span className="code-muted">-H</span> <span className="code-string">&quot;Content-Type: application/json&quot;</span> \</CodeLine><CodeLine>  <span className="code-muted">-d</span> <span className="code-string">&apos;{`{`}&quot;from&quot;: &quot;hello@yourdomain.com&quot;,</span></CodeLine><CodeLine>     <span className="code-string">&quot;to&quot;: [&quot;you@example.com&quot;],</span></CodeLine><CodeLine>     <span className="code-string">&quot;subject&quot;: &quot;Welcome&quot;,</span></CodeLine><CodeLine>     <span className="code-string">&quot;text&quot;: &quot;Welcome to the team&quot;{`}`}&apos;</span></CodeLine></pre>
            <div className="docs-response"><span>201</span><code>{`{"id":"019..."}`}</code></div>
          </div>
        </section>

        <section className="docs-section docs-contract-grid">
          <article>
            <span className="docs-number">01</span>
            <h2>Retry safely with one key</h2>
            <p>Send an <code>Idempotency-Key</code> and reuse it if a request times out. The API returns the original message instead of creating a duplicate.</p>
          </article>
          <article>
            <span className="docs-number">02</span>
            <h2>See what happened</h2>
            <p>Read the message status and its history, then receive the same updates in your product through signed webhooks.</p>
          </article>
          <article>
            <span className="docs-number">03</span>
            <h2>Know when your domain is ready</h2>
            <p>Connect a sender domain and follow each setup check from one clear dashboard.</p>
          </article>
        </section>

        <section className="docs-next-step">
          <div>
            <p className="landing-eyebrow">Ready to test it?</p>
            <h2>Create a workspace and send from your domain.</h2>
          </div>
          <div className="docs-next-step-actions">
            <Link href="/signup" className="landing-button">Create a free account <span aria-hidden="true">-&gt;</span></Link>
            <Link href="/pricing" className="landing-text-link">Compare plans <span aria-hidden="true">-&gt;</span></Link>
          </div>
        </section>

        <footer className="docs-footer">
          <Link href="/privacy">Privacy</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/subprocessors">Subprocessors</Link>
          <Link href="/refunds">Refunds</Link>
        </footer>
      </div>
    </main>
  );
}
