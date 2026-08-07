import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Terms of Service | Sender API",
  description: "The rules for using Sender API transactional email infrastructure.",
};

export default function TermsPage() {
  return (
    <main className="legal-shell">
      <div className="legal-container">
        <Link href="/" className="legal-back">&lt;- Sender API</Link>
        <p className="legal-kicker">Legal / 02</p>
        <h1>Terms of Service</h1>
        <p className="legal-lead">These baseline terms keep Sender API useful for legitimate product email and set clear expectations around accounts, content, billing, and availability.</p>
        <div className="legal-meta">
          <span>Last updated: 8 August 2026</span>
          <span>Contact: support@chydo.lol</span>
        </div>

        <section className="legal-section">
          <h2>1. Accepting these terms</h2>
          <p>By creating an account or using Sender API, you agree to these Terms of Service and the <Link href="/privacy">Privacy Policy</Link>. If you use the service for an organization, you confirm that you can bind that organization to these terms.</p>
        </section>

        <section className="legal-section">
          <h2>2. Your account</h2>
          <p>Provide accurate account information, keep credentials and API keys confidential, and notify us promptly about unauthorized access. You are responsible for activity under your account and for the people you invite to your workspace.</p>
        </section>

        <section className="legal-section">
          <h2>3. Email and acceptable use</h2>
          <p>Use Sender API for permission-based, lawful transactional messages such as receipts, account notices, invitations, alerts, and password resets. You must control the sender domains you connect and have a lawful basis to contact each recipient.</p>
          <p>Do not send spam, phishing, malware, deceptive or unlawful content, unsolicited bulk campaigns, messages to purchased lists, or traffic intended to evade provider controls or damage delivery reputation. We may restrict or suspend traffic that creates a security, legal, abuse, or deliverability risk.</p>
        </section>

        <section className="legal-section">
          <h2>4. Your content</h2>
          <p>You retain responsibility for the content, recipients, sender identities, domains, and instructions you submit. You grant Sender API and its subprocessors the limited permission needed to host, process, transmit, and store that content to provide the service and its security controls.</p>
        </section>

        <section className="legal-section">
          <h2>5. Billing and cancellation</h2>
          <p>Paid plans, prices, included limits, and billing intervals are shown at checkout or in the billing area. Recurring plans renew until cancelled. Cancellation stops the next renewal; access and paid features may remain available through the current billing period. Taxes, payment-provider terms, and any checkout-specific terms may also apply.</p>
          <p>Refund requests are handled under our <Link href="/refunds">Refunds Policy</Link>.</p>
        </section>

        <section className="legal-section">
          <h2>6. Service and third parties</h2>
          <p>Sender API depends on hosting, database, authentication, email, payment, DNS, recipient, and other third-party systems. Delivery may be delayed, rejected, or affected by provider limits, recipient systems, network failures, or events outside our reasonable control. See the <Link href="/subprocessors">Subprocessors List</Link> for the current core providers.</p>
        </section>

        <section className="legal-section">
          <h2>7. Suspension and termination</h2>
          <p>We may suspend or terminate access when necessary to prevent abuse, protect the service or other users, comply with law, address non-payment, or respond to a material breach. Where practical, we will provide notice and an opportunity to resolve the issue. You may stop using the service at any time.</p>
        </section>

        <section className="legal-section">
          <h2>8. Disclaimers and limits</h2>
          <p>The service is provided on an &quot;as available&quot; basis. We do not promise that every message will reach its recipient or that the service will be uninterrupted. To the extent allowed by law, neither party is liable for indirect, incidental, special, consequential, or lost-profit damages.</p>
        </section>

        <p className="legal-contact">Support contact: <a href="mailto:support@chydo.lol">support@chydo.lol</a></p>
        <div className="legal-links" aria-label="Legal pages">
          <Link href="/privacy">Privacy</Link>
          <Link href="/subprocessors">Subprocessors</Link>
          <Link href="/refunds">Refunds</Link>
        </div>
      </div>
    </main>
  );
}
