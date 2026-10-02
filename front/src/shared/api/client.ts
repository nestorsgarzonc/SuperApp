const BASE = '/api/v1';

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

export type Role = 'RESIDENT' | 'GUARD' | 'ADMIN';

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
};

interface Envelope<T> {
  success: boolean;
  data?: T;
  error?: { code?: string; message?: string; details?: Record<string, unknown> };
}

interface RequestOpts {
  method?: string;
  body?: unknown;
  auth?: boolean;
}

export async function api<T>(path: string, opts: RequestOpts = {}): Promise<T> {
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
  if (!res.ok) {
    const err = json?.error;
    throw new ApiError(err?.message ?? `Error ${res.status}`, res.status, err?.details, err?.code);
  }
  return json?.data as T;
}
