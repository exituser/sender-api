"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { api } from "@/lib/api";

interface Email {
  id: string;
  from: string;
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  html: string;
  text: string;
  status: string;
  tags: { name: string; value: string }[];
  metadata: Record<string, string>;
  headers: Record<string, string>;
  reply_to: string[];
  attachments: { filename: string }[];
  scheduled_at: string | null;
  created_at: string;
  sent_at: string | null;
  delivery_review_started_at?: string | null;
  provider_evidence?: boolean;
  can_reconcile?: boolean;
}

interface EmailEvent {
  id: string;
  event: string;
  timestamp: string;
  data: Record<string, unknown>;
}

function statusLabel(status: string) {
  switch (status) {
    case "queued": return "Waiting to send";
    case "sending": return "Sending";
    case "sent": return "Accepted";
    case "delivered": return "Delivered";
    case "opened": return "Opened";
    case "clicked": return "Link clicked";
    case "bounced": return "Could not deliver";
    case "complained": return "Recipient reported";
    case "failed": return "Needs attention";
    case "ambiguous": return "Delivery needs review";
    case "cancelled": return "Cancelled";
    default: return "Processing";
  }
}

function eventLabel(event: string) {
  switch (event) {
    case "email.sent": return "Message accepted";
    case "email.delivered": return "Message delivered";
    case "email.bounced": return "Message could not be delivered";
    case "email.complained": return "Recipient reported a message";
    case "email.failed": return "Message could not be sent";
    case "email.ambiguous": return "Delivery confirmation needs review";
    case "email.retrying": return "Message will be retried";
    case "email.opened": return "Message opened";
    case "email.clicked": return "Link clicked";
    default: return "Message updated";
  }
}

export default function EmailDetailPage() {
  const params = useParams();
  const router = useRouter();
  const [email, setEmail] = useState<Email | null>(null);
  const [events, setEvents] = useState<EmailEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const [reconciling, setReconciling] = useState(false);
  const emailID = params.id as string;

  const loadEmail = useCallback(async () => {
    try {
      const data = await api.emails.get(emailID) as Email;
      setEmail(data);
      const evts = await api.emails.events(emailID) as EmailEvent[];
      setEvents(evts || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load this message. Try again.");
    } finally {
      setLoading(false);
    }
  }, [emailID]);

  useEffect(() => {
    void Promise.resolve().then(loadEmail);
  }, [loadEmail]);

  const getStatusColor = (status: string) => {
    switch (status) {
      case "sent": return "bg-green-100 text-green-800";
      case "delivered":
      case "opened":
      case "clicked": return "bg-green-100 text-green-800";
      case "failed": return "bg-red-100 text-red-800";
      case "ambiguous": return "bg-amber-100 text-amber-900";
      case "bounced":
      case "complained": return "bg-orange-100 text-orange-800";
      case "queued":
      case "sending": return "bg-yellow-100 text-yellow-800";
      default: return "bg-gray-100 text-gray-800";
    }
  };

  const cancelScheduledEmail = async () => {
    if (!email || !window.confirm("Cancel this scheduled email?")) return;
    setCancelling(true);
    setError("");
    try {
      await api.emails.cancel(email.id);
      await loadEmail();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to cancel scheduled email");
    } finally {
      setCancelling(false);
    }
  };

  const reconcileDelivery = async (action: "accepted" | "failed") => {
    const prompt = action === "accepted"
      ? "Confirm that the delivery service accepted this message?"
      : "Mark this message as not sent? It will stay closed and will not be retried.";
    if (!window.confirm(prompt)) return;
    setReconciling(true);
    setError("");
    try {
      await api.emails.reconcile(emailID, action);
      await loadEmail();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to save the delivery review. Try again.");
    } finally {
      setReconciling(false);
    }
  };

  if (loading) return <div className="text-center py-8">Loading...</div>;
  if (!email) return <div className="py-8 text-center text-sm text-gray-600">Unable to find this message.</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
          <button type="button" onClick={() => router.back()} className="text-gray-500 hover:text-gray-700">
          &larr; Back to messages
        </button>
        <h1 className="text-2xl font-bold">Message details</h1>
        <span className={`px-2 py-1 text-xs font-semibold rounded-full ${getStatusColor(email.status)}`}>
          {statusLabel(email.status)}
        </span>
        {email.status === "queued" && email.scheduled_at && <button type="button" onClick={cancelScheduledEmail} disabled={cancelling} className="ml-auto px-3 py-2 text-sm text-red-700 border border-red-300 rounded-md hover:bg-red-50 disabled:opacity-50">{cancelling ? "Cancelling…" : "Cancel scheduled message"}</button>}
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}

      {email.status === "ambiguous" && (
        <section className="rounded-xl border border-amber-200 bg-amber-50 p-5" aria-labelledby="delivery-review-title">
          <h2 id="delivery-review-title" className="font-semibold text-amber-950">We could not confirm the final handoff</h2>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-amber-900">
            The delivery service may have accepted this message, but its confirmation did not reach us. We will not send it again automatically, so your recipient will not receive a duplicate from a retry.
          </p>
          {email.delivery_review_started_at && <p className="mt-2 text-xs text-amber-800">Waiting for review since {new Date(email.delivery_review_started_at).toLocaleString()}.</p>}
          <p className="mt-3 text-sm leading-6 text-amber-900">
            {email.provider_evidence
              ? "A trusted delivery update is available, so an owner or admin can safely confirm that it was accepted."
              : "Check your delivery service activity first. If the message is not there, close it as not sent."}
          </p>
          {email.can_reconcile ? (
            <div className="mt-4 flex flex-wrap gap-3">
              {email.provider_evidence && <button type="button" disabled={reconciling} onClick={() => void reconcileDelivery("accepted")} className="rounded-md bg-amber-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">Confirm accepted</button>}
              <button type="button" disabled={reconciling} onClick={() => void reconcileDelivery("failed")} className="rounded-md border border-amber-400 bg-white px-4 py-2 text-sm font-medium text-amber-950 disabled:opacity-50">Mark as not sent</button>
            </div>
          ) : (
            <p className="mt-4 text-sm font-medium text-amber-950">Ask a workspace owner or admin to complete this review.</p>
          )}
        </section>
      )}

      <div className="bg-white shadow rounded-lg p-6 space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <span className="text-sm font-medium text-gray-500">From</span>
            <p className="text-gray-900">{email.from}</p>
          </div>
          <div>
            <span className="text-sm font-medium text-gray-500">To</span>
            <p className="text-gray-900">{email.to.join(", ")}</p>
          </div>
          <div>
            <span className="text-sm font-medium text-gray-500">Subject</span>
            <p className="text-gray-900">{email.subject}</p>
          </div>
          <div>
            <span className="text-sm font-medium text-gray-500">Created</span>
            <p className="text-gray-900">{new Date(email.created_at).toLocaleString()}</p>
          </div>
          {email.scheduled_at && <div><span className="text-sm font-medium text-gray-500">Scheduled</span><p className="text-gray-900">{new Date(email.scheduled_at).toLocaleString()}</p></div>}
        </div>

        {email.cc?.length > 0 && (
          <div>
            <span className="text-sm font-medium text-gray-500">CC</span>
            <p className="text-gray-900">{email.cc.join(", ")}</p>
          </div>
        )}
        {email.bcc?.length > 0 && <div><span className="text-sm font-medium text-gray-500">BCC</span><p className="text-gray-900">{email.bcc.join(", ")}</p></div>}
        {email.reply_to?.length > 0 && <div><span className="text-sm font-medium text-gray-500">Reply-To</span><p className="text-gray-900">{email.reply_to.join(", ")}</p></div>}
        {email.tags?.length > 0 && <div><span className="text-sm font-medium text-gray-500">Labels</span><p className="text-gray-900">{email.tags.map((tag) => `${tag.name}=${tag.value}`).join(", ")}</p></div>}
        {(Object.keys(email.metadata ?? {}).length > 0 || Object.keys(email.headers ?? {}).length > 0) && <details className="rounded-xl border border-gray-200 bg-gray-50 p-4"><summary className="cursor-pointer text-sm font-semibold text-gray-800">Advanced message details</summary>{Object.keys(email.metadata ?? {}).length > 0 && <div className="mt-4"><span className="text-sm font-medium text-gray-500">Internal metadata</span><pre className="mt-1 overflow-x-auto whitespace-pre-wrap text-sm">{JSON.stringify(email.metadata, null, 2)}</pre></div>}{Object.keys(email.headers ?? {}).length > 0 && <div className="mt-4"><span className="text-sm font-medium text-gray-500">Extra message headers</span><pre className="mt-1 overflow-x-auto whitespace-pre-wrap text-sm">{JSON.stringify(email.headers, null, 2)}</pre></div>}</details>}
        {email.attachments?.length > 0 && <div><span className="text-sm font-medium text-gray-500">Attachments</span><p className="text-gray-900">{email.attachments.map((attachment) => attachment.filename).join(", ")}</p></div>}

        {email.html && (
          <div>
            <span className="text-sm font-medium text-gray-500">HTML Preview</span>
            <iframe
              title="Email HTML preview"
              sandbox=""
              srcDoc={email.html}
              className="mt-2 min-h-96 w-full rounded border bg-gray-50"
            />
          </div>
        )}

        {email.text && (
          <div>
            <span className="text-sm font-medium text-gray-500">Text</span>
            <pre className="mt-2 border rounded p-4 bg-gray-50 text-sm whitespace-pre-wrap">{email.text}</pre>
          </div>
        )}
      </div>

      <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-lg font-medium mb-4">Message timeline</h2>
        {events.length === 0 ? (
          <p className="text-gray-500">No updates yet</p>
        ) : (
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500">Event</th>
                <th className="px-4 py-2 text-left text-xs font-medium text-gray-500">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {events.map((evt) => (
                <tr key={evt.id}>
                  <td className="px-4 py-2 text-sm">{eventLabel(evt.event)}</td>
                  <td className="px-4 py-2 text-sm text-gray-500">{new Date(evt.timestamp).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
