"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

interface Contact {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  subscribed: boolean;
  created_at: string;
}

export default function ContactsPage() {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [error, setError] = useState("");
  const [form, setForm] = useState({ email: "", first_name: "", last_name: "" });

  const loadContacts = useCallback(async () => {
    try {
      const data = await api.contacts.list() as { data: Contact[] };
      setContacts(data.data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadContacts);
  }, [loadContacts]);

  const addContact = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      await api.contacts.create(form);
      setForm({ email: "", first_name: "", last_name: "" });
      setShowForm(false);
      await loadContacts();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add contact");
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Contacts</h1>
        <button onClick={() => setShowForm((visible) => !visible)} className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Add Contact
        </button>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm">{error}</div>}
      {showForm && (
        <form onSubmit={addContact} className="bg-white shadow rounded-lg p-6 grid gap-3 md:grid-cols-3">
          <input required type="email" placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} className="px-3 py-2 border rounded-md" />
          <input placeholder="First name" value={form.first_name} onChange={(e) => setForm({ ...form, first_name: e.target.value })} className="px-3 py-2 border rounded-md" />
          <input placeholder="Last name" value={form.last_name} onChange={(e) => setForm({ ...form, last_name: e.target.value })} className="px-3 py-2 border rounded-md" />
          <button type="submit" className="md:col-span-3 px-4 py-2 bg-green-600 text-white rounded-md">Save contact</button>
        </form>
      )}

      <div className="bg-white shadow rounded-lg overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Email
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Name
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Status
              </th>
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                Created
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {contacts.map((contact) => (
              <tr key={contact.id}>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                  {contact.email}
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
                  {contact.first_name} {contact.last_name}
                </td>
                <td className="px-6 py-4 whitespace-nowrap">
                  <span
                    className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                      contact.subscribed
                        ? "bg-green-100 text-green-800"
                        : "bg-red-100 text-red-800"
                    }`}
                  >
                    {contact.subscribed ? "Subscribed" : "Unsubscribed"}
                  </span>
                </td>
                <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                  {new Date(contact.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {contacts.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            No contacts found
          </div>
        )}
      </div>
    </div>
  );
}
