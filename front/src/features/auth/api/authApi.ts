import { apiFetch, setAccessToken } from '../../../lib/http';

export interface Complex { id: string; name: string; address: string; status: 'Activo' | 'Pendiente'; role?: string; permissions?: string[]; unitId?: string; unitName?: string; }
export interface AuthIdentity { id: string; name: string; email: string; roles: string[]; permissions: string[]; complexes: Complex[]; activeComplex?: Complex; }
interface LoginResponse { user: { id: string; name?: string; full_name?: string; email: string; roles?: string[]; permissions?: string[]; complexes?: Complex[] }; tokens: { access_token: string; refresh_token?: string }; }

// El ms de auth solo identifica a la persona. Los conjuntos y el rol de cada uno
// son de control-de-accesos (GET /me/memberships), que es quien los administra.
interface MembershipRow { complex_id: string; complex_name: string; role: string; unit_id?: string; unit_name?: string }
interface MembershipsResponse { data: { user_id: string; name?: string; memberships: MembershipRow[] } }

const ROLE_PRIORITY: Record<string, number> = { ADMIN: 3, GUARD: 2, RESIDENT: 1 };
const GATE_PERMISSIONS = ['access.home.read', 'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read'];
const PERMISSIONS_BY_ROLE: Record<string, string[]> = {
  RESIDENT: ['access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read'],
  GUARD: GATE_PERMISSIONS,
  ADMIN: GATE_PERMISSIONS,
};

/** Un conjunto por membresía; si hay varias en el mismo conjunto gana el rol más alto. */
export function complexesFromMemberships(rows: MembershipRow[]): Complex[] {
  const byComplex = new Map<string, MembershipRow>();
  for (const row of rows) {
    const current = byComplex.get(row.complex_id);
    if (!current || (ROLE_PRIORITY[row.role] ?? 0) > (ROLE_PRIORITY[current.role] ?? 0)) byComplex.set(row.complex_id, row);
  }
  return [...byComplex.values()].map((row) => ({
    id: row.complex_id, name: row.complex_name, address: '', status: 'Activo' as const, role: row.role,
    permissions: PERMISSIONS_BY_ROLE[row.role] ?? [],
    ...(row.unit_id ? { unitId: row.unit_id } : {}), ...(row.unit_name ? { unitName: row.unit_name } : {}),
  }));
}

async function loadComplexes(): Promise<Complex[]> {
  try {
    const response = await apiFetch<MembershipsResponse>('/me/memberships');
    return complexesFromMemberships(response.data.memberships);
  } catch {
    return [];
  }
}

export async function login(email: string, password: string, remember: boolean): Promise<AuthIdentity> {
  const response = await apiFetch<LoginResponse>('/auth/login', {
    method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase(), password, remember, device_name: 'Navegador web' }),
  });
  setAccessToken(response.tokens.access_token);
  const complexes = response.user.complexes ?? await loadComplexes();
  const roles = response.user.roles ?? [...new Set(complexes.map(({ role }) => role ?? 'RESIDENT'))];
  // Con un solo conjunto los permisos son los de ese conjunto; con varios, la unión hasta que elija uno.
  const permissions = response.user.permissions ?? [...new Set(complexes.flatMap((complex) => complex.permissions ?? []))];
  return {
    id: response.user.id, name: response.user.name ?? response.user.full_name ?? 'Usuario', email: response.user.email,
    roles: roles.length ? roles : ['residente'], permissions: permissions.length ? permissions : ['home.read'], complexes,
  };
}

export async function requestPasswordReset(email: string): Promise<void> {
  await apiFetch<void>('/auth/forgot-password', { method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase() }) });
}
export async function activateComplex(complex: Complex): Promise<Complex> {
  // La membresía y el rol ya vienen verificados por control-de-accesos, que además
  // los vuelve a comprobar en cada petición (X-Complex-ID): no hay nada que activar en auth.
  if (complex.permissions) return complex;
  const response = await apiFetch<{ active_complex: { id: string; permissions?: string[]; role?: string }; access_token?: string }>('/auth/select-complex', {
    method: 'POST', body: JSON.stringify({ complex_id: complex.id }),
  });
  if (response.active_complex.id !== complex.id) throw new Error('El conjunto no coincide con la identidad autenticada.');
  if (response.access_token) setAccessToken(response.access_token);
  return { ...complex, role: response.active_complex.role ?? complex.role, permissions: response.active_complex.permissions ?? complex.permissions };
}
export async function resetPassword(token: string, password: string): Promise<void> {
  await apiFetch<void>('/auth/reset-password', { method: 'POST', body: JSON.stringify({ token, password }) });
}
export async function logout(all = false): Promise<void> {
  await apiFetch<void>(all ? '/auth/logout-all' : '/auth/logout', { method: 'POST' });
  setAccessToken(null);
}
