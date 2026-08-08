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

const eventOptions = [
  { value: "email.sent", label: "Message accepted" },
  { value: "email.delivered", label: "Message delivered" },
  { value: "email.bounced", label: "Message could not be delivered" },
  { value: "email.complained", label: "Recipient reported a message" },
  { value: "email.failed", label: "Message could not be sent" },
];

const defaultEvents = ["email.sent", "email.failed"];

function eventLabel(event: string) {
  return eventOptions.find((option) => option.value === event)?.label || "Message update";
}

export default function WebhooksPage() {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ url: "", events: defaultEvents });
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
        events: form.events,
      };
      if (editing) await api.webhooks.update(editing.id, payload);
      else await api.webhooks.create(payload);
      setForm({ url: "", events: defaultEvents });
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
    setForm({ url: webhook.url, events: webhook.events });
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

  const replayDelivery = async (webhookId: string, deliveryId: string) => {
    setError("");
    try {
      await api.webhooks.replay(webhookId, deliveryId);
      await loadDeliveries(webhookId);
    } catch (err) {
      setError(err instanceof Error ? err.message : "We couldn’t retry this update. Try again.");
    }
  };

  if (loading) {
    return <div className="py-8 text-center text-sm text-gray-600" aria-busy="true">Loading your connections…</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div><p className="dashboard-eyebrow">Keep your app informed</p><h1 className="mt-1 text-3xl font-semibold tracking-tight text-gray-950">App connections</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-gray-600">Send message updates to your app so it always knows what happened.</p></div>
        <button type="button" onClick={() => { setEditing(null); setForm({ url: "", events: defaultEvents }); setShowForm((visible) => !visible); }} aria-expanded={showForm} aria-controls="webhook-form" className="dashboard-button dashboard-button-primary self-start sm:self-auto">
          {showForm ? "Close form" : "Add a connection"}
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
      {showForm && (
        <form id="webhook-form" onSubmit={addWebhook} className="space-y-5 rounded-2xl border border-gray-200 bg-white p-5 shadow-sm sm:p-6">
          <div><label htmlFor="webhook-url" className="block text-sm font-semibold text-gray-900">Your app’s update URL</label><p className="mt-1 text-sm leading-6 text-gray-600">We’ll send a secure update here whenever one of the selected events happens.</p><input id="webhook-url" name="url" autoComplete="url" required type="url" placeholder="https://example.com/updates" value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} className="mt-3 min-h-10 w-full rounded-xl border border-gray-300 px-3 text-base sm:text-sm" /></div>
          <fieldset><legend className="text-sm font-semibold text-gray-900">Updates to send</legend><p className="mt-1 text-sm leading-6 text-gray-600">Choose the moments your app needs to know about.</p><div className="mt-3 grid gap-3 sm:grid-cols-2">{eventOptions.map((option) => <label key={option.value} className="flex items-start gap-3 rounded-xl border border-gray-200 p-3 text-sm text-gray-800"><input type="checkbox" checked={form.events.includes(option.value)} onChange={(event) => setForm({ ...form, events: event.target.checked ? [...form.events, option.value] : form.events.filter((value) => value !== option.value) })} className="mt-0.5 size-4 accent-blue-700" />{option.label}</label>)}</div></fieldset>
          <button type="submit" disabled={saving || form.events.length === 0} className="dashboard-button dashboard-button-primary disabled:opacity-50">{editing ? "Save connection" : "Create connection"}</button>
        </form>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                App URL
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Updates
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
                  {webhook.events.map(eventLabel).join(", ")}
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
                  <button type="button" onClick={() => toggleWebhook(webhook)} disabled={saving} className="ml-3 text-gray-700 hover:underline disabled:opacity-50">{webhook.active ? "Pause" : "Resume"}</button>
                  <button type="button" onClick={() => void testWebhook(webhook.id)} disabled={testingId === webhook.id || !webhook.active} className="ml-3 text-indigo-600 hover:underline disabled:opacity-50">{testingId === webhook.id ? "Sending…" : "Send test"}</button>
                  <button type="button" onClick={() => historyId === webhook.id ? setHistoryId(null) : void loadDeliveries(webhook.id)} className="ml-3 text-gray-700 hover:underline">{historyId === webhook.id ? "Hide attempts" : "View attempts"}</button>
                  <button type="button" onClick={() => deleteWebhook(webhook.id)} className="ml-3 text-red-600 hover:underline">Remove</button>
                </td>
              </tr>
              {historyId === webhook.id && (
                <tr key={`${webhook.id}-history`}>
                  <td colSpan={4} className="px-6 py-4 bg-gray-50">
                    <div className="text-sm font-medium text-gray-700 mb-2">Recent update attempts</div>
                    {(deliveries[webhook.id] || []).length === 0 ? (
                      <div className="text-sm text-gray-500">No delivery attempts yet.</div>
                    ) : (
                      <div className="space-y-2">
                        {(deliveries[webhook.id] || []).map((delivery) => (
                          <div key={delivery.id} className="flex flex-wrap items-center gap-3 text-xs text-gray-600">
                            <span className="font-medium text-gray-800">{eventLabel(delivery.event)}</span>
                            <span>{delivery.status === "delivered" ? "Sent" : delivery.status === "failed" ? "Could not send" : "Waiting"}</span>
                            <span>{delivery.attempts} attempt{delivery.attempts === 1 ? "" : "s"}</span>
                            <span>{new Date(delivery.created_at).toLocaleString()}</span>
                            {delivery.last_error && <span className="text-red-600">Check the URL and try again.</span>}
                            {delivery.status === "failed" && <button type="button" onClick={() => void replayDelivery(webhook.id, delivery.id)} className="text-blue-700 hover:underline">Try again</button>}
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
            <p className="font-medium text-gray-900">No connections yet</p>
            <p className="mt-1 text-sm text-gray-500">Add one to let your app react to message updates.</p>
          </div>
        )}
      </div>
    </div>
  );
}
