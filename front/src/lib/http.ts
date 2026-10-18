const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';
let accessToken: string | null = null;
let refreshToken: string | null = null;
export function setRefreshToken(t: string | null): void { refreshToken = t; }
let refreshInFlight: Promise<boolean> | null = null;

export class HttpError extends Error {
  constructor(public readonly status: number, message = 'No fue posible completar la solicitud.') {
    super(message);
    this.name = 'HttpError';
  }
}

export function setAccessToken(token: string | null): void { accessToken = token; }
export function getAccessToken(): string | null { return accessToken; }

async function refreshSession(): Promise<boolean> {
  if (!refreshInFlight) {
    refreshInFlight = fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    }).then(async (response) => {
      if (!response.ok) return false;
      const payload: { access_token?: string; refresh_token?: string; tokens?: { access_token: string; refresh_token?: string } } = await response.json();
      const token = payload.access_token ?? payload.tokens?.access_token;
      const nextRefresh = payload.refresh_token ?? payload.tokens?.refresh_token;
      if (nextRefresh) refreshToken = nextRefresh;
      if (!token) return false;
      setAccessToken(token);
      return true;
    }).catch(() => false).finally(() => { refreshInFlight = null; });
  }
  return refreshInFlight;
}

export async function apiFetch<T>(path: string, init: RequestInit = {}, retry = true): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`);
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, { ...init, headers });
  } catch {
    throw new HttpError(0, 'No pudimos conectarnos. Revisa tu conexión e inténtalo nuevamente.');
  }
  if (response.status === 401 && retry && path !== '/auth/login' && path !== '/auth/refresh') {
    if (await refreshSession()) return apiFetch<T>(path, init, false);
    setAccessToken(null);
    window.dispatchEvent(new CustomEvent('superapp:session-expired'));
    throw new HttpError(401, 'Tu sesión terminó. Inicia sesión nuevamente.');
  }
  if (!response.ok) throw new HttpError(response.status, 'No fue posible completar la solicitud.');
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
