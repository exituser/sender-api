import type { Metadata } from "next";
import Link from "next/link";
import { BrandMark } from "@/components/brand-mark";

export const metadata: Metadata = {
  title: "Sender API | Transactional email infrastructure",
  description: "A transactional email API for SaaS products, marketplaces, and mobile apps.",
};

const capabilities = [
  {
    number: "01",
    title: "Ship email without the plumbing",
    body: "Queue, send, and inspect every message through one predictable contract. Idempotency keys keep retries from becoming duplicates.",
  },
  {
    number: "02",
    title: "Turn your domain into a channel",
    body: "Add a sending domain, publish the DNS records, and see SPF, DKIM, DMARC, and SES status together in one dashboard.",
  },
  {
    number: "03",
    title: "Know what happened next",
    body: "Bounces, complaints, and delivery events stay connected to the original message so your product can respond with confidence.",
  },
];

function CodeLine({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <span className={`block ${className}`}>{children}</span>;
}

export default function Home() {
  return (
    <main className="landing-shell overflow-hidden bg-[#f6f4ef] text-[#172027]">
      <a className="skip-link" href="#main-content">Skip to content</a>

      <nav className="landing-nav" aria-label="Public navigation">
        <Link href="/" className="landing-brand" aria-label="Sender API home">
          <BrandMark />
          <span>Sender API</span>
        </Link>
        <div className="hidden items-center gap-7 md:flex">
          <a href="#how-it-works" className="landing-nav-link">How it works</a>
          <Link href="/docs" className="landing-nav-link">API docs</Link>
          <Link href="/pricing" className="landing-nav-link">Plans</Link>
        </div>
        <div className="flex items-center gap-3">
          <Link href="/login" className="landing-nav-link hidden sm:inline">Sign in</Link>
          <Link href="/signup" className="landing-button landing-button-small">Create free account</Link>
        </div>
      </nav>

      <div id="main-content">
        <section className="landing-hero landing-container">
          <div className="landing-hero-copy">
            <p className="landing-eyebrow"><span className="landing-pulse" aria-hidden="true" /> For SaaS, marketplaces, and mobile apps</p>
            <h1>Transactional email API.<br /><em>Built to ship.</em></h1>
            <p className="landing-hero-text">
              Send receipts, password resets, invites, and alerts from your own domain. One API for delivery, retries, and the events that follow.
            </p>
            <div className="flex flex-wrap items-center gap-4">
              <Link href="/signup" className="landing-button">Create a free account <span aria-hidden="true">-&gt;</span></Link>
              <Link href="/docs" className="landing-text-link">Read the API docs <span aria-hidden="true">-&gt;</span></Link>
            </div>
            <p className="landing-caption">Built on Amazon SES. Designed for transactional traffic, not bulk campaigns.</p>
          </div>

          <div className="landing-hero-visual" aria-label="Sender API delivery preview">
            <div className="landing-orbit-label">EXAMPLE WORKSPACE / 01</div>
            <div className="landing-preview">
              <div className="landing-preview-topbar">
                <span className="landing-window-dots" aria-hidden="true"><i /><i /><i /></span>
                <span>sender-api / emails</span>
                <span className="landing-preview-live">DEMO DATA</span>
              </div>
              <div className="landing-preview-body">
                <div className="landing-preview-sidebar">
                  <span className="landing-preview-logo"><BrandMark size={23} /></span>
                  <span className="landing-sidebar-active">Emails</span>
                  <span>Contacts</span>
                  <span>Domains</span>
                  <span>Webhooks</span>
                </div>
                <div className="landing-preview-main">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <p className="landing-preview-kicker">Message trace</p>
                      <p className="landing-preview-title">Delivery overview</p>
                    </div>
                    <span className="landing-status">Operational</span>
                  </div>
                  <div className="landing-metrics">
                    <div><strong>201</strong><span>Request accepted</span></div>
                    <div><strong>sent</strong><span>Provider state</span></div>
                    <div><strong>event</strong><span>Trace available</span></div>
                  </div>
                  <div className="landing-chart" aria-hidden="true">
                    <span className="landing-chart-grid grid-one" />
                    <span className="landing-chart-grid grid-two" />
                    <svg viewBox="0 0 420 112" preserveAspectRatio="none">
                      <path d="M0 94 C28 91, 34 66, 61 72 S88 86, 110 61 S140 63, 162 55 S194 70, 214 43 S242 52, 264 42 S292 51, 316 25 S346 37, 366 21 S397 25, 420 8" />
                    </svg>
                    <span className="landing-chart-label">EVENT TRACE</span>
                  </div>
                  <div className="landing-message-row">
                    <span className="landing-message-dot" aria-hidden="true" />
                    <span>Welcome to the team</span>
                    <code>delivered</code>
                    <time>provider event</time>
                  </div>
                </div>
              </div>
            </div>
            <div className="landing-hero-note"><span>01</span> From API request to provider event in one trace.</div>
          </div>
        </section>

        <div className="landing-rule" />

        <section className="landing-container landing-section landing-workflow-section">
          <div className="landing-section-heading landing-section-heading-wide">
            <p className="landing-eyebrow">From domain to delivery</p>
            <h2>Three steps between<br /><span>your product and inbox.</span></h2>
          </div>
          <div className="landing-workflow">
            <article><span>01</span><h3>Connect your domain</h3><p>Publish the DNS records and see sender verification status in one place.</p></article>
            <article><span>02</span><h3>Send through one API</h3><p>Use a scoped key, an idempotency key, and the endpoint your product already understands.</p></article>
            <article><span>03</span><h3>Act on what happened</h3><p>Follow delivery, bounce, complaint, and webhook events back to the original send.</p></article>
          </div>
        </section>

        <section id="how-it-works" className="landing-container landing-section">
          <div className="landing-section-heading">
            <p className="landing-eyebrow">A real API contract</p>
            <h2>Go from zero to<br /><span>your first send.</span></h2>
          </div>
          <div id="quickstart" className="landing-quickstart">
            <div className="landing-code-panel">
              <div className="landing-code-heading"><span>POST</span> /api/v1/emails</div>
              <pre aria-label="Example email API request"><CodeLine><span className="code-muted">curl</span> https://chydo.lol/api/v1/emails \</CodeLine><CodeLine>  <span className="code-muted">-H</span> <span className="code-string">&quot;Authorization: Bearer re_live_...&quot;</span> \</CodeLine><CodeLine>  <span className="code-muted">-H</span> <span className="code-string">&quot;Idempotency-Key: welcome-user-42&quot;</span> \</CodeLine><CodeLine>  <span className="code-muted">-H</span> <span className="code-string">&quot;Content-Type: application/json&quot;</span> \</CodeLine><CodeLine>  <span className="code-muted">-d</span> <span className="code-string">&apos;{`{`}&quot;from&quot;: &quot;hello@yourdomain.com&quot;,</span></CodeLine><CodeLine>     <span className="code-string">&quot;to&quot;: [&quot;you@example.com&quot;],</span></CodeLine><CodeLine>     <span className="code-string">&quot;subject&quot;: &quot;Welcome&quot;{`}`}&apos;</span></CodeLine></pre>
              <div className="landing-code-result"><span>201</span><code>{`{"id":"019..."}`}</code></div>
            </div>
            <div className="landing-quickstart-copy">
              <p className="landing-eyebrow">A small contract</p>
              <h3>Your first send is the easy part.</h3>
              <p>Every request gets an ID, a durable status, and a place in the event history. Retry safely without creating duplicate messages.</p>
              <dl className="landing-contract-list">
                <div><dt>01</dt><dd>API keys scoped to a team</dd></div>
                <div><dt>02</dt><dd>Idempotent retries</dd></div>
                <div><dt>03</dt><dd>Delivery events connected to sends</dd></div>
              </dl>
            </div>
          </div>
        </section>

        <section id="reliability" className="landing-container landing-section landing-capabilities-section">
          <div className="landing-section-heading landing-section-heading-wide">
            <p className="landing-eyebrow">Built around the edge cases</p>
            <h2>Reliable primitives<br /><span>for product teams.</span></h2>
          </div>
          <div className="landing-capabilities">
            {capabilities.map((capability) => (
              <article key={capability.number} className="landing-capability">
                <span className="landing-capability-number">{capability.number}</span>
                <h3>{capability.title}</h3>
                <p>{capability.body}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="landing-trust-band landing-container" aria-label="Sender API foundations">
          <p className="landing-eyebrow">The foundations</p>
          <div className="landing-trust-items">
            <span>Scoped API keys</span>
            <span>Idempotent retries</span>
            <span>SPF / DKIM / DMARC</span>
            <span>Delivery webhooks</span>
            <span>Amazon SES</span>
          </div>
        </section>

        <section className="landing-cta landing-container">
          <div>
            <p className="landing-eyebrow">Ready when your product is</p>
            <h2>Make every product email<br /><em>feel intentional.</em></h2>
          </div>
          <div className="landing-cta-action">
            <p>Bring your domain and your next user journey. Start with the free plan, then scale when the product is ready.</p>
            <div className="flex flex-wrap items-center gap-4">
              <Link href="/signup" className="landing-button">Create a free account <span aria-hidden="true">-&gt;</span></Link>
              <Link href="/pricing" className="landing-text-link">Compare plans <span aria-hidden="true">-&gt;</span></Link>
            </div>
          </div>
        </section>
      </div>

      <footer className="landing-footer landing-container">
        <div className="landing-brand"><BrandMark /><span>Sender API</span></div>
        <p>Transactional email, without the detour.</p>
        <div className="landing-footer-links">
          <Link href="/privacy" className="landing-footer-link">Privacy</Link>
          <Link href="/terms" className="landing-footer-link">Terms</Link>
          <Link href="/docs" className="landing-footer-link">API docs</Link>
          <Link href="/pricing" className="landing-footer-link">Plans</Link>
          <Link href="/subprocessors" className="landing-footer-link">Subprocessors</Link>
          <Link href="/refunds" className="landing-footer-link">Refunds</Link>
          <Link href="/login" className="landing-footer-link">Sign in</Link>
        </div>
      </footer>
    </main>
  );
}
