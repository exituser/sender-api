"use client";

import { Fragment, useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface Webhook {
  id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}

interface WebhookDelivery {
  id: string;
  event: string;
  status: string;
  attempts: number;
  last_error?: string;
  created_at: string;
  delivered_at?: string;
}

export default function WebhooksPage() {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ url: "", events: "email.sent,email.failed" });
  const [editing, setEditing] = useState<Webhook | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [historyId, setHistoryId] = useState<string | null>(null);
  const [deliveries, setDeliveries] = useState<Record<string, WebhookDelivery[]>>({});
  const [testingId, setTestingId] = useState<string | null>(null);

  const loadWebhooks = useCallback(async () => {
    setError("");
    try {
      const data = await api.webhooks.list() as { data: Webhook[] };
      setWebhooks(data.data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load webhooks");
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
    setSaving(true);
    try {
      const payload = {
        url: form.url,
        events: form.events.split(",").map((value) => value.trim()).filter(Boolean),
      };
      if (editing) await api.webhooks.update(editing.id, payload);
      else await api.webhooks.create(payload);
      setForm({ url: "", events: "email.sent,email.failed" });
      setEditing(null);
      setShowForm(false);
      await loadWebhooks();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add webhook");
    } finally {
      setSaving(false);
    }
  };

  const editWebhook = (webhook: Webhook) => {
    setEditing(webhook);
    setForm({ url: webhook.url, events: webhook.events.join(",") });
    setShowForm(true);
  };

  const toggleWebhook = async (webhook: Webhook) => {
    setError(""); setSaving(true);
    try { await api.webhooks.update(webhook.id, { active: !webhook.active }); await loadWebhooks(); }
    catch (err) { setError(err instanceof Error ? err.message : "Failed to update webhook"); }
    finally { setSaving(false); }
  };

  const deleteWebhook = async (id: string) => {
    try {
      await api.webhooks.delete(id);
      await loadWebhooks();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete webhook");
    }
  };

  const testWebhook = async (id: string) => {
    setError("");
    setTestingId(id);
    try {
      await api.webhooks.test(id);
      await loadDeliveries(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to queue webhook test");
    } finally {
      setTestingId(null);
    }
  };

  const loadDeliveries = async (id: string) => {
    setError("");
    try {
      const data = await api.webhooks.deliveries(id) as { data: WebhookDelivery[] };
      setDeliveries((current) => ({ ...current, [id]: data.data || [] }));
      setHistoryId(id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load webhook deliveries");
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Webhooks</h1>
        <button type="button" onClick={() => { setEditing(null); setForm({ url: "", events: "email.sent,email.failed" }); setShowForm((visible) => !visible); }} aria-expanded={showForm} aria-controls="webhook-form" className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Add Webhook
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
      {showForm && (
        <form id="webhook-form" onSubmit={addWebhook} className="bg-white shadow rounded-lg p-6 space-y-3">
          <label htmlFor="webhook-url" className="sr-only">Webhook URL</label>
          <input id="webhook-url" name="url" autoComplete="url" required type="url" placeholder="https://example.com/webhook" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <label htmlFor="webhook-events" className="sr-only">Webhook events</label>
          <input id="webhook-events" name="events" required placeholder="Events, comma-separated" value={form.events} onChange={(e) => setForm({ ...form, events: e.target.value })} className="w-full px-3 py-2 border rounded-md" />
          <button type="submit" disabled={saving} className="px-4 py-2 bg-green-600 text-white rounded-md disabled:opacity-50">{editing ? "Update webhook" : "Save webhook"}</button>
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
              <Fragment key={webhook.id}>
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
                  <button type="button" onClick={() => editWebhook(webhook)} className="text-blue-600 hover:underline">Edit</button>
                  <button type="button" onClick={() => toggleWebhook(webhook)} disabled={saving} className="ml-3 text-gray-700 hover:underline disabled:opacity-50">{webhook.active ? "Disable" : "Enable"}</button>
                  <button type="button" onClick={() => void testWebhook(webhook.id)} disabled={testingId === webhook.id || !webhook.active} className="ml-3 text-indigo-600 hover:underline disabled:opacity-50">{testingId === webhook.id ? "Testing..." : "Test"}</button>
                  <button type="button" onClick={() => historyId === webhook.id ? setHistoryId(null) : void loadDeliveries(webhook.id)} className="ml-3 text-gray-700 hover:underline">{historyId === webhook.id ? "Hide history" : "History"}</button>
                  <button type="button" onClick={() => deleteWebhook(webhook.id)} className="ml-3 text-red-600 hover:underline">Delete</button>
                </td>
              </tr>
              {historyId === webhook.id && (
                <tr key={`${webhook.id}-history`}>
                  <td colSpan={4} className="px-6 py-4 bg-gray-50">
                    <div className="text-sm font-medium text-gray-700 mb-2">Recent deliveries</div>
                    {(deliveries[webhook.id] || []).length === 0 ? (
                      <div className="text-sm text-gray-500">No delivery attempts yet.</div>
                    ) : (
                      <div className="space-y-2">
                        {(deliveries[webhook.id] || []).map((delivery) => (
                          <div key={delivery.id} className="flex flex-wrap items-center gap-3 text-xs text-gray-600">
                            <span className="font-medium text-gray-800">{delivery.event}</span>
                            <span>{delivery.status}</span>
                            <span>{delivery.attempts} attempt{delivery.attempts === 1 ? "" : "s"}</span>
                            <span>{new Date(delivery.created_at).toLocaleString()}</span>
                            {delivery.last_error && <span className="text-red-600">{delivery.last_error}</span>}
                          </div>
                        ))}
                      </div>
                    )}
                  </td>
                </tr>
              )}
              </Fragment>
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
