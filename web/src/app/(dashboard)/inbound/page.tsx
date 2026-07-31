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
      setError(err instanceof Error ? err.message : "Failed to load inbound emails");
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
      setError(err instanceof Error ? err.message : "Failed to load inbound email");
    } finally {
      setDetailLoading(null);
    }
  };

  if (loading) return <div className="text-center py-8">Loading...</div>;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Inbound Emails</h1>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">From</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">To</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Subject</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Date</th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Details</th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {emails.map((email) => (
              <tr key={email.id}>
                <td className="px-6 py-4 text-sm text-gray-900">{email.from}</td>
                <td className="px-6 py-4 text-sm text-gray-900">{email.to?.join(", ")}</td>
                <td className="px-6 py-4 text-sm text-gray-900">{email.subject}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{new Date(email.created_at).toLocaleDateString()}</td>
                <td className="px-6 py-4 text-sm"><details aria-label={`Details for inbound email ${email.id}`} onToggle={(event) => { if (event.currentTarget.open) void loadDetail(email.id); }}><summary className="cursor-pointer text-blue-600">{detailLoading === email.id ? "Loading..." : "View"}</summary>{(() => { const detail = details[email.id] || email; return <div className="mt-2 max-w-xl space-y-3"><div><span className="font-medium">Message ID:</span> {detail.message_id || "—"}</div><div><span className="font-medium">Raw object:</span> {detail.raw_s3_key || "Unavailable"}</div><div><span className="font-medium">Attachments:</span> {detail.attachments?.length ?? 0}</div>{detail.text && <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-2">{detail.text}</pre>}{detail.html && <iframe aria-label={`HTML email ${email.id}`} title={`HTML email ${email.id}`} sandbox="" srcDoc={detail.html} className="h-48 w-full rounded border" />}{detail.headers && <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-xs">{JSON.stringify(detail.headers, null, 2)}</pre>}</div>; })()}</details></td>
              </tr>
            ))}
          </tbody>
        </table>
        {emails.length === 0 && (
          <div className="text-center py-8 text-gray-500">No inbound emails</div>
        )}
        <Pagination page={page} pageSize={50} total={total} onPageChange={setPage} disabled={loading} />
      </div>
    </div>
  );
}
