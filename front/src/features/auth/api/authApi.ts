import { apiFetch, getRefreshToken, HttpError, setAccessToken, setRefreshToken } from '../../../lib/http';
import { permissionsForRole, type DemoRole } from '../demoDirectory';

export interface Complex { id: string; name: string; address: string; status: 'Activo' | 'Pendiente'; role?: string; permissions?: string[]; unitId?: string; unitName?: string; }
export interface AuthIdentity { id: string; name: string; email: string; roles: string[]; permissions: string[]; complexes: Complex[]; activeComplex?: Complex; token?: string; }
interface LoginResponse { user: { id: string; name?: string; full_name?: string; email: string; roles?: string[]; permissions?: string[]; complexes?: Complex[] }; tokens: { access_token: string; refresh_token?: string }; }

/** Membresías reales (control-de-accesos, vía gateway): conjunto, rol y unidad. */
interface MembershipsResponse {
  success?: boolean;
  data?: { user_id: string; name?: string; memberships: Array<{ complex_id: string; complex_name: string; role: string; unit_id?: string; unit_name?: string }> };
}

export async function login(email: string, password: string, _remember: boolean): Promise<AuthIdentity> {
  const response = await apiFetch<LoginResponse>('/auth/login', {
    method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase(), password, device_name: 'Navegador web' }),
  });
  setAccessToken(response.tokens.access_token);
  setRefreshToken(response.tokens.refresh_token ?? null);
  let memberships: NonNullable<MembershipsResponse['data']>;
  try {
    const res = await apiFetch<MembershipsResponse>('/me/memberships');
    if (!res.data) throw new HttpError(502);
    memberships = res.data;
  } catch (error) {
    // Sin membresías no hay sesión utilizable: no dejar tokens a medias.
    setAccessToken(null);
    setRefreshToken(null);
    throw error;
  }
  const complexes: Complex[] = memberships.memberships.map((m) => ({
    id: m.complex_id, name: m.complex_name, address: '', status: 'Activo' as const,
    role: m.role, permissions: permissionsForRole[m.role as DemoRole] ?? [],
    ...(m.unit_id ? { unitId: m.unit_id } : {}),
    ...(m.unit_name ? { unitName: m.unit_name } : {}),
  }));
  return {
    id: response.user.id,
    name: response.user.name ?? response.user.full_name ?? memberships.name ?? 'Usuario',
    email: response.user.email,
    roles: [...new Set(complexes.map((c) => c.role as string))],
    permissions: [],
    complexes,
    token: response.tokens.access_token,
  };
}

export async function requestPasswordReset(email: string): Promise<void> {
  await apiFetch<void>('/auth/forgot-password', { method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase() }) });
}
export async function activateComplex(complex: Complex): Promise<Complex> {
  return complex; // DEMO
}
export async function resetPassword(token: string, password: string): Promise<void> {
  await apiFetch<void>('/auth/reset-password', { method: 'POST', body: JSON.stringify({ token, password }) });
}
export async function logout(all = false): Promise<void> {
  try {
    await apiFetch<void>(all ? '/auth/logout-all' : '/auth/logout', { method: 'POST', body: JSON.stringify({ refresh_token: getRefreshToken() }) });
  } finally {
    setAccessToken(null);
    setRefreshToken(null);
  }
}
