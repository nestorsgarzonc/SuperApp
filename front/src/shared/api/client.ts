import { getAccessToken, refreshSession } from '../../lib/http';

const BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public details?: Record<string, unknown>,
    public code?: string,
  ) {
    super(message);
  }
}

export type Role = 'RESIDENT' | 'OWNER' | 'GUARD' | 'ADMIN';

/** Una persona puede tener varios conjuntos y un rol distinto en cada uno. */
export interface Membership {
  complexId: string;
  complexName: string;
  role: Role;
  unitId?: string;
  unitName?: string;
}

/**
 * Sesión que consumen los módulos. Quién la crea (login real del módulo auth,
 * o la entrada de pruebas de control-de-accesos) es indiferente para el resto.
 */
export interface Session {
  token: string;
  complexId: string;
  complexName: string;
  unitId: string;
  unitName?: string;
  userId: string;
  role: Role;
  name: string;
  subtitle: string;
  /** Todos los conjuntos de la persona (para cambiar de conjunto). */
  memberships?: Membership[];
}

let session: Session | null = null;

export const getSession = () => session;
export const setApiSession = (s: Session | null) => {
  session = s;
  // Solo dev: las pruebas e2e leen la sesión en memoria (no existe en el build de producción).
  if (import.meta.env.DEV) (window as unknown as { __superappSession?: Session | null }).__superappSession = s;
};

// El refresh es el de lib/http (uno solo para toda la app); aquí solo se actualiza la sesión.
async function refreshAccessToken(): Promise<string | null> {
  if (!(await refreshSession())) return null;
  const token = getAccessToken();
  if (!token || !session) return null;
  setApiSession({ ...session, token });
  window.dispatchEvent(new CustomEvent('superapp:access-token-refreshed', { detail: { token } }));
  return token;
}

interface Envelope<T> {
  success: boolean;
  data?: T;
  error?: { code?: string; message?: string; details?: Record<string, unknown> };
}

interface RequestOpts {
  method?: string;
  body?: unknown;
  auth?: boolean;
  /**
   * Llamada accesoria (p. ej. notificaciones): si responde 401 y el refresh no
   * lo arregla, falla sola sin cerrar la sesión de la app.
   */
  optional?: boolean;
}

export async function api<T>(path: string, opts: RequestOpts = {}, retry = true): Promise<T> {
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
  if (opts.auth !== false && session) {
    headers.Authorization = `Bearer ${session.token}`;
    headers['X-Complex-ID'] = session.complexId;
  }

  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
  } catch {
    throw new ApiError('Sin conexión con el servidor', 0);
  }

  const json = (await res.json().catch(() => null)) as Envelope<T> | null;
  if (res.status === 401 && opts.auth !== false && retry && path !== '/auth/refresh') {
    const token = await refreshAccessToken();
    if (token) return api<T>(path, opts, false);
    if (!opts.optional) {
      setApiSession(null);
      window.dispatchEvent(new CustomEvent('superapp:session-expired'));
    }
  }
  if (!res.ok) {
    const err = json?.error;
    throw new ApiError(err?.message ?? `Error ${res.status}`, res.status, err?.details, err?.code);
  }
  return json?.data as T;
}
