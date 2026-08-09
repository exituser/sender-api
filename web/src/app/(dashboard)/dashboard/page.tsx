"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

type AlertSeverity = "critical" | "warning" | "info";

interface DashboardAlert {
  code: string;
  severity: AlertSeverity;
  title: string;
  description: string;
  action_label: string;
  action_href: string;
}

interface DashboardSummary {
  status: "ready" | "attention" | "action_needed";
  status_label: string;
  alerts: DashboardAlert[];
  domains: {
    name: string;
    ready: boolean;
    next_action?: string;
    issues?: string[];
  }[];
  delivery: {
    period_days: number;
    total: number;
    accepted: number;
    delivered: number;
    bounced: number;
    complained: number;
    failed: number;
    uncertain: number;
    queued: number;
  };
  delivery_tracking: {
    configured: boolean;
    label: string;
  };
  audience: {
    unsubscribed_contacts: number;
    suppressed: number;
    unsubscribed: number;
    bounced: number;
    complained: number;
  };
  webhooks: {
    configured: number;
    failed: number;
    pending: number;
  };
  activity: {
    id: string;
    email_id: string;
    event: string;
    subject: string;
    timestamp: string;
  }[];
}

const numberFormatter = new Intl.NumberFormat();

function formatNumber(value: number) {
  return numberFormatter.format(value);
}

function activityLabel(event: string) {
  switch (event) {
    case "email.sent": return "Message accepted";
    case "email.delivered": return "Message delivered";
    case "email.bounced": return "Message bounced";
    case "email.complained": return "Complaint received";
    case "email.failed": return "Message could not be sent";
    case "email.ambiguous": return "Delivery needs review";
    case "email.retrying": return "Message will be retried";
    case "email.opened": return "Message opened";
    case "email.clicked": return "Link clicked";
    default: return "Message updated";
  }
}

function activityTone(event: string) {
  if (event.includes("failed") || event.includes("bounced") || event.includes("complained") || event.includes("ambiguous")) {
    return "dashboard-dot dashboard-dot-alert";
  }
  if (event.includes("delivered") || event.includes("sent")) {
    return "dashboard-dot dashboard-dot-success";
  }
  return "dashboard-dot dashboard-dot-neutral";
}

function alertClasses(severity: AlertSeverity) {
  switch (severity) {
    case "critical":
      return {
        card: "border-red-200 bg-red-50/80",
        icon: "bg-red-100 text-red-700",
        action: "text-red-700 hover:text-red-900",
      };
    case "warning":
      return {
        card: "border-amber-200 bg-amber-50/80",
        icon: "bg-amber-100 text-amber-800",
        action: "text-amber-800 hover:text-amber-950",
      };
    default:
      return {
        card: "border-blue-200 bg-blue-50/80",
        icon: "bg-blue-100 text-blue-700",
        action: "text-blue-700 hover:text-blue-900",
      };
  }
}

function StatusIcon({ status }: { status: DashboardSummary["status"] }) {
  if (status === "ready") {
    return <span aria-hidden="true" className="dashboard-status-icon dashboard-status-icon-ready">✓</span>;
  }
  return <span aria-hidden="true" className="dashboard-status-icon dashboard-status-icon-attention">!</span>;
}

export default function DashboardPage() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");

  const loadSummary = useCallback(async (isRefresh = false) => {
    setError("");
    if (isRefresh) setRefreshing(true);
    else setLoading(true);
    try {
      const data = await api.dashboard.summary() as DashboardSummary;
      setSummary(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load your overview. Try again.");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(() => loadSummary());
  }, [loadSummary]);

  if (loading) {
    return (
      <div className="space-y-6" aria-busy="true" aria-label="Loading overview">
        <div className="dashboard-skeleton dashboard-skeleton-heading" />
        <div className="grid gap-4 lg:grid-cols-3">
          <div className="dashboard-skeleton h-44" />
          <div className="dashboard-skeleton h-44" />
          <div className="dashboard-skeleton h-44" />
        </div>
        <div className="dashboard-skeleton h-64" />
      </div>
    );
  }

  if (!summary) {
    return (
      <div className="dashboard-error-card" role="alert">
        <h1 className="text-lg font-semibold text-gray-950">Unable to load your overview</h1>
        <p className="mt-2 text-sm text-gray-600">Check your connection and try again.</p>
        <button type="button" onClick={() => void loadSummary()} className="dashboard-button dashboard-button-primary mt-5">Try again</button>
      </div>
    );
  }

  const hasAlerts = summary.alerts.length > 0;
  const deliverySentence = summary.delivery.total === 0
    ? "No messages sent in the last 7 days"
    : `${formatNumber(summary.delivery.delivered)} of ${formatNumber(summary.delivery.total)} delivered`;

  return (
    <div className="dashboard-page space-y-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="dashboard-eyebrow">Workspace overview</p>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight text-gray-950">Your sending, at a glance</h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-gray-600">See what is working, what needs attention, and the next best step.</p>
        </div>
        <button type="button" onClick={() => void loadSummary(true)} disabled={refreshing} className="dashboard-button dashboard-button-secondary self-start sm:self-auto" aria-busy={refreshing}>
          <span aria-hidden="true" className={refreshing ? "dashboard-refresh-icon dashboard-refresh-icon-spinning" : "dashboard-refresh-icon"}>↻</span>
          {refreshing ? "Refreshing" : "Refresh overview"}
        </button>
      </div>

      <section className={`dashboard-health dashboard-health-${summary.status}`} aria-labelledby="sending-status-title">
        <div className="flex items-start gap-4">
          <StatusIcon status={summary.status} />
          <div>
            <p className="dashboard-eyebrow">Sending status</p>
            <h2 id="sending-status-title" className="mt-1 text-xl font-semibold text-gray-950">{summary.status_label}</h2>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-gray-700">
              {summary.status === "ready" && "Your essential sender setup is complete. Keep an eye on delivery activity as you send."}
              {summary.status === "attention" && "Your messages can send, but a few improvements will make the experience more reliable."}
              {summary.status === "action_needed" && "Complete the highlighted steps before relying on this workspace for live sending."}
            </p>
          </div>
        </div>
        <div className="dashboard-health-count" aria-label={`${summary.alerts.length} actions to review`}>
          <strong>{summary.alerts.length}</strong>
          <span>{summary.alerts.length === 1 ? "action to review" : "actions to review"}</span>
        </div>
      </section>

      {error && <div className="dashboard-error-card" role="alert">{error}</div>}

      <section aria-labelledby="next-actions-title">
        <div className="mb-4 flex items-end justify-between gap-4">
          <div>
            <p className="dashboard-eyebrow">Keep things moving</p>
            <h2 id="next-actions-title" className="mt-1 text-xl font-semibold text-gray-950">Next actions</h2>
          </div>
          {hasAlerts && <span className="text-sm text-gray-500">{summary.alerts.length} to review</span>}
        </div>
        {hasAlerts ? (
          <div className="grid gap-3">
            {summary.alerts.map((alert) => {
              const classes = alertClasses(alert.severity);
              return (
                <article key={`${alert.code}-${alert.title}`} className={`dashboard-alert ${classes.card}`}>
                  <span aria-hidden="true" className={`dashboard-alert-icon ${classes.icon}`}>{alert.severity === "critical" ? "!" : "i"}</span>
                  <div className="min-w-0 flex-1">
                    <h3 className="font-semibold text-gray-950">{alert.title}</h3>
                    <p className="mt-1 max-w-3xl text-sm leading-6 text-gray-700">{alert.description}</p>
                  </div>
                  <Link href={alert.action_href} className={`dashboard-action-link ${classes.action}`}>{alert.action_label}<span aria-hidden="true"> →</span></Link>
                </article>
              );
            })}
          </div>
        ) : (
          <div className="dashboard-clear-state">
            <span aria-hidden="true" className="dashboard-clear-icon">✓</span>
            <div>
              <h3 className="font-semibold text-gray-950">Nothing needs your attention</h3>
              <p className="mt-1 text-sm leading-6 text-gray-600">Your sender setup and connections look good.</p>
            </div>
          </div>
        )}
      </section>

      <section className="grid gap-4 lg:grid-cols-4" aria-label="Workspace health">
        <article className="dashboard-stat-card">
          <div className="dashboard-card-heading"><span className="dashboard-card-icon dashboard-card-icon-blue" aria-hidden="true">↗</span><h2>Delivery health</h2></div>
          <p className="dashboard-stat-number">{formatNumber(summary.delivery.delivered)}</p>
          <p className="text-sm font-medium text-gray-800">{deliverySentence}</p>
          <p className="mt-2 text-sm leading-6 text-gray-500">Last {summary.delivery.period_days} days</p>
          {summary.delivery.failed > 0 && <p className="mt-3 text-sm text-red-700">{formatNumber(summary.delivery.failed)} need review</p>}
          {summary.delivery.uncertain > 0 && <p className="mt-2 text-sm text-amber-800">{formatNumber(summary.delivery.uncertain)} need delivery confirmation</p>}
          <Link href="/emails" className="dashboard-card-link">Review messages <span aria-hidden="true">→</span></Link>
        </article>

        <article className="dashboard-stat-card">
          <div className="dashboard-card-heading"><span className="dashboard-card-icon dashboard-card-icon-green" aria-hidden="true">♡</span><h2>Audience protection</h2></div>
          <p className="dashboard-stat-number">{formatNumber(summary.audience.suppressed)}</p>
          <p className="text-sm font-medium text-gray-800">recipients protected</p>
          <p className="mt-2 text-sm leading-6 text-gray-500">They will not receive messages until you review their status.</p>
          <p className="mt-3 text-xs text-gray-500">{formatNumber(summary.audience.unsubscribed)} unsubscribed · {formatNumber(summary.audience.bounced)} bounced · {formatNumber(summary.audience.complained)} complaints</p>
          <Link href="/contacts" className="dashboard-card-link">Review contacts <span aria-hidden="true">→</span></Link>
        </article>

        <article className="dashboard-stat-card">
          <div className="dashboard-card-heading"><span className="dashboard-card-icon dashboard-card-icon-purple" aria-hidden="true">⌁</span><h2>App connections</h2></div>
          <p className="dashboard-stat-number">{formatNumber(summary.webhooks.configured)}</p>
          <p className="text-sm font-medium text-gray-800">active connections</p>
          <p className="mt-2 text-sm leading-6 text-gray-500">Connections keep your app up to date about message activity.</p>
          {summary.webhooks.failed > 0 && <p className="mt-3 text-sm text-red-700">{formatNumber(summary.webhooks.failed)} updates need review</p>}
          <Link href="/webhooks" className="dashboard-card-link">Manage connections <span aria-hidden="true">→</span></Link>
        </article>

        <article className="dashboard-stat-card">
          <div className="dashboard-card-heading"><span className="dashboard-card-icon dashboard-card-icon-amber" aria-hidden="true">◎</span><h2>Delivery updates</h2></div>
          <p className="dashboard-stat-number text-2xl">{summary.delivery_tracking.configured ? "On" : "Off"}</p>
          <p className="text-sm font-medium text-gray-800">{summary.delivery_tracking.configured ? "Full status updates" : "Accepted status only"}</p>
          <p className="mt-2 text-sm leading-6 text-gray-500">Know when messages arrive, bounce, or need attention.</p>
          {!summary.delivery_tracking.configured && <Link href="/docs" className="dashboard-card-link">Read setup guide <span aria-hidden="true">→</span></Link>}
        </article>
      </section>

      <div className="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
        <section className="dashboard-panel" aria-labelledby="domains-title">
          <div className="dashboard-panel-heading">
            <div><p className="dashboard-eyebrow">Sender setup</p><h2 id="domains-title" className="mt-1 text-xl font-semibold text-gray-950">Your domains</h2></div>
            <Link href="/domains" className="dashboard-card-link dashboard-card-link-compact">Manage domains <span aria-hidden="true">→</span></Link>
          </div>
          {summary.domains.length === 0 ? (
            <div className="dashboard-panel-empty"><p className="font-medium text-gray-950">No sender domain yet</p><p className="mt-1 text-sm leading-6 text-gray-600">Add the domain you want customers to see in the From address.</p><Link href="/domains" className="dashboard-button dashboard-button-primary mt-4">Add a domain</Link></div>
          ) : (
            <div className="mt-5 space-y-3">
              {summary.domains.map((item) => (
                <div key={item.name} className="dashboard-domain-row">
                  <span aria-hidden="true" className={item.ready ? "dashboard-domain-icon dashboard-domain-icon-ready" : "dashboard-domain-icon dashboard-domain-icon-warning"}>{item.ready ? "✓" : "!"}</span>
                  <div className="min-w-0 flex-1"><p className="truncate font-medium text-gray-950">{item.name}</p><p className="mt-1 text-sm text-gray-500">{item.next_action}</p></div>
                  <Link href="/domains" className="dashboard-action-link text-gray-700 hover:text-gray-950">{item.ready ? "View" : "Fix"}<span aria-hidden="true"> →</span></Link>
                </div>
              ))}
            </div>
          )}
        </section>

        <section className="dashboard-panel" aria-labelledby="activity-title">
          <div className="dashboard-panel-heading"><div><p className="dashboard-eyebrow">What happened recently</p><h2 id="activity-title" className="mt-1 text-xl font-semibold text-gray-950">Recent activity</h2></div><Link href="/emails" className="dashboard-card-link dashboard-card-link-compact">View messages <span aria-hidden="true">→</span></Link></div>
          {summary.activity.length === 0 ? (
            <div className="dashboard-panel-empty"><p className="font-medium text-gray-950">No activity yet</p><p className="mt-1 text-sm leading-6 text-gray-600">Send a message and its progress will appear here.</p><Link href="/emails" className="dashboard-button dashboard-button-primary mt-4">Send an email</Link></div>
          ) : (
            <ol className="mt-5 space-y-4">
              {summary.activity.map((item) => (
                <li key={item.id} className="flex items-start gap-3"><span aria-hidden="true" className={activityTone(item.event)} /><div className="min-w-0 flex-1"><Link href={`/emails/${item.email_id}`} className="block truncate text-sm font-medium text-gray-900 hover:text-blue-700">{activityLabel(item.event)}</Link><p className="mt-0.5 truncate text-sm text-gray-500">{item.subject || "Untitled message"}</p></div><time className="shrink-0 text-xs text-gray-500" dateTime={item.timestamp}>{new Date(item.timestamp).toLocaleDateString()}</time></li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </div>
  );
}
