import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Privacy Policy | Sender API",
  description: "How Sender API processes account, message, and delivery data.",
};

export default function PrivacyPage() {
  return (
    <main className="legal-shell">
      <div className="legal-container">
        <Link href="/" className="legal-back">&lt;- Sender API</Link>
        <p className="legal-kicker">Legal / 01</p>
        <h1>Privacy Policy</h1>
        <p className="legal-lead">Sender API provides transactional email infrastructure. This policy explains what we process, why we process it, and the choices available to you.</p>
        <div className="legal-meta">
          <span>Last updated: 8 August 2026</span>
          <span>Contact: support@chydo.lol</span>
        </div>

        <section className="legal-section">
          <h2>1. Scope and roles</h2>
          <p>When you create an account, Sender API processes information needed to provide the service. If you use Sender API on behalf of a company, that company normally decides why recipient and message data is processed, while Sender API processes it on the company&apos;s instructions. You remain responsible for providing recipients with any notices required by applicable law.</p>
        </section>

        <section className="legal-section">
          <h2>2. Information we process</h2>
          <ul>
            <li>Account and authentication data, such as your email address, user ID, sign-in events, and password-reset activity.</li>
            <li>Workspace data, including team members, roles, sender domains, API key names and hashes, webhooks, contacts, and billing identifiers.</li>
            <li>Email data you submit, including sender and recipient addresses, subject, message content, attachments, headers, tags, and metadata.</li>
            <li>Delivery and inbound events, including provider message IDs, delivery status, bounces, complaints, timestamps, and event payloads.</li>
            <li>Technical and support information needed to secure, troubleshoot, and maintain the service.</li>
          </ul>
        </section>

        <section className="legal-section">
          <h2>3. How we use information</h2>
          <ul>
            <li>Authenticate users, manage workspaces, and provide the dashboard and API.</li>
            <li>Queue, deliver, and report on requested messages through our email provider.</li>
            <li>Process bounces, complaints, suppressions, inbound messages, and webhooks.</li>
            <li>Prevent fraud, abuse, spam, unauthorized access, and damage to sender reputation.</li>
            <li>Provide support, maintain security, improve reliability, and meet legal obligations.</li>
          </ul>
        </section>

        <section className="legal-section">
          <h2>4. Subprocessors</h2>
          <p>We use the providers listed in our <Link href="/subprocessors">Subprocessors List</Link> to operate Sender API. They receive only the information needed for their documented service.</p>
        </section>

        <section className="legal-section">
          <h2>5. Retention and deletion</h2>
          <p>We keep information while it is needed to provide the service, maintain delivery history, prevent abuse, resolve disputes, and meet legal or accounting obligations. Retention depends on the data type and account state. You can request account-data export or deletion at <a href="mailto:support@chydo.lol">support@chydo.lol</a>; we may retain limited records where required for security, fraud prevention, or law.</p>
        </section>

        <section className="legal-section">
          <h2>6. Security</h2>
          <p>We use access controls, scoped API keys, encrypted HTTPS connections, provider security controls, and operational monitoring appropriate to the service. No online service can guarantee absolute security. Protect your credentials and rotate API keys if you suspect exposure.</p>
        </section>

        <section className="legal-section">
          <h2>7. Your choices</h2>
          <p>You may access and update account information in the product, delete API keys, manage sender domains, and contact us about access, correction, export, or deletion requests. Depending on your location, you may also have additional rights under applicable privacy law.</p>
        </section>

        <p className="legal-contact">Privacy contact: <a href="mailto:support@chydo.lol">support@chydo.lol</a></p>
        <div className="legal-links" aria-label="Legal pages">
          <Link href="/terms">Terms</Link>
          <Link href="/subprocessors">Subprocessors</Link>
          <Link href="/refunds">Refunds</Link>
        </div>
      </div>
    </main>
  );
}
