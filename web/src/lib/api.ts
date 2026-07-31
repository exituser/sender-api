import { createClient } from "@/lib/supabase/client";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
const ACTIVE_TEAM_KEY = "sender-api.active-team";

let activeTeamCache: { sessionKey: string; teamId: string } | undefined;
let activeTeamRequest: { sessionKey: string; promise: Promise<string> } | undefined;
let activeTeamGeneration = 0;

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

  const requestHeaders: Record<string, string> = {
    ...authHeaders,
    ...headers,
  };
  if (typeof window !== "undefined" && !path.startsWith("/teams")) {
    const teamId = await getActiveTeamId(session?.access_token, authHeaders);
    if (teamId) {
      requestHeaders["X-Team-ID"] = teamId;
    }
  }

  const isFormData = typeof FormData !== "undefined" && body instanceof FormData;
  if (!isFormData) {
    requestHeaders["Content-Type"] = "application/json";
  }

  const res = await fetch(`${API_BASE_URL}/api/v1${path}`, {
    method,
    headers: requestHeaders,
    body: body ? (isFormData ? body as FormData : JSON.stringify(body)) : undefined,
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

async function getActiveTeamId(
  accessToken: string | undefined,
  authHeaders: Record<string, string>
): Promise<string> {
  if (!accessToken) {
    invalidateActiveTeamCache();
    return "";
  }

  if (activeTeamCache?.sessionKey === accessToken) {
    return activeTeamCache.teamId;
  }

  if (activeTeamRequest?.sessionKey === accessToken) {
    return activeTeamRequest.promise;
  }

  const generation = activeTeamGeneration;
  const promise = fetchActiveTeamId(accessToken, authHeaders, generation);
  activeTeamRequest = { sessionKey: accessToken, promise };
  try {
    return await promise;
  } finally {
    if (activeTeamRequest?.promise === promise) {
      activeTeamRequest = undefined;
    }
  }
}

async function fetchActiveTeamId(
  sessionKey: string,
  authHeaders: Record<string, string>,
  generation: number
): Promise<string> {
  const stored = window.localStorage.getItem(ACTIVE_TEAM_KEY);
  const response = await fetch(`${API_BASE_URL}/api/v1/teams`, {
    headers: authHeaders,
  });
  if (!response.ok) {
    invalidateActiveTeamCache();
    return "";
  }
  const teams = (await response.json()) as { id: string }[];
  const activeTeam = teams.find((team) => team.id === stored) ?? teams[0];
  if (activeTeam?.id) {
    if (generation !== activeTeamGeneration) {
      return "";
    }
    window.localStorage.setItem(ACTIVE_TEAM_KEY, activeTeam.id);
    activeTeamCache = { sessionKey, teamId: activeTeam.id };
    return activeTeam.id;
  }
  invalidateActiveTeamCache();
  return "";
}

export function setActiveTeamId(teamId: string) {
  if (typeof window !== "undefined") {
    window.localStorage.setItem(ACTIVE_TEAM_KEY, teamId);
  }
  invalidateActiveTeamCache();
}

export function clearActiveTeamId() {
  if (typeof window !== "undefined") {
    window.localStorage.removeItem(ACTIVE_TEAM_KEY);
  }
  invalidateActiveTeamCache();
}

export function invalidateActiveTeamCache() {
  activeTeamGeneration += 1;
  activeTeamCache = undefined;
  activeTeamRequest = undefined;
}

function createIdempotencyKey(): string {
  if (typeof globalThis.crypto?.randomUUID === "function") {
    return globalThis.crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export const api = {
  emails: {
    list: (limit = 50, offset = 0) =>
      request(`/emails?limit=${limit}&offset=${offset}`),
    get: (id: string) => request(`/emails/${id}`),
    send: (data: unknown, idempotencyKey = createIdempotencyKey()) =>
      request("/emails", { method: "POST", body: data, headers: { "Idempotency-Key": idempotencyKey } }),
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
      request("/teams", { method: "POST", body: data }).finally(invalidateActiveTeamCache),
    update: (id: string, data: unknown) =>
      request(`/teams/${id}`, { method: "PATCH", body: data }).finally(invalidateActiveTeamCache),
    delete: (id: string) =>
      request(`/teams/${id}`, { method: "DELETE" }).finally(invalidateActiveTeamCache),
    members: (id: string) => request(`/teams/${id}/members`),
    invite: (id: string, data: unknown) =>
      request(`/teams/${id}/invite`, { method: "POST", body: data }),
    invitations: (id: string) => request(`/teams/${id}/invitations`),
    revokeInvitation: (id: string, invitationId: string) =>
      request(`/teams/${id}/invitations/${invitationId}`, { method: "DELETE" }),
    acceptInvitation: (data: unknown) =>
      request("/teams/invitations/accept", { method: "POST", body: data }),
    removeMember: (id: string, userId: string) =>
      request(`/teams/${id}/members/${userId}`, { method: "DELETE" }),
    updateMemberRole: (id: string, userId: string, role: string) =>
      request(`/teams/${id}/members/${userId}/role`, { method: "PATCH", body: { role } }),
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
    import: (data: FormData) =>
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
    deliveries: (id: string, limit = 50) =>
      request(`/webhooks/${id}/deliveries?limit=${limit}`),
    test: (id: string) =>
      request(`/webhooks/${id}/test`, { method: "POST" }),
  },
  inbound: {
    list: (limit = 50, offset = 0) =>
      request(`/inbound?limit=${limit}&offset=${offset}`),
    get: (id: string) => request(`/inbound/${id}`),
  },
  billing: {
    summary: () => request("/billing"),
    checkout: (plan: string) => request("/billing/checkout", { method: "POST", body: { plan } }),
    portal: () => request("/billing/portal", { method: "POST" }),
  },
};
