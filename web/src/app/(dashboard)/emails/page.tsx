"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { Pagination } from "@/components/pagination";

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
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [sending, setSending] = useState(false);
  const [form, setForm] = useState({ from: "", to: "", cc: "", bcc: "", replyTo: "", subject: "", text: "", html: "", headers: "", metadata: "", tags: "", scheduledAt: "" });
  const [attachments, setAttachments] = useState<File[]>([]);

  const splitAddresses = (value: string) => value.split(",").map((address) => address.trim()).filter(Boolean);

  const parseKeyValues = (value: string, itemName: string) => value.split("\n").filter(Boolean).map((line) => {
    const [key, ...parts] = line.split("=");
    if (!key?.trim() || parts.length === 0) throw new Error(`${itemName} must use key=value, one per line`);
    return [key.trim(), parts.join("=").trim()] as const;
  });

  const encodeAttachment = async (file: File) => {
    if (file.size > 5 * 1024 * 1024) throw new Error(`${file.name} exceeds the 5 MB attachment limit`);
    const dataUrl = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onerror = () => reject(new Error(`Could not read ${file.name}`));
      reader.onload = () => resolve(String(reader.result));
      reader.readAsDataURL(file);
    });
    return { filename: file.name, content: dataUrl.slice(dataUrl.indexOf(",") + 1) };
  };

  const loadEmails = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await api.emails.list(50, page * 50) as { data: Email[]; total: number };
      setEmails(data.data || []);
      setTotal(data.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load emails");
    } finally {
      setLoading(false);
    }
  }, [page]);

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
      const metadata = Object.fromEntries(parseKeyValues(form.metadata, "Metadata"));
      const tags = parseKeyValues(form.tags, "Tags").map(([name, value]) => ({ name, value }));
      const headers = Object.fromEntries(parseKeyValues(form.headers, "Headers"));
      const encodedAttachments = await Promise.all(attachments.map(encodeAttachment));
      await api.emails.send({
        from: form.from,
        to: splitAddresses(form.to),
        cc: splitAddresses(form.cc),
        bcc: splitAddresses(form.bcc),
        reply_to: splitAddresses(form.replyTo),
        subject: form.subject,
        text: form.text,
        html: form.html,
        headers,
        metadata,
        tags,
        attachments: encodedAttachments,
        scheduled_at: form.scheduledAt ? new Date(form.scheduledAt).toISOString() : undefined,
      });
      setForm({ from: "", to: "", cc: "", bcc: "", replyTo: "", subject: "", text: "", html: "", headers: "", metadata: "", tags: "", scheduledAt: "" });
      setAttachments([]);
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
        <button
          type="button"
          onClick={() => setShowForm((visible) => !visible)}
          aria-expanded={showForm}
          aria-controls="send-email-form"
          className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
        >
          {showForm ? "Close email form" : "Send email"}
        </button>
      </div>

      {showForm && (
        <form id="send-email-form" onSubmit={sendEmail} className="bg-white shadow rounded-lg p-6 space-y-4">
          <div>
            <label htmlFor="email-from" className="block text-sm font-medium text-gray-700">From</label>
            <input id="email-from" name="from" autoComplete="email" spellCheck={false} required type="email" placeholder="sender@example.com" value={form.from} onChange={(e) => setForm({ ...form, from: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div><label htmlFor="email-cc" className="block text-sm font-medium text-gray-700">CC</label><input id="email-cc" type="text" placeholder="cc@example.com" value={form.cc} onChange={(e) => setForm({ ...form, cc: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /></div>
            <div><label htmlFor="email-bcc" className="block text-sm font-medium text-gray-700">BCC</label><input id="email-bcc" type="text" placeholder="bcc@example.com" value={form.bcc} onChange={(e) => setForm({ ...form, bcc: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /></div>
          </div>
          <div><label htmlFor="email-reply-to" className="block text-sm font-medium text-gray-700">Reply-To</label><input id="email-reply-to" type="text" placeholder="reply@example.com" value={form.replyTo} onChange={(e) => setForm({ ...form, replyTo: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /></div>
          <div>
            <label htmlFor="email-to" className="block text-sm font-medium text-gray-700">To</label>
            <input id="email-to" name="to" autoComplete="email" spellCheck={false} required type="email" multiple aria-describedby="email-to-hint" placeholder="recipient@example.com, another@example.com" value={form.to} onChange={(e) => setForm({ ...form, to: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" />
            <p id="email-to-hint" className="mt-1 text-sm text-gray-500">Separate multiple recipients with commas.</p>
          </div>
          <div><label htmlFor="email-html" className="block text-sm font-medium text-gray-700">HTML (optional)</label><textarea id="email-html" value={form.html} onChange={(e) => setForm({ ...form, html: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md min-h-32 font-mono text-sm" /></div>
          <div className="grid gap-4 md:grid-cols-2">
            <div><label htmlFor="email-headers" className="block text-sm font-medium text-gray-700">Custom headers</label><textarea id="email-headers" placeholder="X-Campaign=welcome" value={form.headers} onChange={(e) => setForm({ ...form, headers: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /><p className="mt-1 text-xs text-gray-500">One header=value pair per line.</p></div>
            <div><label htmlFor="email-metadata" className="block text-sm font-medium text-gray-700">Metadata</label><textarea id="email-metadata" placeholder="campaign=welcome" value={form.metadata} onChange={(e) => setForm({ ...form, metadata: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /><p className="mt-1 text-xs text-gray-500">One key=value pair per line.</p></div>
            <div><label htmlFor="email-tags" className="block text-sm font-medium text-gray-700">Tags</label><textarea id="email-tags" placeholder="category=transactional" value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /><p className="mt-1 text-xs text-gray-500">One name=value pair per line.</p></div>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div><label htmlFor="email-scheduled-at" className="block text-sm font-medium text-gray-700">Schedule for (optional)</label><input id="email-scheduled-at" type="datetime-local" value={form.scheduledAt} onChange={(e) => setForm({ ...form, scheduledAt: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" /></div>
            <div><label htmlFor="email-attachments" className="block text-sm font-medium text-gray-700">Attachments</label><input id="email-attachments" type="file" multiple onChange={(e) => setAttachments(Array.from(e.target.files ?? []))} className="mt-1 w-full px-3 py-2 border rounded-md" /><p className="mt-1 text-xs text-gray-500">Up to 5 MB per file; payload stays within the API’s 10 MB limit.</p></div>
          </div>
          <div>
            <label htmlFor="email-subject" className="block text-sm font-medium text-gray-700">Subject</label>
            <input id="email-subject" name="subject" required type="text" value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md" />
          </div>
          <div>
            <label htmlFor="email-message" className="block text-sm font-medium text-gray-700">Message</label>
            <textarea id="email-message" name="message" required value={form.text} onChange={(e) => setForm({ ...form, text: e.target.value })} className="mt-1 w-full px-3 py-2 border rounded-md min-h-32" />
          </div>
          <button disabled={sending} type="submit" className="px-4 py-2 bg-green-600 text-white rounded-md disabled:opacity-50" aria-busy={sending}>{sending ? "Sending..." : "Queue email"}</button>
        </form>
      )}

      {error && (
        <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">
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
        <Pagination page={page} pageSize={50} total={total} onPageChange={setPage} disabled={loading} />
      </div>
    </div>
  );
}
