/**
 * Critical flow: AUTHENTICATION (front tier).
 *
 * Components under test (each isolated; the network is replaced by MSW handlers
 * declared per test, so no backend is involved):
 *   - loginSchema / passwordSchema   (features/auth/schemas/authSchemas.ts)
 *   - LoginPage                      (features/auth/pages/LoginPage.tsx)
 *   - login()                        (features/auth/api/authApi.ts)
 *   - AuthProvider.signIn            (features/auth/components/AuthContext.tsx)
 *   - api() session handling         (shared/api/client.ts + lib/http.ts)
 *
 * Every test title starts with its test-case ID (TC-AUTH-xx).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { http, HttpResponse } from 'msw';
import { server } from '../server';
import { loginSchema, passwordSchema } from '../../features/auth/schemas/authSchemas';
import { LoginPage } from '../../features/auth/pages/LoginPage';
import { AuthProvider, useAuth } from '../../features/auth/components/AuthContext';
import { login } from '../../features/auth/api/authApi';
import { api, getSession, setApiSession, type Session } from '../../shared/api/client';
import { getAccessToken, getRefreshToken, setAccessToken, setRefreshToken } from '../../lib/http';

const API = '/api/v1';
const okLogin = () =>
  HttpResponse.json({ user: { id: 'u1', full_name: 'Ana García', email: 'ana@demo.test' }, tokens: { access_token: 'access-1', refresh_token: 'refresh-1' } });
const membership = (id: string, role = 'RESIDENT') => ({ complex_id: id, complex_name: `Conjunto ${id}`, role });

beforeEach(() => {
  setAccessToken(null);
  setRefreshToken(null);
  setApiSession(null);
});
afterEach(cleanup);

// ── Input validation: equivalence partitioning + boundary values ────────────

describe('loginSchema / passwordSchema', () => {
  it.each([
    // [id, email, password, valid]
    ['TC-AUTH-01a valid partition', 'ana@demo.test', 'x', true],
    ['TC-AUTH-01b email with surrounding spaces is trimmed', '  ana@demo.test  ', 'x', true],
    ['TC-AUTH-01c malformed email', 'ana@', 'x', false],
    ['TC-AUTH-01d empty password (length 0, below min=1)', 'ana@demo.test', '', false],
  ])('%s', (_id, email, password, valid) => {
    expect(loginSchema.safeParse({ email, password, remember: false }).success).toBe(valid);
  });

  it.each([
    // BVA on the minimum length (8) with every other rule satisfied
    ['TC-AUTH-02a 7 chars (min-1)', 'Abcde1!', false],
    ['TC-AUTH-02b 8 chars (min)', 'Abcdef1!', true],
    ['TC-AUTH-02c 9 chars (min+1)', 'Abcdefg1!', true],
  ])('%s', (_id, password, valid) => {
    expect(passwordSchema.safeParse({ password, confirmPassword: password }).success).toBe(valid);
  });

  // Decision table: each rule of the password policy is violated alone.
  it.each([
    ['TC-AUTH-03a no uppercase', 'abcdef1!', 'abcdef1!', 'Incluye al menos una mayúscula.'],
    ['TC-AUTH-03b no digit', 'Abcdefg!', 'Abcdefg!', 'Incluye al menos un número.'],
    ['TC-AUTH-03c no symbol', 'Abcdefg1', 'Abcdefg1', 'Incluye al menos un símbolo.'],
    ['TC-AUTH-03d confirmation differs', 'Abcdef1!', 'Abcdef1?', 'Las contraseñas no coinciden.'],
  ])('%s', (_id, password, confirmPassword, message) => {
    const r = passwordSchema.safeParse({ password, confirmPassword });
    expect(r.success).toBe(false);
    expect(r.error?.issues.map((i) => i.message)).toEqual([message]);
  });
});

// ── LoginPage: exceptional responses of the login endpoint ───────────────────

function renderLogin() {
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/account-blocked" element={<p>Cuenta bloqueada</p>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  );
}
function submitLogin(email = 'ana@demo.test', password = 'whatever') {
  fireEvent.change(screen.getByLabelText('Correo electrónico'), { target: { value: email } });
  fireEvent.change(screen.getByLabelText('Contraseña'), { target: { value: password } });
  fireEvent.click(screen.getByRole('button', { name: /iniciar sesión/i }));
}

describe('LoginPage', () => {
  it('TC-AUTH-04 a 401 shows one generic message (no user enumeration) and stores no token', async () => {
    server.use(http.post(`${API}/auth/login`, () => HttpResponse.json({ code: 'invalid_credentials' }, { status: 401 })));
    renderLogin();
    submitLogin();
    expect(await screen.findByRole('alert')).toHaveTextContent('No pudimos validar tus credenciales');
    expect(screen.queryByText(/no existe|contraseña incorrecta/i)).not.toBeInTheDocument();
    expect(getAccessToken()).toBeNull();
  });

  it('TC-AUTH-05 a 423 (account blocked) navigates to the blocked page', async () => {
    server.use(http.post(`${API}/auth/login`, () => HttpResponse.json({ code: 'account_blocked' }, { status: 423 })));
    renderLogin();
    submitLogin();
    expect(await screen.findByText('Cuenta bloqueada')).toBeInTheDocument();
  });

  it('TC-AUTH-06 a network failure shows the connection message with a retry action', async () => {
    server.use(http.post(`${API}/auth/login`, () => HttpResponse.error()));
    renderLogin();
    submitLogin();
    expect(await screen.findByRole('alert')).toHaveTextContent('No pudimos conectarnos');
    expect(screen.getByRole('button', { name: 'Intentar nuevamente' })).toBeInTheDocument();
  });

  it('TC-AUTH-07 an invalid email is rejected client-side without calling the API', async () => {
    const spy = vi.fn();
    server.use(http.post(`${API}/auth/login`, () => { spy(); return okLogin(); }));
    renderLogin();
    submitLogin('no-es-un-correo', 'x');
    expect(await screen.findByText('Ingresa un correo electrónico válido.')).toBeInTheDocument();
    expect(spy).not.toHaveBeenCalled();
  });
});

// ── login(): exceptional behavior ────────────────────────────────────────────

describe('login()', () => {
  it('TC-AUTH-08 normalizes the email and discards the tokens when memberships cannot be loaded', async () => {
    let sentEmail = '';
    server.use(
      http.post(`${API}/auth/login`, async ({ request }) => {
        sentEmail = ((await request.json()) as { email: string }).email;
        return okLogin();
      }),
      http.get(`${API}/me/memberships`, () => HttpResponse.json({ success: false }, { status: 500 })),
    );
    await expect(login('  ANA@Demo.Test ', 'secret', false)).rejects.toMatchObject({ status: 500 });
    expect(sentEmail).toBe('ana@demo.test');
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
  });
});

// ── AuthProvider.signIn: state transitions after a successful login ─────────
//
//   anonymous --login, 0 memberships--> /unauthorized (no identity, token cleared)
//   anonymous --login, 1 membership --> /dashboard     (complex auto-selected)
//   anonymous --login, n>1          --> /select-complex (no active complex yet)

function Probe() {
  const { signIn, identity, activeComplex } = useAuth();
  const location = useLocation();
  return (
    <>
      <button onClick={() => void signIn('ana@demo.test', 'secret', false)}>entrar</button>
      <output data-testid="path">{location.pathname}</output>
      <output data-testid="identity">{identity ? identity.name : 'none'}</output>
      <output data-testid="active">{activeComplex?.id ?? 'none'}</output>
    </>
  );
}

describe('AuthProvider.signIn', () => {
  it.each([
    ['TC-AUTH-09a no memberships', [], '/unauthorized', 'none', 'none'],
    ['TC-AUTH-09b one membership', [membership('c1')], '/dashboard', 'Ana García', 'c1'],
    ['TC-AUTH-09c two memberships', [membership('c1'), membership('c2', 'GUARD')], '/select-complex', 'Ana García', 'none'],
  ])('%s', async (_id, memberships, path, identity, active) => {
    server.use(
      http.post(`${API}/auth/login`, okLogin),
      http.get(`${API}/me/memberships`, () => HttpResponse.json({ success: true, data: { user_id: 'u1', memberships } })),
    );
    render(<MemoryRouter initialEntries={['/login']}><AuthProvider><Probe /></AuthProvider></MemoryRouter>);
    fireEvent.click(screen.getByRole('button', { name: 'entrar' }));
    await waitFor(() => expect(screen.getByTestId('path')).toHaveTextContent(path));
    expect(screen.getByTestId('identity')).toHaveTextContent(identity);
    expect(screen.getByTestId('active')).toHaveTextContent(active);
    if (memberships.length === 0) expect(getAccessToken()).toBeNull();
  });
});

// ── Session expiry: authenticated → expired ──────────────────────────────────

const session: Session = { token: 'expired', complexId: 'c1', complexName: 'Los Pinos', unitId: 'u-101', userId: 'u1', role: 'RESIDENT', name: 'Ana', subtitle: 'Residente' };

describe('api() session expiry', () => {
  it('TC-AUTH-10 a 401 whose refresh fails clears the session and emits session-expired', async () => {
    server.use(
      http.get(`${API}/private`, () => HttpResponse.json({ success: false, error: { message: 'expired' } }, { status: 401 })),
      http.post(`${API}/auth/refresh`, () => HttpResponse.json({}, { status: 401 })),
    );
    const expired = vi.fn();
    window.addEventListener('superapp:session-expired', expired);
    setRefreshToken('refresh-1');
    setApiSession(session);

    await expect(api('/private')).rejects.toMatchObject({ status: 401 });
    expect(getSession()).toBeNull();
    expect(expired).toHaveBeenCalledTimes(1);
    window.removeEventListener('superapp:session-expired', expired);
  });

  it('TC-AUTH-11 concurrent 401s share a single refresh call (refresh token is single-use)', async () => {
    let refreshCalls = 0;
    server.use(
      http.get(`${API}/a`, ({ request }) =>
        request.headers.get('authorization') === 'Bearer fresh' ? HttpResponse.json({ success: true, data: 'a' }) : HttpResponse.json({}, { status: 401 })),
      http.get(`${API}/b`, ({ request }) =>
        request.headers.get('authorization') === 'Bearer fresh' ? HttpResponse.json({ success: true, data: 'b' }) : HttpResponse.json({}, { status: 401 })),
      http.post(`${API}/auth/refresh`, () => {
        refreshCalls += 1;
        return HttpResponse.json({ access_token: 'fresh', refresh_token: 'refresh-2' });
      }),
    );
    setRefreshToken('refresh-1');
    setApiSession(session);

    await expect(Promise.all([api('/a'), api('/b')])).resolves.toEqual(['a', 'b']);
    expect(refreshCalls).toBe(1);
    expect(getRefreshToken()).toBe('refresh-2');
  });
});
