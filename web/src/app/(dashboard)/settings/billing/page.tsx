"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

type BillingSummary = {
  plan: "free" | "pro" | "scale";
  status: string;
  current_period_end?: string;
  cancel_at_period_end: boolean;
  has_customer: boolean;
  has_subscription: boolean;
};

export default function BillingPage() {
  const [summary, setSummary] = useState<BillingSummary | null>(null);
  const [selectedPlan, setSelectedPlan] = useState<"pro" | "scale">("pro");
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      setSummary(await api.billing.summary() as BillingSummary);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load billing status");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const checkout = async () => {
    setWorking(true); setError("");
    try {
      const session = await api.billing.checkout(selectedPlan) as { url: string };
      window.location.assign(session.url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to start checkout");
      setWorking(false);
    }
  };

  const portal = async () => {
    setWorking(true); setError("");
    try {
      const session = await api.billing.portal() as { url: string };
      window.location.assign(session.url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to open billing portal");
      setWorking(false);
    }
  };

  if (loading) return <div className="text-center py-8">Loading...</div>;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Billing</h1>
      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
      <section className="bg-white shadow rounded-lg p-6 space-y-4">
        <h2 className="text-lg font-medium">Current subscription</h2>
        <dl className="grid gap-3 sm:grid-cols-2 text-sm">
          <div><dt className="text-gray-500">Plan</dt><dd className="font-medium">{summary?.plan ?? "free"}</dd></div>
          <div><dt className="text-gray-500">Status</dt><dd className="font-medium">{summary?.status ?? "inactive"}</dd></div>
          {summary?.current_period_end && <div><dt className="text-gray-500">Current period ends</dt><dd>{new Date(summary.current_period_end).toLocaleDateString()}</dd></div>}
          <div><dt className="text-gray-500">Cancellation</dt><dd>{summary?.cancel_at_period_end ? "At period end" : "Not scheduled"}</dd></div>
        </dl>
        {summary?.has_customer && <button type="button" onClick={() => void portal()} disabled={working} className="rounded-md border px-4 py-2 text-sm disabled:opacity-50">Manage billing</button>}
      </section>
      <section className="bg-white shadow rounded-lg p-6 space-y-4">
        <h2 className="text-lg font-medium">Change plan</h2>
        <p className="text-sm text-gray-600">Checkout and subscription state are handled by Stripe. The API activates a paid plan only after a verified webhook.</p>
        <div className="flex flex-wrap items-end gap-3">
          <label className="text-sm"><span className="block text-gray-700 mb-1">Plan</span><select value={selectedPlan} onChange={(event) => setSelectedPlan(event.target.value as "pro" | "scale")} className="border rounded-md px-3 py-2"><option value="pro">Pro</option><option value="scale">Scale</option></select></label>
          <button type="button" onClick={() => void checkout()} disabled={working} className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">Start checkout</button>
        </div>
      </section>
    </div>
  );
}
