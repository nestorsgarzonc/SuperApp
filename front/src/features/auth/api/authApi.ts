import { apiFetch, getRefreshToken, setAccessToken, setRefreshToken } from '../../../lib/http';
import { demoMemberships, permissionsForRole } from '../demoDirectory';

export interface Complex { id: string; name: string; address: string; status: 'Activo' | 'Pendiente'; role?: string; permissions?: string[]; unitId?: string; unitName?: string; }
export interface AuthIdentity { id: string; name: string; email: string; roles: string[]; permissions: string[]; complexes: Complex[]; activeComplex?: Complex; token?: string; }
interface LoginResponse { user: { id: string; name?: string; full_name?: string; email: string; roles?: string[]; permissions?: string[]; complexes?: Complex[] }; tokens: { access_token: string; refresh_token?: string }; }

export async function login(email: string, password: string, _remember: boolean): Promise<AuthIdentity> {
  const response = await apiFetch<LoginResponse>('/auth/login', {
    method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase(), password, device_name: 'Navegador web' }),
  });
  setAccessToken(response.tokens.access_token);
  setRefreshToken(response.tokens.refresh_token ?? null);
  const userEmail = response.user.email.toLowerCase();
  const complexes: Complex[] = (demoMemberships[userEmail] ?? []).map((m) => ({
    id: m.id, name: m.name, address: '', status: 'Activo' as const,
    role: m.role, permissions: permissionsForRole[m.role],
  }));
  return {
    id: response.user.id,
    name: response.user.name ?? response.user.full_name ?? 'Usuario',
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
