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
}

interface EmailEvent {
  id: string;
  event: string;
  timestamp: string;
  data: Record<string, unknown>;
}

export default function EmailDetailPage() {
  const params = useParams();
  const router = useRouter();
  const [email, setEmail] = useState<Email | null>(null);
  const [events, setEvents] = useState<EmailEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const emailID = params.id as string;

  const loadEmail = useCallback(async () => {
    try {
      const data = await api.emails.get(emailID) as Email;
      setEmail(data);
      const evts = await api.emails.events(emailID) as EmailEvent[];
      setEvents(evts || []);
    } catch (err) {
      console.error(err);
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
      case "failed": return "bg-red-100 text-red-800";
      case "queued": return "bg-yellow-100 text-yellow-800";
      case "delivered": return "bg-blue-100 text-blue-800";
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

  if (loading) return <div className="text-center py-8">Loading...</div>;
  if (!email) return <div className="text-center py-8">Email not found</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <button onClick={() => router.back()} className="text-gray-500 hover:text-gray-700">
          &larr; Back
        </button>
        <h1 className="text-2xl font-bold">Email Details</h1>
        <span className={`px-2 py-1 text-xs font-semibold rounded-full ${getStatusColor(email.status)}`}>
          {email.status}
        </span>
        {email.status === "queued" && email.scheduled_at && <button type="button" onClick={cancelScheduledEmail} disabled={cancelling} className="ml-auto px-3 py-2 text-sm text-red-700 border border-red-300 rounded-md hover:bg-red-50 disabled:opacity-50">{cancelling ? "Cancelling..." : "Cancel scheduled email"}</button>}
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}

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
        {email.tags?.length > 0 && <div><span className="text-sm font-medium text-gray-500">Tags</span><p className="text-gray-900">{email.tags.map((tag) => `${tag.name}=${tag.value}`).join(", ")}</p></div>}
        {Object.keys(email.metadata ?? {}).length > 0 && <div><span className="text-sm font-medium text-gray-500">Metadata</span><pre className="mt-1 text-sm whitespace-pre-wrap">{JSON.stringify(email.metadata, null, 2)}</pre></div>}
        {Object.keys(email.headers ?? {}).length > 0 && <div><span className="text-sm font-medium text-gray-500">Headers</span><pre className="mt-1 text-sm whitespace-pre-wrap">{JSON.stringify(email.headers, null, 2)}</pre></div>}
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
        <h2 className="text-lg font-medium mb-4">Events</h2>
        {events.length === 0 ? (
          <p className="text-gray-500">No events</p>
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
                  <td className="px-4 py-2 text-sm">{evt.event}</td>
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
