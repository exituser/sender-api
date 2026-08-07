import type { Metadata } from "next";
import Link from "next/link";
import { BrandMark } from "@/components/brand-mark";

export const metadata: Metadata = {
  title: "Plans | Sender API",
  description: "Sender API plans and daily recipient limits.",
};

const plans = [
  {
    name: "Free",
    label: "For development and early product flows",
    limit: "1,000 recipients / day",
    details: ["Transactional email API", "Sender domain verification", "Delivery history", "Team-scoped API keys"],
  },
  {
    name: "Pro",
    label: "For growing transactional traffic",
    limit: "10,000 recipients / day",
    details: ["Everything in Free", "Delivery webhooks", "Higher daily limit", "Priority support"],
  },
  {
    name: "Scale",
    label: "For products with established volume",
    limit: "100,000 recipients / day",
    details: ["Everything in Pro", "Highest daily limit", "Inbound email support", "Priority support"],
  },
];

export default function PricingPage() {
  return (
    <main className="pricing-shell">
      <nav className="docs-nav" aria-label="Plans navigation">
        <Link href="/" className="landing-brand" aria-label="Sender API home">
          <BrandMark />
          <span>Sender API</span>
        </Link>
        <div className="docs-nav-links">
          <Link href="/docs">API docs</Link>
          <Link href="/signup" className="landing-button landing-button-small">Create an account</Link>
        </div>
      </nav>

      <div className="pricing-container">
        <div className="pricing-intro">
          <p className="landing-eyebrow">Plans / 01</p>
          <h1>Start small.<br /><em>Scale with the send.</em></h1>
          <p>Start with the free plan, then move up as your product sends more messages and needs more delivery tools.</p>
        </div>

        <div className="pricing-grid">
          {plans.map((plan, index) => (
            <article className={`pricing-plan${index === 1 ? " pricing-plan-featured" : ""}`} key={plan.name}>
              {index === 1 && <span className="pricing-plan-badge">Most capacity</span>}
              <p className="pricing-plan-number">0{index + 1}</p>
              <h2>{plan.name}</h2>
              <p className="pricing-plan-label">{plan.label}</p>
              <strong className="pricing-plan-limit">{plan.limit}</strong>
              <ul>
                {plan.details.map((detail) => <li key={detail}>{detail}</li>)}
              </ul>
              <Link href="/signup" className={index === 1 ? "landing-button" : "pricing-plan-link"}>{index === 0 ? "Create a free account" : `Start with ${plan.name}`}</Link>
            </article>
          ))}
        </div>

        <p className="pricing-note">Paid-plan prices are shown at checkout. Plan limits and features are listed above.</p>

        <section className="pricing-bottom">
          <div><p className="landing-eyebrow">No migration tax</p><h2>Every plan uses the same API.</h2></div>
          <Link href="/docs" className="landing-text-link">Read the API docs <span aria-hidden="true">-&gt;</span></Link>
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
