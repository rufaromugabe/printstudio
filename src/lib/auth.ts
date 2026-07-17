const ACCESS_TOKEN_KEY = "printstudio-access-token";
const SESSION_USER_KEY = "printstudio-session-user";
const LEGACY_GOOGLE_TOKEN_KEY = "printstudio-google-token";

export type SessionUser = {
  id: string;
  workspaceId: string;
  role: string;
  email?: string;
  displayName?: string;
};

export type AuthSession = {
  accessToken: string;
  tokenType: string;
  expiresAt: string;
  expiresIn: number;
  user: SessionUser;
};

const apiBase = () => process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export function getAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return sessionStorage.getItem(ACCESS_TOKEN_KEY);
}

export function setAccessToken(token: string) {
  sessionStorage.setItem(ACCESS_TOKEN_KEY, token);
  localStorage.removeItem(LEGACY_GOOGLE_TOKEN_KEY);
}

export function getSessionUser(): SessionUser | null {
  if (typeof window === "undefined") return null;
  const raw = sessionStorage.getItem(SESSION_USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as SessionUser;
  } catch {
    return null;
  }
}

export function setSessionUser(user: SessionUser) {
  sessionStorage.setItem(SESSION_USER_KEY, JSON.stringify(user));
}

export function clearAccessToken() {
  sessionStorage.removeItem(ACCESS_TOKEN_KEY);
  sessionStorage.removeItem(SESSION_USER_KEY);
  localStorage.removeItem(LEGACY_GOOGLE_TOKEN_KEY);
}

export function hasSession(): boolean {
  return Boolean(getAccessToken());
}

export function isAdminRole(role?: string | null): boolean {
  return role === "owner" || role === "admin";
}

export async function exchangeGoogleCredential(idToken: string): Promise<AuthSession> {
  const response = await fetch(`${apiBase()}/v1/auth/google`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ idToken }),
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.message ?? `Google sign-in failed (${response.status})`);
  }
  const session = body as AuthSession;
  setAccessToken(session.accessToken);
  setSessionUser(session.user);
  return session;
}

export async function fetchSessionUser(): Promise<SessionUser> {
  const response = await fetch(`${apiBase()}/v1/auth/me`, {
    credentials: "include",
    headers: { ...authHeaders() },
  });
  const body = await response.json().catch(() => ({}));
  if (response.status === 401) {
    handleUnauthorized();
    throw new Error(body.message ?? "Session expired — sign in again");
  }
  if (!response.ok) {
    throw new Error(body.message ?? `Could not load session (${response.status})`);
  }
  const user = body as SessionUser;
  setSessionUser(user);
  return user;
}

export async function logoutSession(): Promise<void> {
  try {
    await fetch(`${apiBase()}/v1/auth/logout`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
  } finally {
    clearAccessToken();
  }
}

export function authHeaders(extra?: HeadersInit): HeadersInit {
  const token = getAccessToken();
  return {
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...extra,
  };
}

export function handleUnauthorized() {
  clearAccessToken();
  if (typeof window !== "undefined" && process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID) {
    window.location.reload();
  }
}
