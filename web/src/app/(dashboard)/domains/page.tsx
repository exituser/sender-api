"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface Domain {
  id: string;
  name: string;
  status: string;
  spf_status: string;
  dkim_status: string;
  dmarc_status: string;
  created_at: string;
  dns_records?: DNSRecord[];
}

interface DNSRecord {
  type: string;
  host: string;
  value: string;
  ttl: number;
  status: string;
  optional?: boolean;
}

interface DomainSetup {
  id: string;
  name: string;
  dns_records: DNSRecord[];
}

function statusLabel(status: string) {
  switch (status) {
    case "verified": return "Ready to send";
    case "failed": return "Needs attention";
    default: return "Setup in progress";
  }
}

function statusClasses(status: string) {
  switch (status) {
    case "verified": return "bg-green-100 text-green-800";
    case "failed": return "bg-red-100 text-red-800";
    default: return "bg-amber-100 text-amber-800";
  }
}

function recordStatus(status: string) {
  if (status === "verified") return "Ready";
  if (status === "failed") return "Update this record";
  return "Add this record";
}

function recordLabel(record: DNSRecord) {
  if (record.optional) return "Optional — only for receiving email";
  return record.type;
}

function recordStatusLabel(record: DNSRecord) {
  if (record.optional) return "Optional";
  return recordStatus(record.status);
}

function domainNextStep(item: Domain) {
  if (item.status !== "verified") return "Publish the records below, then check again.";
  if (item.dmarc_status !== "verified") return "Add sender protection to unlock marketing sending.";
  return "Your essential setup is complete.";
}

export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [setup, setSetup] = useState<DomainSetup | null>(null);

  const selectSetup = useCallback((item: Domain) => {
    if (!item.dns_records?.length) {
      setSetup(null);
      return;
    }
    window.localStorage.setItem("sender-api.domain-setup-id", item.id);
    setSetup({ id: item.id, name: item.name, dns_records: item.dns_records });
  }, []);

  const loadDomains = useCallback(async () => {
    setError("");
    try {
      const data = await api.domains.list() as { data: Domain[] };
      const nextDomains = data.data || [];
      setDomains(nextDomains);
      const savedID = window.localStorage.getItem("sender-api.domain-setup-id");
      const selected = nextDomains.find((item) => item.id === savedID) ?? nextDomains[0];
      if (selected) selectSetup(selected);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to load your domains. Try again.");
    } finally {
      setLoading(false);
    }
  }, [selectSetup]);

  useEffect(() => {
    void Promise.resolve().then(loadDomains);
  }, [loadDomains]);

  const addDomain = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      const created = await api.domains.create({ name }) as DomainSetup;
      setName("");
      setShowForm(false);
      setSetup(created);
      window.localStorage.setItem("sender-api.domain-setup-id", created.id);
      await loadDomains();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to add this domain. Check the name and try again.");
    }
  };

  const verifyDomain = async (id: string) => {
    setError("");
    try {
      await api.domains.verify(id);
      await loadDomains();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to check this domain. Try again.");
    }
  };

  const viewSetup = (item: Domain) => {
    selectSetup(item);
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  if (loading) return <div className="py-8 text-center text-sm text-gray-600" aria-busy="true">Loading your domains…</div>;

  return (
    <div className="space-y-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="dashboard-eyebrow">Sender setup</p>
          <h1 className="mt-1 text-3xl font-semibold tracking-tight text-gray-950">Domains</h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-gray-600">Connect the domain people will see when your messages arrive. We’ll check the setup for you.</p>
        </div>
        <button type="button" onClick={() => setShowForm((visible) => !visible)} aria-expanded={showForm} aria-controls="domain-form" className="dashboard-button dashboard-button-primary self-start sm:self-auto">
          {showForm ? "Close form" : "Add a domain"}
        </button>
      </div>

      {error && <div className="dashboard-error-card" role="alert">{error}</div>}

      {showForm && (
        <form id="domain-form" onSubmit={addDomain} className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm sm:p-6">
          <label htmlFor="domain-name" className="block text-sm font-semibold text-gray-900">Domain name</label>
          <p className="mt-1 text-sm leading-6 text-gray-600">Use the domain you control, such as example.com.</p>
          <div className="mt-4 flex flex-col gap-3 sm:flex-row">
            <input id="domain-name" name="domain" autoComplete="url" required type="text" placeholder="example.com" value={name} onChange={(e) => setName(e.target.value)} className="min-h-10 flex-1 rounded-xl border border-gray-300 px-3 text-base text-gray-900 focus:border-blue-600 focus:ring-2 focus:ring-blue-200 sm:text-sm" />
            <button type="submit" className="dashboard-button dashboard-button-primary">Continue</button>
          </div>
        </form>
      )}

      {setup && (
        <section className="rounded-2xl border border-blue-200 bg-blue-50 p-5 sm:p-6" aria-labelledby="domain-setup-title">
          <div>
            <p className="dashboard-eyebrow text-blue-700">One-time setup</p>
            <h2 id="domain-setup-title" className="mt-1 text-xl font-semibold text-blue-950">Connect {setup.name}</h2>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-blue-900">Add the records below wherever you manage your domain. If you’re not sure where that is, send this list to your domain provider. Then choose “Check again”.</p>
          </div>
          <div className="mt-5 overflow-x-auto rounded-xl border border-blue-200 bg-white">
            <table className="min-w-full text-sm">
              <thead className="bg-blue-50 text-left text-blue-950">
                <tr><th className="px-4 py-3 font-semibold">Record</th><th className="px-4 py-3 font-semibold">Where to add it</th><th className="px-4 py-3 font-semibold">Value</th><th className="px-4 py-3 font-semibold">Status</th></tr>
              </thead>
              <tbody>
                {setup.dns_records.map((record) => (
                  <tr key={record.type + "-" + record.host} className="border-t border-blue-100 align-top">
                    <td className="px-4 py-3 font-medium text-gray-900">{recordLabel(record)}</td>
                    <td className="px-4 py-3 text-gray-700"><code className="break-all">{record.host}</code></td>
                    <td className="max-w-md break-all px-4 py-3 text-gray-700"><code>{record.value}</code></td>
                    <td className="whitespace-nowrap px-4 py-3 text-gray-700">{recordStatusLabel(record)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {domains.length === 0 ? (
        <section className="rounded-2xl border border-dashed border-gray-300 bg-white p-8 text-center" aria-labelledby="empty-domains-title">
          <h2 id="empty-domains-title" className="font-semibold text-gray-950">No sender domain yet</h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-gray-600">Add the domain you want customers to see in the From address. We’ll guide you through the short setup.</p>
          <button type="button" onClick={() => setShowForm(true)} className="dashboard-button dashboard-button-primary mt-5">Add a domain</button>
        </section>
      ) : (
        <section aria-labelledby="connected-domains-title">
          <div className="mb-4"><h2 id="connected-domains-title" className="text-xl font-semibold text-gray-950">Connected domains</h2><p className="mt-1 text-sm text-gray-600">A domain marked “Ready to send” can be used for transactional messages.</p></div>
          <div className="grid gap-4 lg:grid-cols-2">
            {domains.map((item) => (
              <article key={item.id} className="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm sm:p-6">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0"><h3 className="truncate font-semibold text-gray-950">{item.name}</h3><p className="mt-2 text-sm leading-6 text-gray-600">{domainNextStep(item)}</p></div>
                  <span className={`shrink-0 rounded-full px-2.5 py-1 text-xs font-semibold ${statusClasses(item.status)}`}>{statusLabel(item.status)}</span>
                </div>
                {item.status === "verified" && item.dmarc_status !== "verified" && <div className="mt-4 rounded-xl bg-amber-50 p-3 text-sm leading-6 text-amber-900"><strong>Marketing note:</strong> add sender protection before sending marketing messages.</div>}
                <div className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
                  <span className={item.spf_status === "verified" ? "text-green-700" : "text-gray-600"}>{item.spf_status === "verified" ? "✓ Sender records ready" : "○ Sender records to do"}</span>
                  <span className={item.dkim_status === "verified" ? "text-green-700" : "text-gray-600"}>{item.dkim_status === "verified" ? "✓ Email authentication ready" : "○ Email authentication to do"}</span>
                  <span className={item.dmarc_status === "verified" ? "text-green-700" : "text-gray-600"}>{item.dmarc_status === "verified" ? "✓ Sender protection ready" : "○ Sender protection optional for transactional"}</span>
                </div>
                <div className="mt-5 flex flex-wrap gap-3">
                  <button type="button" onClick={() => viewSetup(item)} className="dashboard-button dashboard-button-secondary">View setup</button>
                  {item.status !== "verified" && <button type="button" onClick={() => void verifyDomain(item.id)} className="dashboard-button dashboard-button-secondary">Check again</button>}
                </div>
              </article>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
