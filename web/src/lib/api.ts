import { createClient } from "@/lib/supabase/client";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
const ACTIVE_TEAM_KEY = "sender-api.active-team";

interface RequestOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, headers = {} } = options;

  const supabase = createClient();
  const { data: { session } } = await supabase.auth.getSession();

  const authHeaders: Record<string, string> = {};
  if (session?.access_token) {
    authHeaders["Authorization"] = `Bearer ${session.access_token}`;
  }

  const requestHeaders = {
    ...authHeaders,
    ...headers,
  };
  if (typeof window !== "undefined" && !path.startsWith("/teams")) {
    const teamId = await getActiveTeamId(authHeaders);
    if (teamId) {
      requestHeaders["X-Team-ID"] = teamId;
    }
  }

  const res = await fetch(`${API_BASE_URL}/api/v1${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...requestHeaders,
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ error: "Unknown error" }));
    throw new Error(error.error || `HTTP ${res.status}`);
  }

  if (res.status === 204) {
    return {} as T;
  }

  return res.json();
}

async function getActiveTeamId(authHeaders: Record<string, string>): Promise<string> {
  const stored = window.localStorage.getItem(ACTIVE_TEAM_KEY);
  const response = await fetch(`${API_BASE_URL}/api/v1/teams`, {
    headers: authHeaders,
  });
  if (!response.ok) {
    clearActiveTeamId();
    return "";
  }
  const teams = (await response.json()) as { id: string }[];
  const activeTeam = teams.find((team) => team.id === stored) ?? teams[0];
  if (activeTeam?.id) {
    setActiveTeamId(activeTeam.id);
    return activeTeam.id;
  }
  clearActiveTeamId();
  return "";
}

export function setActiveTeamId(teamId: string) {
  if (typeof window !== "undefined") {
    window.localStorage.setItem(ACTIVE_TEAM_KEY, teamId);
  }
}

export function clearActiveTeamId() {
  if (typeof window !== "undefined") {
    window.localStorage.removeItem(ACTIVE_TEAM_KEY);
  }
}

export const api = {
  emails: {
    list: (limit = 50, offset = 0) =>
      request(`/emails?limit=${limit}&offset=${offset}`),
    get: (id: string) => request(`/emails/${id}`),
    send: (data: unknown) =>
      request("/emails", { method: "POST", body: data }),
    batch: (data: unknown) =>
      request("/emails/batch", { method: "POST", body: data }),
    events: (id: string) => request(`/emails/${id}/events`),
    cancel: (id: string) =>
      request(`/emails/${id}`, { method: "DELETE" }),
  },
  teams: {
    list: () => request("/teams"),
    get: (id: string) => request(`/teams/${id}`),
    create: (data: unknown) =>
      request("/teams", { method: "POST", body: data }),
    update: (id: string, data: unknown) =>
      request(`/teams/${id}`, { method: "PATCH", body: data }),
    delete: (id: string) =>
      request(`/teams/${id}`, { method: "DELETE" }),
    members: (id: string) => request(`/teams/${id}/members`),
    invite: (id: string, data: unknown) =>
      request(`/teams/${id}/invite`, { method: "POST", body: data }),
  },
  contacts: {
    list: (limit = 50, offset = 0) =>
      request(`/contacts?limit=${limit}&offset=${offset}`),
    get: (id: string) => request(`/contacts/${id}`),
    create: (data: unknown) =>
      request("/contacts", { method: "POST", body: data }),
    update: (id: string, data: unknown) =>
      request(`/contacts/${id}`, { method: "PATCH", body: data }),
    delete: (id: string) =>
      request(`/contacts/${id}`, { method: "DELETE" }),
    import: (data: unknown) =>
      request("/contacts/import", { method: "POST", body: data }),
  },
  domains: {
    list: () => request("/domains"),
    get: (id: string) => request(`/domains/${id}`),
    create: (data: unknown) =>
      request("/domains", { method: "POST", body: data }),
    verify: (id: string) =>
      request(`/domains/${id}/verify`, { method: "POST" }),
    delete: (id: string) =>
      request(`/domains/${id}`, { method: "DELETE" }),
  },
  apiKeys: {
    list: () => request("/api-keys"),
    create: (data: unknown) =>
      request("/api-keys", { method: "POST", body: data }),
    delete: (id: string) =>
      request(`/api-keys/${id}`, { method: "DELETE" }),
  },
  webhooks: {
    list: () => request("/webhooks"),
    get: (id: string) => request(`/webhooks/${id}`),
    create: (data: unknown) =>
      request("/webhooks", { method: "POST", body: data }),
    update: (id: string, data: unknown) =>
      request(`/webhooks/${id}`, { method: "PATCH", body: data }),
    delete: (id: string) =>
      request(`/webhooks/${id}`, { method: "DELETE" }),
  },
  inbound: {
    list: (limit = 50, offset = 0) =>
      request(`/inbound?limit=${limit}&offset=${offset}`),
  },
};
