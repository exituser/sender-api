"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Pagination } from "@/components/pagination";

interface InboundEmail {
  id: string;
  from: string;
  to: string[];
  subject: string;
  text?: string;
  html?: string;
  message_id?: string;
  raw_s3_key?: string;
  headers?: Record<string, string>;
  attachments?: unknown[];
  created_at: string;
}

export default function InboundPage() {
  const [emails, setEmails] = useState<InboundEmail[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [details, setDetails] = useState<Record<string, InboundEmail>>({});
  const [detailLoading, setDetailLoading] = useState<string | null>(null);

  const loadEmails = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await api.inbound.list(50, page * 50) as { data: InboundEmail[]; total: number };
      setEmails(data.data || []);
      setTotal(data.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : "We couldn’t load received messages. Try again.");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void Promise.resolve().then(loadEmails);
  }, [loadEmails]);

  const loadDetail = async (id: string) => {
    if (details[id]) return;
    setDetailLoading(id);
    try {
      const detail = await api.inbound.get(id) as InboundEmail;
      setDetails((current) => ({ ...current, [id]: detail }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "We couldn’t open this message. Try again.");
    } finally {
      setDetailLoading(null);
    }
  };

  if (loading) return <div className="py-8 text-center text-sm text-gray-600" aria-busy="true">Loading received messages…</div>;

  return (
    <div className="space-y-6">
      <div>
        <p className="dashboard-eyebrow">Messages sent to you</p>
        <h1 className="mt-1 text-3xl font-semibold tracking-tight text-gray-950">Received messages</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-gray-600">See messages that arrived at your connected addresses.</p>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Sender</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Recipient</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Subject</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Received</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Message</th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {emails.map((email) => (
              <tr key={email.id}>
                <td className="px-6 py-4 text-sm text-gray-900">{email.from}</td>
                <td className="px-6 py-4 text-sm text-gray-900">{email.to?.join(", ")}</td>
                <td className="px-6 py-4 text-sm text-gray-900">{email.subject}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{new Date(email.created_at).toLocaleString()}</td>
                <td className="px-6 py-4 text-sm"><details aria-label={`Open message from ${email.from}`} onToggle={(event) => { if (event.currentTarget.open) void loadDetail(email.id); }}><summary className="cursor-pointer text-blue-600">{detailLoading === email.id ? "Loading…" : "Open message"}</summary>{(() => { const detail = details[email.id] || email; return <div className="mt-3 max-w-xl space-y-3"><p className="text-sm text-gray-600">Attachments: {detail.attachments?.length ?? 0}</p>{detail.text && <div className="max-h-64 overflow-auto whitespace-pre-wrap rounded-xl bg-gray-50 p-3 text-sm text-gray-800">{detail.text}</div>}{detail.html && <iframe aria-label={`Message from ${email.from}`} title={`Message from ${email.from}`} sandbox="" srcDoc={detail.html} className="h-64 w-full rounded border" />}</div>; })()}</details></td>
              </tr>
            ))}
          </tbody>
        </table>
        {emails.length === 0 && (
          <div className="px-6 py-8 text-center text-gray-500">No received messages yet.</div>
        )}
        <Pagination page={page} pageSize={50} total={total} onPageChange={setPage} disabled={loading} />
      </div>
    </div>
  );
}
