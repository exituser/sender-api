"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";

interface Email {
  id: string;
  from: string;
  to: string[];
  subject: string;
  status: string;
  created_at: string;
}

export default function EmailsPage() {
  const [emails, setEmails] = useState<Email[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [sending, setSending] = useState(false);
  const [form, setForm] = useState({ from: "", to: "", subject: "", text: "" });

  const loadEmails = useCallback(async () => {
    try {
      const data = await api.emails.list() as { data: Email[] };
      setEmails(data.data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load emails");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadEmails);
  }, [loadEmails]);

  const getStatusColor = (status: string) => {
    switch (status) {
      case "sent":
        return "bg-green-100 text-green-800";
      case "failed":
        return "bg-red-100 text-red-800";
      case "queued":
        return "bg-yellow-100 text-yellow-800";
      default:
        return "bg-gray-100 text-gray-800";
    }
  };

  const sendEmail = async (event: React.FormEvent) => {
    event.preventDefault();
    setSending(true);
    setError("");
    try {
      await api.emails.send({
        from: form.from,
        to: form.to.split(",").map((address) => address.trim()).filter(Boolean),
        subject: form.subject,
        text: form.text,
      });
      setForm({ from: "", to: "", subject: "", text: "" });
      setShowForm(false);
      await loadEmails();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to send email");
    } finally {
      setSending(false);
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Emails</h1>
        <button onClick={() => setShowForm((visible) => !visible)} className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Send Email
        </button>
      </div>

      {showForm && (
        <form onSubmit={sendEmail} className="bg-white shadow rounded-lg p-6 space-y-4">
          <input required type="email" placeholder="From" value={form.from} onChange={(e) => setForm({ ...form, from: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <input required placeholder="To (comma-separated)" value={form.to} onChange={(e) => setForm({ ...form, to: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <input required placeholder="Subject" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <textarea required placeholder="Message" value={form.text} onChange={(e) => setForm({ ...form, text: e.target.value })} className="w-full px-3 py-2 border rounded-md min-h-32" />
          <button disabled={sending} type="submit" className="px-4 py-2 bg-green-600 text-white rounded-md disabled:opacity-50">{sending ? "Sending..." : "Queue email"}</button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm">
          {error}
        </div>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                To
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Subject
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Date
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {emails.map((email) => (
              <tr key={email.id}>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                  <Link href={`/emails/${email.id}`} className="hover:underline">{email.to.join(", ")}</Link>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                  {email.subject}
                </td>
                <td className="px-6 py-4 whitespace-nowrap">
                  <span
                    className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${getStatusColor(
                      email.status
                    )}`}
                  >
                    {email.status}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                  {new Date(email.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {emails.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            No emails found
          </div>
        )}
      </div>
    </div>
  );
}
