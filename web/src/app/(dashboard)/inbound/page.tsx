"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Pagination } from "@/components/pagination";

interface InboundEmail {
  id: string;
  from_addr: string;
  to_addr: string[];
  subject: string;
  text: string;
  created_at: string;
}

export default function InboundPage() {
  const [emails, setEmails] = useState<InboundEmail[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

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
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {emails.map((email) => (
              <tr key={email.id}>
                <td className="px-6 py-4 text-sm text-gray-900">{email.from_addr}</td>
                <td className="px-6 py-4 text-sm text-gray-900">{email.to_addr?.join(", ")}</td>
                <td className="px-6 py-4 text-sm text-gray-900">{email.subject}</td>
                <td className="px-6 py-4 text-sm text-gray-500">{new Date(email.created_at).toLocaleDateString()}</td>
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
