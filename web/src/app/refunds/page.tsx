import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Refunds Policy | Sender API",
  description: "Refund and cancellation terms for Sender API plans.",
};

export default function RefundsPage() {
  return (
    <main className="legal-shell">
      <div className="legal-container">
        <Link href="/" className="legal-back">&lt;- Sender API</Link>
        <p className="legal-kicker">Legal / 04</p>
        <h1>Refunds Policy</h1>
        <p className="legal-lead">A straightforward policy for paid Sender API plans, billing mistakes, and cancellations.</p>
        <div className="legal-meta">
          <span>Last updated: 8 August 2026</span>
          <span>Contact: support@chydo.lol</span>
        </div>

        <section className="legal-section">
          <h2>1. Cancel before renewal</h2>
          <p>You can cancel a recurring plan from the billing area. Cancellation stops the next renewal; the current paid period normally remains active until its scheduled end date.</p>
        </section>

        <section className="legal-section">
          <h2>2. When we issue refunds</h2>
          <ul>
            <li>Duplicate or clearly incorrect charges caused by a billing error.</li>
            <li>A refund required by applicable consumer law.</li>
            <li>A material service issue that prevented meaningful use of a newly purchased plan, when the issue is confirmed and a refund is appropriate.</li>
            <li>Another exception approved by Sender API support after reviewing the account and payment.</li>
          </ul>
        </section>

        <section className="legal-section">
          <h2>3. What is generally not refundable</h2>
          <p>Unused time after a voluntary cancellation, messages already accepted for delivery, and charges resulting from activity under your account are generally not refundable unless required by law or approved as an exception. A plan can have its own checkout-specific terms.</p>
        </section>

        <section className="legal-section">
          <h2>4. How to request a refund</h2>
          <p>Email <a href="mailto:support@chydo.lol">support@chydo.lol</a> from the account email within 14 days of the charge. Include the workspace, charge date, amount, and a short explanation. We may ask for the payment receipt or transaction ID. Approved refunds are sent to the original payment method; processing time depends on the payment provider.</p>
        </section>

        <section className="legal-section">
          <h2>5. Account closure</h2>
          <p>Closing an account does not automatically reverse prior charges. It may also remove access to delivery history and other workspace data, subject to the <Link href="/privacy">Privacy Policy</Link> and any retention required for security, fraud prevention, disputes, or accounting.</p>
        </section>

        <p className="legal-contact">Billing contact: <a href="mailto:support@chydo.lol">support@chydo.lol</a></p>
        <div className="legal-links" aria-label="Legal pages">
          <Link href="/privacy">Privacy</Link>
          <Link href="/terms">Terms</Link>
          <Link href="/subprocessors">Subprocessors</Link>
        </div>
      </div>
    </main>
  );
}
