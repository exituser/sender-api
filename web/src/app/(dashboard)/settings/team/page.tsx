"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { api, setActiveTeamId } from "@/lib/api";

type TeamInfo = { id: string; name: string; plan: string };
type Member = { user_id: string; email: string; role: "owner" | "admin" | "member"; created_at: string };
type Invitation = { id: string; email: string; role: "admin" | "member"; status: string; expires_at: string };

export default function SettingsPage() {
  const [teams, setTeams] = useState<TeamInfo[]>([]);
  const [team, setTeam] = useState<TeamInfo | null>(null);
  const [members, setMembers] = useState<Member[]>([]);
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [teamName, setTeamName] = useState("");
  const [invite, setInvite] = useState({ email: "", role: "member" });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [inviteToken, setInviteToken] = useState("");
  const [acceptToken, setAcceptToken] = useState("");

  const loadTeam = useCallback(async (teamId?: string) => {
    setLoading(true); setError("");
    try {
      const data = await api.teams.list() as TeamInfo[];
      const selected = data.find((item) => item.id === teamId) ?? data[0] ?? null;
      setTeams(data); setTeam(selected); setTeamName(selected?.name ?? "");
      if (selected) {
        const [teamMembers, teamInvitations] = await Promise.all([
          api.teams.members(selected.id),
          api.teams.invitations(selected.id),
        ]);
        setMembers(teamMembers as Member[]);
        setInvitations(teamInvitations as Invitation[]);
      } else { setMembers([]); setInvitations([]); }
    } catch (err) { setError(err instanceof Error ? err.message : "Failed to load team"); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { void loadTeam(); }, [loadTeam]);

  const submitTeam = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); setSaving(true); setError("");
    try {
      if (team) { await api.teams.update(team.id, { name: teamName }); await loadTeam(team.id); }
      else { const created = await api.teams.create({ name: teamName }) as TeamInfo; setActiveTeamId(created.id); await loadTeam(created.id); }
    } catch (err) { setError(err instanceof Error ? err.message : "Failed to save team"); }
    finally { setSaving(false); }
  };

  const changeTeam = async (id: string) => { setActiveTeamId(id); await loadTeam(id); };
  const addMember = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault(); if (!team) return; setSaving(true); setError("");
    try { const response = await api.teams.invite(team.id, invite) as { token?: string }; setInviteToken(response.token ?? ""); setInvite({ email: "", role: "member" }); await loadTeam(team.id); }
    catch (err) { setError(err instanceof Error ? err.message : "Failed to invite member"); }
    finally { setSaving(false); }
  };
  const updateRole = async (member: Member, role: string) => { if (!team) return; setSaving(true); setError(""); try { await api.teams.updateMemberRole(team.id, member.user_id, role); await loadTeam(team.id); } catch (err) { setError(err instanceof Error ? err.message : "Failed to update role"); } finally { setSaving(false); } };
  const removeMember = async (member: Member) => { if (!team || !window.confirm(`Remove ${member.email} from this team?`)) return; setSaving(true); setError(""); try { await api.teams.removeMember(team.id, member.user_id); await loadTeam(team.id); } catch (err) { setError(err instanceof Error ? err.message : "Failed to remove member"); } finally { setSaving(false); } };
  const revokeInvitation = async (invitation: Invitation) => { if (!team || !window.confirm(`Revoke invitation for ${invitation.email}?`)) return; setSaving(true); setError(""); try { await api.teams.revokeInvitation(team.id, invitation.id); await loadTeam(team.id); } catch (err) { setError(err instanceof Error ? err.message : "Failed to revoke invitation"); } finally { setSaving(false); } };
  const acceptInvitation = async (event: FormEvent<HTMLFormElement>) => { event.preventDefault(); setSaving(true); setError(""); try { await api.teams.acceptInvitation({ token: acceptToken }); setAcceptToken(""); await loadTeam(); } catch (err) { setError(err instanceof Error ? err.message : "Failed to accept invitation"); } finally { setSaving(false); } };

  if (loading) return <div className="text-center py-8">Loading...</div>;
  return <div className="space-y-6">
    <h1 className="text-2xl font-bold">Team Settings</h1>
    {error && <div className="p-3 bg-red-50 text-red-700 rounded-md text-sm" role="alert">{error}</div>}
    {teams.length > 1 && <div className="bg-white shadow rounded-lg p-6"><label htmlFor="active-team" className="block text-sm font-medium text-gray-700">Active team</label><select id="active-team" value={team?.id ?? ""} onChange={(event) => void changeTeam(event.target.value)} className="mt-1 w-full px-3 py-2 border rounded-md">{teams.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></div>}
    <form onSubmit={submitTeam} className="bg-white shadow rounded-lg p-6 space-y-4"><h2 className="text-lg font-medium">{team ? "Team information" : "Create your first team"}</h2><label className="block text-sm font-medium text-gray-700" htmlFor="team-name">Team name</label><input id="team-name" value={teamName} onChange={(event) => setTeamName(event.target.value)} className="w-full px-3 py-2 border rounded-md" maxLength={255} required /><p className="text-sm text-gray-600">Plan: {team?.plan ?? "free"}</p><button type="submit" disabled={saving} className="rounded-md bg-black px-4 py-2 text-sm font-medium text-white disabled:opacity-50">{saving ? "Saving..." : team ? "Save team" : "Create team"}</button></form>
    <form onSubmit={acceptInvitation} className="bg-white shadow rounded-lg p-6 space-y-3"><h2 className="text-lg font-medium">Accept an invitation</h2><label htmlFor="invitation-token" className="text-sm text-gray-600">Paste a one-time invitation token</label><div className="flex gap-3"><input id="invitation-token" value={acceptToken} onChange={(event) => setAcceptToken(event.target.value)} className="flex-1 px-3 py-2 border rounded-md" required /><button type="submit" disabled={saving} className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50">Accept</button></div></form>
    {team && <><form onSubmit={addMember} className="bg-white shadow rounded-lg p-6 space-y-3"><h2 className="text-lg font-medium">Invite member</h2><div className="grid gap-3 md:grid-cols-3"><input aria-label="Member email" type="email" placeholder="member@example.com" value={invite.email} onChange={(event) => setInvite({ ...invite, email: event.target.value })} required className="px-3 py-2 border rounded-md" /><select aria-label="Invited member role" value={invite.role} onChange={(event) => setInvite({ ...invite, role: event.target.value })} className="px-3 py-2 border rounded-md"><option value="member">Member</option><option value="admin">Admin</option></select><button disabled={saving} className="px-4 py-2 bg-blue-600 text-white rounded-md disabled:opacity-50">Invite</button></div>{inviteToken && <output className="block rounded bg-yellow-50 p-3 text-sm text-yellow-900">Copy this one-time invitation token and send it securely: <code className="break-all">{inviteToken}</code><button type="button" className="ml-3 underline" onClick={() => void navigator.clipboard?.writeText(inviteToken)}>Copy</button></output>}</form>{invitations.length > 0 && <div className="bg-white shadow rounded-lg p-6"><h2 className="text-lg font-medium mb-3">Pending invitations</h2><ul className="space-y-2">{invitations.map((invitation) => <li key={invitation.id} className="flex flex-wrap items-center justify-between gap-2 border-b pb-2 text-sm"><span>{invitation.email} · {invitation.role} · {invitation.status}</span>{invitation.status === "pending" && <button type="button" disabled={saving} onClick={() => void revokeInvitation(invitation)} className="text-red-600 hover:underline">Revoke</button>}</li>)}</ul></div>}<div className="bg-white shadow rounded-lg overflow-hidden"><table className="min-w-full divide-y divide-gray-200"><thead className="bg-gray-50"><tr><th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Member</th><th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Role</th><th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th></tr></thead><tbody className="divide-y divide-gray-200">{members.map((member) => <tr key={member.user_id}><td className="px-6 py-4 text-sm">{member.email}</td><td className="px-6 py-4"><select aria-label={`Role for ${member.email}`} value={member.role} disabled={saving || member.role === "owner"} onChange={(event) => void updateRole(member, event.target.value)} className="px-2 py-1 border rounded-md text-sm"><option value="owner">Owner</option><option value="admin">Admin</option><option value="member">Member</option></select></td><td className="px-6 py-4 text-sm">{member.role !== "owner" && <button type="button" disabled={saving} onClick={() => void removeMember(member)} className="text-red-600 hover:underline disabled:opacity-50">Remove</button>}</td></tr>)}</tbody></table></div></>}
  </div>;
}
