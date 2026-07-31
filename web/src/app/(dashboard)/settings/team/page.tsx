"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { api, setActiveTeamId } from "@/lib/api";

type TeamInfo = {
  id: string;
  name: string;
  plan: string;
};

export default function SettingsPage() {
  const [team, setTeam] = useState<TeamInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [teamName, setTeamName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");

  const loadTeam = useCallback(async () => {
    try {
      const data = await api.teams.list() as TeamInfo[];
      if (data && data.length > 0) {
        setTeam(data[0]);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(loadTeam);
  }, [loadTeam]);

  const createTeam = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setCreating(true);
    setError("");
    try {
      const created = await api.teams.create({ name: teamName }) as TeamInfo;
      setActiveTeamId(created.id);
      setTeam(created);
      setTeamName("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create team");
    } finally {
      setCreating(false);
    }
  };

  if (loading) {
    return <div className="text-center py-8">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Team Settings</h1>

      {team ? <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-lg font-medium mb-4">Team Information</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Team Name
            </label>
            <input
              type="text"
              value={team?.name || ""}
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md"
              readOnly
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700">
              Plan
            </label>
            <p className="mt-1 text-sm text-gray-900">{team?.plan || "free"}</p>
          </div>
        </div>
      </div> : <div className="bg-white shadow rounded-lg p-6">
        <h2 className="text-lg font-medium mb-4">Create your first team</h2>
        <p className="text-sm text-gray-600 mb-4">
          A team is required before you can send email or manage resources.
        </p>
        <form onSubmit={createTeam} className="space-y-4">
          <label className="block text-sm font-medium text-gray-700" htmlFor="team-name">
            Team Name
          </label>
          <input
            id="team-name"
            type="text"
            value={teamName}
            onChange={(event) => setTeamName(event.target.value)}
            className="block w-full px-3 py-2 border border-gray-300 rounded-md"
            maxLength={255}
            required
          />
          {error && <p className="text-sm text-red-600" role="alert">{error}</p>}
          <button
            type="submit"
            disabled={creating}
            className="rounded-md bg-black px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
          >
            {creating ? "Creating..." : "Create team"}
          </button>
        </form>
      </div>}
    </div>
  );
}
