/**
 * Critical flow: ACCESS CONTROL (front tier).
 *
 * Components under test:
 *   - RequireRole       (modules/control-de-accesos/routes.tsx)  role + permission gate per page
 *   - ProtectedRoute    (features/auth/routes/guards.tsx)        authentication gate
 *   - permissionsForRole (features/auth/demoDirectory.ts)        role → permission matrix
 *   - api() headers     (shared/api/client.ts)                   tenant scoping of every request
 *
 * Isolation: both auth contexts are replaced by STUBS (vi.mock) whose value
 * each test sets, so the guards are exercised without a login flow. The
 * network is MSW. Each test title starts with its ID (TC-AC-xx).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { http, HttpResponse } from 'msw';
import { server } from '../server';
import type { Role, Session } from '../../shared/api/client';

const auth = vi.hoisted(() => ({
  session: null as Session | null,
  identity: null as { roles: string[]; permissions: string[] } | null,
}));
vi.mock('../../shared/auth/AuthContext', () => ({ useAuth: () => ({ session: auth.session }) }));
vi.mock('../../features/auth/components/AuthContext', () => ({ useAuth: () => ({ identity: auth.identity }) }));

import { RequireRole } from '../../modules/control-de-accesos/routes';
import { ProtectedRoute } from '../../features/auth/routes/guards';
import { permissionsForRole } from '../../features/auth/demoDirectory';
import { api, ApiError, getSession, setApiSession } from '../../shared/api/client';

const GATE: Role[] = ['GUARD', 'ADMIN'];
const sessionAs = (role: Role): Session => ({
  token: 'tok-1', complexId: '00000000-0000-0000-0000-000000000001', complexName: 'Los Pinos',
  unitId: role === 'RESIDENT' ? 'unit-101' : '', userId: 'u1', role, name: 'Persona', subtitle: role,
});

function Where() {
  const loc = useLocation();
  return <p data-testid="where">{loc.pathname}{(loc.state as { from?: string } | null)?.from ? ` from=${(loc.state as { from: string }).from}` : ''}</p>;
}

beforeEach(() => {
  auth.session = null;
  auth.identity = null;
  setApiSession(null);
});
afterEach(cleanup);

// ── RequireRole: decision table ──────────────────────────────────────────────
//
//   rule | session | role ∈ allowed | has permission | outcome
//   R1   |   no    |       -        |       -        | /unauthorized
//   R2   |   yes   |      no        |      yes       | /unauthorized
//   R3   |   yes   |      yes       |      no        | /unauthorized
//   R4   |   yes   |      yes       |      yes       | page rendered

describe('RequireRole (gate pages: GUARD/ADMIN + access.pass.validate)', () => {
  it.each([
    ['TC-AC-01 R1 no session', null, [], false],
    ['TC-AC-02 R2 resident on a gate page', 'RESIDENT', ['access.pass.validate'], false],
    ['TC-AC-03 R3 guard without the permission', 'GUARD', ['access.history.read'], false],
    ['TC-AC-04 R4 guard with the permission', 'GUARD', ['access.pass.validate'], true],
    ['TC-AC-05 R4 admin with the permission', 'ADMIN', permissionsForRole.ADMIN, true],
  ] as const)('%s', (_id, role, permissions, allowed) => {
    auth.session = role ? sessionAs(role) : null;
    auth.identity = role ? { roles: [role], permissions: [...permissions] } : null;
    render(
      <MemoryRouter initialEntries={['/porteria/validar']}>
        <Routes>
          <Route path="/porteria/validar" element={<RequireRole roles={GATE} permission="access.pass.validate"><p>Validar pase</p></RequireRole>} />
          <Route path="/unauthorized" element={<p>Acceso no autorizado</p>} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.queryByText('Validar pase') !== null).toBe(allowed);
    expect(screen.queryByText('Acceso no autorizado') !== null).toBe(!allowed);
  });
});

// ── ProtectedRoute: authentication gate ──────────────────────────────────────

describe('ProtectedRoute', () => {
  const renderAt = (props: { permission?: string; roles?: string[] }) =>
    render(
      <MemoryRouter initialEntries={['/admin']}>
        <Routes>
          <Route element={<ProtectedRoute {...props} />}><Route path="/admin" element={<p>Contenido protegido</p>} /></Route>
          <Route path="/login" element={<Where />} />
          <Route path="/unauthorized" element={<Where />} />
        </Routes>
      </MemoryRouter>,
    );

  it('TC-AC-06 an anonymous visitor goes to /login and remembers where they were going', () => {
    renderAt({});
    expect(screen.getByTestId('where')).toHaveTextContent('/login from=/admin');
  });

  it.each([
    ['TC-AC-07a missing permission', { permission: 'access.platform.manage' }, { roles: ['ADMIN'], permissions: ['access.home.read'] }, '/unauthorized'],
    ['TC-AC-07b role not in the list', { roles: ['ADMIN'] }, { roles: ['RESIDENT'], permissions: ['access.home.read'] }, '/unauthorized'],
    ['TC-AC-07c permission and role match', { permission: 'access.home.read', roles: ['RESIDENT'] }, { roles: ['RESIDENT'], permissions: ['access.home.read'] }, null],
  ])('%s', (_id, props, identity, redirect) => {
    auth.identity = identity;
    renderAt(props);
    if (redirect) expect(screen.getByTestId('where')).toHaveTextContent(redirect);
    else expect(screen.getByText('Contenido protegido')).toBeInTheDocument();
  });
});

// ── Role → permission matrix (equivalence classes = roles) ───────────────────

describe('permissionsForRole', () => {
  it('TC-AC-08 separation of duties: residents create passes, guards validate them, admins do both', () => {
    const can = (role: keyof typeof permissionsForRole, p: string) => permissionsForRole[role].includes(p);
    expect([can('RESIDENT', 'access.authorizations.create'), can('RESIDENT', 'access.pass.validate')]).toEqual([true, false]);
    expect([can('GUARD', 'access.authorizations.create'), can('GUARD', 'access.pass.validate')]).toEqual([false, true]);
    expect(permissionsForRole.RESIDENT.every((p) => can('ADMIN', p))).toBe(true);
    expect(permissionsForRole.GUARD.every((p) => can('ADMIN', p))).toBe(true);
  });
});

// ── api(): every request is scoped to the session's complex ─────────────────

describe('api() tenant headers', () => {
  it('TC-AC-09 sends Bearer token + X-Complex-ID with a session, and neither for auth:false', async () => {
    const seen: Array<{ auth: string | null; complex: string | null }> = [];
    server.use(http.get('/api/v1/echo', ({ request }) => {
      seen.push({ auth: request.headers.get('authorization'), complex: request.headers.get('x-complex-id') });
      return HttpResponse.json({ success: true, data: null });
    }));
    setApiSession(sessionAs('GUARD'));
    await api('/echo');
    await api('/echo', { auth: false });

    expect(seen[0]).toEqual({ auth: 'Bearer tok-1', complex: '00000000-0000-0000-0000-000000000001' });
    expect(seen[1]).toEqual({ auth: null, complex: null });
  });

  it('TC-AC-10 a 403 surfaces as ApiError with its code and does NOT end the session', async () => {
    server.use(http.get('/api/v1/forbidden', () =>
      HttpResponse.json({ success: false, error: { code: 'AUTH_403', message: 'your role does not have access to this operation' } }, { status: 403 })));
    setApiSession(sessionAs('RESIDENT'));

    const err = await api('/forbidden').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 403, code: 'AUTH_403' });
    expect(getSession()?.role).toBe('RESIDENT');
  });
});
