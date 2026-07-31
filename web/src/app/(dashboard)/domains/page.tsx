"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface Domain {
  id: string;
  name: string;
  status: string;
  spf_status: string;
  dkim_status: string;
  created_at: string;
}

export default function DomainsPage() {
  const [domains, setDomains] = useState<Domain[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  const loadDomains = useCallback(async () => {
    setError("");
    try {
      const data = await api.domains.list() as { data: Domain[] };
      setDomains(data.data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load domains");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadDomains);
  }, [loadDomains]);

  const getStatusColor = (status: string) => {
    switch (status) {
      case "verified":
        return "bg-green-100 text-green-800";
      case "pending":
        return "bg-yellow-100 text-yellow-800";
      case "failed":
        return "bg-red-100 text-red-800";
      default:
        return "bg-gray-100 text-gray-800";
    }
  };

  const addDomain = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      await api.domains.create({ name });
      setName("");
      setShowForm(false);
      await loadDomains();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add domain");
    }
  };

  const verifyDomain = async (id: string) => {
    try {
      await api.domains.verify(id);
      await loadDomains();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to verify domain");
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Domains</h1>
        <button type="button" onClick={() => setShowForm((visible) => !visible)} aria-expanded={showForm} aria-controls="domain-form" className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Add Domain
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
      {showForm && (
        <form id="domain-form" onSubmit={addDomain} className="bg-white shadow rounded-lg p-6 flex gap-3">
          <label htmlFor="domain-name" className="sr-only">Domain</label>
          <input id="domain-name" name="domain" autoComplete="url" required type="text" placeholder="example.com" value={name} onChange={(e) => setName(e.target.value)} className="flex-1 px-3 py-2 border rounded-md" />
          <button type="submit" className="px-4 py-2 bg-green-600 text-white rounded-md">Save</button>
        </form>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Domain
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                SPF
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                DKIM
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {domains.map((domain) => (
              <tr key={domain.id}>
                <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                  {domain.name}
                </td>
                <td className="px-6 py-4 whitespace-nowrap">
                  <span
                    className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${getStatusColor(
                      domain.status
                    )}`}
                  >
                    {domain.status}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                  {domain.spf_status}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                  {domain.dkim_status}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm">
                  {domain.status !== "verified" && <button onClick={() => verifyDomain(domain.id)} className="text-blue-600 hover:underline">Verify</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {domains.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            No domains configured
          </div>
        )}
      </div>
    </div>
  );
}
