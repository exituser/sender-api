"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  created_at: string;
}

export default function APIKeysPage() {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [newKey, setNewKey] = useState("");
  const [error, setError] = useState("");

  const loadKeys = useCallback(async () => {
    setError("");
    try {
      const data = await api.apiKeys.list() as APIKey[];
      setKeys(data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load API keys");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadKeys);
  }, [loadKeys]);

  const createKey = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      const response = await api.apiKeys.create({ name }) as { key: string };
      setNewKey(response.key);
      setName("");
      setShowForm(false);
      await loadKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create key");
    }
  };

  const deleteKey = async (id: string) => {
    setError("");
    try {
      await api.apiKeys.delete(id);
      await loadKeys();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete key");
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">API Keys</h1>
        <button type="button" onClick={() => setShowForm((visible) => !visible)} aria-expanded={showForm} aria-controls="api-key-form" className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Create Key
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
      {newKey && <div className="p-3 bg-yellow-50 text-yellow-900 rounded-md text-sm">Copy this key now: <code>{newKey}</code></div>}
      {showForm && (
        <form id="api-key-form" onSubmit={createKey} className="bg-white shadow rounded-lg p-6 flex gap-3">
          <label htmlFor="api-key-name" className="sr-only">Key name</label>
          <input id="api-key-name" name="name" required placeholder="Key name" value={name} onChange={(e) => setName(e.target.value)} className="flex-1 px-3 py-2 border rounded-md" />
          <button type="submit" className="px-4 py-2 bg-green-600 text-white rounded-md">Create</button>
        </form>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Name
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Key
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Created
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {keys.map((key) => (
              <tr key={key.id}>
                <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                  {key.name}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 font-mono">
                  {key.key_prefix}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                  {new Date(key.created_at).toLocaleDateString()}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm">
                  <button onClick={() => deleteKey(key.id)} className="text-red-600 hover:text-red-900">
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {keys.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            No API keys
          </div>
        )}
      </div>
    </div>
  );
}
