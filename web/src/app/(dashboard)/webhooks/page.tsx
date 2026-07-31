"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface Webhook {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}

export default function WebhooksPage() {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ url: "", events: "email.sent,email.failed" });
  const [error, setError] = useState("");

  const loadWebhooks = useCallback(async () => {
    try {
      const data = await api.webhooks.list() as { data: Webhook[] };
      setWebhooks(data.data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadWebhooks);
  }, [loadWebhooks]);

  const addWebhook = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      await api.webhooks.create({
        url: form.url,
        events: form.events.split(",").map((value) => value.trim()).filter(Boolean),
      });
      setForm({ url: "", events: "email.sent,email.failed" });
      setShowForm(false);
      await loadWebhooks();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add webhook");
    }
  };

  const deleteWebhook = async (id: string) => {
    try {
      await api.webhooks.delete(id);
      await loadWebhooks();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete webhook");
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Webhooks</h1>
        <button onClick={() => setShowForm((visible) => !visible)} className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Add Webhook
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm">{error}</div>}
      {showForm && (
        <form onSubmit={addWebhook} className="bg-white shadow rounded-lg p-6 space-y-3">
          <input required type="url" placeholder="https://example.com/webhook" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <input required placeholder="Events, comma-separated" value={form.events} onChange={(e) => setForm({ ...form, events: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <button type="submit" className="px-4 py-2 bg-green-600 text-white rounded-md">Save webhook</button>
        </form>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                URL
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Events
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {webhooks.map((webhook) => (
              <tr key={webhook.id}>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                  {webhook.url}
                </td>
                <td className="px-6 py-4 text-sm text-gray-500">
                  {webhook.events.join(", ")}
                </td>
                <td className="px-6 py-4 whitespace-nowrap">
                  <span
                    className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                      webhook.active
                        ? "bg-green-100 text-green-800"
                        : "bg-red-100 text-red-800"
                    }`}
                  >
                    {webhook.active ? "Active" : "Inactive"}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm">
                  <button onClick={() => deleteWebhook(webhook.id)} className="text-red-600 hover:underline">Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {webhooks.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            No webhooks configured
          </div>
        )}
      </div>
    </div>
  );
}
