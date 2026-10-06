import { apiFetch, setAccessToken } from '../../../lib/http';

export interface Complex { id: string; name: string; address: string; status: 'Activo' | 'Pendiente'; role?: string; permissions?: string[]; unitId?: string; unitName?: string; }
export interface AuthIdentity { id: string; name: string; email: string; roles: string[]; permissions: string[]; complexes: Complex[]; activeComplex?: Complex; }
interface LoginResponse { user: { id: string; name?: string; full_name?: string; email: string; roles?: string[]; permissions?: string[]; complexes?: Complex[] }; tokens: { access_token: string; refresh_token?: string }; }

export async function login(email: string, password: string, remember: boolean): Promise<AuthIdentity> {
  const response = await apiFetch<LoginResponse>('/auth/login', {
    method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase(), password, remember, device_name: 'Navegador web' }),
  });
  setAccessToken(response.tokens.access_token);
  return {
    id: response.user.id, name: response.user.name ?? response.user.full_name ?? 'Usuario', email: response.user.email,
    roles: response.user.roles ?? ['residente'], permissions: response.user.permissions ?? ['home.read'], complexes: response.user.complexes ?? [],
  };
}

export async function requestPasswordReset(email: string): Promise<void> {
  await apiFetch<void>('/auth/forgot-password', { method: 'POST', body: JSON.stringify({ email: email.trim().toLowerCase() }) });
}
export async function activateComplex(complex: Complex): Promise<Complex> {
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
