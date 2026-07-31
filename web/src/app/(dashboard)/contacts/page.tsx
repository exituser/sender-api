"use client";

import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Pagination } from "@/components/pagination";

interface Contact {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  subscribed: boolean;
  properties?: Record<string, string>;
  created_at: string;
}

export default function ContactsPage() {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const emptyForm = { email: "", first_name: "", last_name: "", subscribed: true, properties: "" };
  const [form, setForm] = useState(emptyForm);
  const [editing, setEditing] = useState<Contact | null>(null);
  const [importing, setImporting] = useState(false);

  const loadContacts = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await api.contacts.list(50, page * 50) as { data: Contact[]; total: number };
      setContacts(data.data || []);
      setTotal(data.total || 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load contacts");
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    void Promise.resolve().then(loadContacts);
  }, [loadContacts]);

  const parseProperties = (value: string) => Object.fromEntries(value.split("\n").filter(Boolean).map((line) => {
    const [key, ...parts] = line.split("=");
    if (!key?.trim() || parts.length === 0) throw new Error("Properties must use key=value, one per line");
    return [key.trim(), parts.join("=").trim()];
  }));

  const addContact = async (event: React.FormEvent) => {
    event.preventDefault();
    setError("");
    setSaving(true);
    try {
      const payload = { email: form.email, first_name: form.first_name || undefined, last_name: form.last_name || undefined, subscribed: form.subscribed, properties: parseProperties(form.properties) };
      if (editing) await api.contacts.update(editing.id, payload);
      else await api.contacts.create(payload);
      setForm(emptyForm);
      setEditing(null);
      setShowForm(false);
      await loadContacts();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add contact");
    } finally {
      setSaving(false);
    }
  };

  const beginEdit = (contact: Contact) => {
    setEditing(contact);
    setForm({ email: contact.email, first_name: contact.first_name ?? "", last_name: contact.last_name ?? "", subscribed: contact.subscribed, properties: Object.entries(contact.properties ?? {}).map(([key, value]) => `${key}=${value}`).join("\n") });
    setShowForm(true);
  };

  const deleteContact = async (id: string) => {
    if (!window.confirm("Delete this contact?")) return;
    setError("");
    try { await api.contacts.delete(id); await loadContacts(); }
    catch (err) { setError(err instanceof Error ? err.message : "Failed to delete contact"); }
  };

  const importContacts = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setImporting(true); setError("");
    try { const data = new FormData(); data.append("file", file); await api.contacts.import(data); await loadContacts(); }
    catch (err) { setError(err instanceof Error ? err.message : "Failed to import contacts"); }
    finally { setImporting(false); }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center gap-3">
        <h1 className="text-2xl font-bold">Contacts</h1>
        <div className="flex gap-3"><label className="px-4 py-2 bg-gray-700 text-white rounded-md hover:bg-gray-800 cursor-pointer">{importing ? "Importing..." : "Import CSV"}<input type="file" accept=".csv,text/csv" onChange={importContacts} disabled={importing} className="sr-only" /></label><button type="button" onClick={() => { setEditing(null); setForm(emptyForm); setShowForm((visible) => !visible); }} aria-expanded={showForm} aria-controls="contact-form" className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">
          Add Contact
        </button></div>
      </div>

      {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
      {showForm && (
        <form id="contact-form" onSubmit={addContact} className="bg-white shadow rounded-lg p-6 grid gap-3 md:grid-cols-3">
          <label htmlFor="contact-email" className="sr-only">Email</label>
          <input id="contact-email" name="email" autoComplete="email" required type="email" placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} className="px-3 py-2 border rounded-md" />
          <label htmlFor="contact-first-name" className="sr-only">First name</label>
          <input id="contact-first-name" name="first_name" autoComplete="off" placeholder="First name" value={form.first_name} onChange={(e) => setForm({ ...form, first_name: e.target.value })} className="px-3 py-2 border rounded-md" />
          <label htmlFor="contact-last-name" className="sr-only">Last name</label>
          <input id="contact-last-name" name="last_name" autoComplete="off" placeholder="Last name" value={form.last_name} onChange={(e) => setForm({ ...form, last_name: e.target.value })} className="px-3 py-2 border rounded-md" />
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={form.subscribed} onChange={(e) => setForm({ ...form, subscribed: e.target.checked })} /> Subscribed</label>
          <textarea placeholder="Properties: key=value" value={form.properties} onChange={(e) => setForm({ ...form, properties: e.target.value })} className="md:col-span-2 px-3 py-2 border rounded-md" />
          <button disabled={saving} aria-busy={saving} type="submit" className="md:col-span-3 px-4 py-2 bg-green-600 text-white rounded-md disabled:opacity-50">{saving ? "Saving..." : editing ? "Update contact" : "Save contact"}</button>
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
              <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
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
                <td className="px-6 py-4 whitespace-nowrap text-sm"><button type="button" onClick={() => beginEdit(contact)} className="text-blue-600 hover:underline">Edit</button><button type="button" onClick={() => deleteContact(contact.id)} className="ml-3 text-red-600 hover:underline">Delete</button></td>
              </tr>
            ))}
          </tbody>
        </table>
        {contacts.length === 0 && (
          <div className="text-center py-8 text-gray-500">
            No contacts found
          </div>
        )}
        <Pagination page={page} pageSize={50} total={total} onPageChange={setPage} disabled={loading} />
      </div>
    </div>
  );
}
