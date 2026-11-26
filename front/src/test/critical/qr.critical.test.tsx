/**
 * Critical flow: QR PASS GENERATION AND VALIDATION (front tier).
 *
 * Components under test:
 *   - grantView / rejectText   (modules/control-de-accesos/types.ts)   pure logic
 *   - useQrDataUrl             (modules/control-de-accesos/useQrDataUrl.ts)
 *   - NewAuthorizationPage     (modules/control-de-accesos/pages/resident.tsx)  pass generation form
 *   - ValidatePassPage         (modules/control-de-accesos/pages/guard.tsx)     gate validation
 *
 * Isolation:
 *   - session context  → STUB (vi.mock of shared/auth/AuthContext)
 *   - backend          → MSW handlers per test (FAKE ms responses)
 *   - clock            → only `Date` is faked and pinned to 2026-11-21 10:00 local,
 *                        so default windows and "already passed" checks are deterministic;
 *                        real timers keep MSW and React Testing Library working.
 * Each test title starts with its ID (TC-QR-xx).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { http, HttpResponse } from 'msw';
import { server } from '../server';
import type { Session } from '../../shared/api/client';

const ctx = vi.hoisted(() => ({ session: null as Session | null }));
vi.mock('../../shared/auth/AuthContext', () => ({
  useAuth: () => ({ session: ctx.session, profile: ctx.session && { role: ctx.session.role, name: ctx.session.name, subtitle: '' } }),
}));

import { grantView, rejectText, type AccessGrant } from '../../modules/control-de-accesos/types';
import { useQrDataUrl } from '../../modules/control-de-accesos/useQrDataUrl';
import { NewAuthorizationPage } from '../../modules/control-de-accesos/pages/resident';
import { ValidatePassPage } from '../../modules/control-de-accesos/pages/guard';

const API = '/api/v1';
const C1 = '00000000-0000-0000-0000-000000000001';
const UNIT = '10000000-0000-0000-0000-000000000101';
const NOW = new Date(2026, 10, 21, 10, 0, 0); // local time
const resident: Session = { token: 't', complexId: C1, complexName: 'Los Pinos', unitId: UNIT, unitName: 'A-101', userId: 'u1', role: 'RESIDENT', name: 'Ana', subtitle: '' };
const guard: Session = { ...resident, unitId: '', role: 'GUARD', name: 'Luis' };

const qrSystemUp = http.get(`${API}/access/qr-system`, () => HttpResponse.json({ success: true, data: { available: true, state: 'CLOSED' } }));
const qrSystemDown = http.get(`${API}/access/qr-system`, () =>
  HttpResponse.json({ success: true, data: { available: false, state: 'OPEN', retry_after_seconds: 8, fallback: 'PRE_AUTH' } }));
const breakerOpen = () => HttpResponse.json({ success: false, error: { code: 'QR_001', message: 'QR system unavailable' } }, { status: 503 });

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(NOW);
  server.use(qrSystemUp);
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// ── grantView: state of a pass as the resident sees it (decision table + BVA) ─

const grant = (over: Partial<AccessGrant>): AccessGrant => ({
  id: 'g1', complex_id: C1, unit_id: UNIT, created_by: 'u1', method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Juan',
  status: 'ACTIVE', created_at: NOW.toISOString(), single_use: true, ...over,
});
const at = (msFromNow: number) => new Date(NOW.getTime() + msFromNow).toISOString();

describe('grantView', () => {
  it.each([
    ['TC-QR-01a CANCELLED wins over an open window', grant({ status: 'CANCELLED', valid_until: at(3600e3) }), 'cancelled'],
    ['TC-QR-01b USED is shown as expired', grant({ status: 'USED', valid_until: at(3600e3) }), 'expired'],
    ['TC-QR-01c EXPIRED status', grant({ status: 'EXPIRED' }), 'expired'],
    ['TC-QR-01d ACTIVE, window not started', grant({ valid_from: at(3600e3), valid_until: at(7200e3) }), 'upcoming'],
    ['TC-QR-01e ACTIVE, inside the window', grant({ valid_from: at(-3600e3), valid_until: at(3600e3) }), 'active'],
    ['TC-QR-01f ACTIVE, window ended', grant({ valid_from: at(-7200e3), valid_until: at(-3600e3) }), 'expired'],
  ])('%s', (_id, g, view) => {
    expect(grantView(g, NOW.getTime())).toBe(view);
  });

  it.each([
    // boundaries of valid_until (expired only when strictly before now)
    ['TC-QR-02a valid_until = now - 1ms', { valid_until: at(-1) }, 'expired'],
    ['TC-QR-02b valid_until = now', { valid_until: at(0) }, 'active'],
    ['TC-QR-02c valid_until = now + 1ms', { valid_until: at(1) }, 'active'],
    // boundaries of valid_from (upcoming only when strictly after now)
    ['TC-QR-02d valid_from = now', { valid_from: at(0), valid_until: at(3600e3) }, 'active'],
    ['TC-QR-02e valid_from = now + 1ms', { valid_from: at(1), valid_until: at(3600e3) }, 'upcoming'],
  ])('%s', (_id, over, view) => {
    expect(grantView(grant(over), NOW.getTime())).toBe(view);
  });
});

// ── rejectText: one message per rejection reason (equivalence classes) ───────

describe('rejectText', () => {
  const when = (iso: string) => `@${iso}`;
  it.each([
    ['NOT_YET_VALID', { valid_from: 'F' }, 'El pase aún no está vigente. Empieza @F.'],
    ['EXPIRED', { valid_until: 'U' }, 'El pase venció @U.'],
    ['ALREADY_USED', {}, 'Este pase ya fue utilizado (es de un solo uso).'],
    ['CANCELLED', {}, 'El residente revocó este pase.'],
    ['NOT_FOUND', {}, 'No se pudo validar el pase. Verifica el código o valídalo por documento.'],
    ['NO_AUTHORIZATION', {}, 'No hay una autorización vigente para este documento.'],
  ] as const)('TC-QR-03 %s', (reason, extra, text) => {
    expect(rejectText({ reason, ...extra }, when)).toBe(text);
  });

  it('TC-QR-03 no reason → null (page falls back to the generic list)', () => {
    expect(rejectText({}, when)).toBeNull();
  });
});

// ── useQrDataUrl: client-side QR image ──────────────────────────────────────

describe('useQrDataUrl', () => {
  it('TC-QR-04 renders a PNG data URL for a code, and none for an empty code', async () => {
    const { result, rerender } = renderHook(({ v }: { v: string | null }) => useQrDataUrl(v), { initialProps: { v: 'SA-7K29-4QX1' as string | null } });
    await waitFor(() => expect(result.current).toMatch(/^data:image\/png;base64,/));
    const first = result.current;

    rerender({ v: 'SA-OTHER-CODE' });
    await waitFor(() => expect(result.current).toMatch(/^data:image\/png;base64,/));
    expect(result.current).not.toBe(first); // a different code → a different image

    rerender({ v: null });
    expect(result.current).toBeNull();
  });
});

// ── NewAuthorizationPage: generating a pass ──────────────────────────────────

function renderNewPass() {
  ctx.session = resident;
  const view = render(
    <MemoryRouter initialEntries={['/autorizaciones/nueva']}>
      <Routes>
        <Route path="/autorizaciones/nueva" element={<NewAuthorizationPage />} />
        <Route path="/autorizaciones/:id" element={<p>Pase creado</p>} />
      </Routes>
    </MemoryRouter>,
  );
  const [start, end] = Array.from(view.container.querySelectorAll<HTMLInputElement>('input[type="time"]'));
  return { start: start!, end: end! };
}
const typeName = (v: string) => fireEvent.change(screen.getByPlaceholderText('Juan Pérez'), { target: { value: v } });
const submit = (label: RegExp = /generar pase/i) => fireEvent.click(screen.getByRole('button', { name: label }));

describe('NewAuthorizationPage', () => {
  it('TC-QR-05 a blank visitor name is rejected and nothing is sent', async () => {
    const posted = vi.fn();
    server.use(http.post(`${API}/access-grants`, () => { posted(); return HttpResponse.json({}); }));
    renderNewPass();
    typeName('   ');
    submit();
    expect(await screen.findByText('Escribe el nombre del visitante')).toBeInTheDocument();
    expect(posted).not.toHaveBeenCalled();
  });

  it.each([
    ['TC-QR-06a end before start', '12:00', '11:59', 'Debe ser posterior a la hora de inicio'],
    ['TC-QR-06b end = start (boundary)', '12:00', '12:00', 'Debe ser posterior a la hora de inicio'],
    ['TC-QR-06c end already passed today', '08:00', '09:59', 'La hora fin ya pasó'],
  ])('%s', async (_id, s, e, message) => {
    const { start, end } = renderNewPass();
    typeName('Juan Pérez');
    fireEvent.change(start, { target: { value: s } });
    fireEvent.change(end, { target: { value: e } });
    submit();
    expect(await screen.findByText(message)).toBeInTheDocument();
  });

  it('TC-QR-07 PRE_AUTH requires the visitor document', async () => {
    renderNewPass();
    fireEvent.click(screen.getByRole('radio', { name: 'Por documento' }));
    typeName('Juan Pérez');
    submit(/autorizar visitante/i);
    expect(await screen.findByText('El guardia validará con este documento')).toBeInTheDocument();
  });

  it('TC-QR-08 a valid QR request (end = start + 1 min) sends the window and opens the new pass', async () => {
    let body: Record<string, unknown> = {};
    server.use(http.post(`${API}/access-grants`, async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>;
      return HttpResponse.json({ success: true, data: grant({ id: 'new-grant' }) }, { status: 201 });
    }));
    const { start, end } = renderNewPass();
    typeName('  Juan Pérez  ');
    fireEvent.change(start, { target: { value: '12:00' } });
    fireEvent.change(end, { target: { value: '12:01' } });
    submit();

    expect(await screen.findByText('Pase creado')).toBeInTheDocument();
    expect(body).toMatchObject({
      unit_id: UNIT, method: 'QR', visitor_type: 'VISITOR', visitor_name: 'Juan Pérez',
      valid_from: new Date(2026, 10, 21, 12, 0).toISOString(),
      valid_until: new Date(2026, 10, 21, 12, 1).toISOString(),
    });
    expect(body).not.toHaveProperty('visitor_id_number');
  });

  it('TC-QR-09 breaker opens while generating (QR_001) → switches to PRE_AUTH and explains', async () => {
    server.use(http.post(`${API}/access-grants`, breakerOpen));
    renderNewPass();
    typeName('Juan Pérez');
    submit();
    expect(await screen.findByText(/El sistema de QR no está disponible/)).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Por documento' })).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByPlaceholderText('1.020.345.678')).toBeInTheDocument();
  });

  it('TC-QR-10 QR system already down → the QR option is disabled from the start', async () => {
    server.use(qrSystemDown);
    renderNewPass();
    await waitFor(() => expect(screen.getByRole('radio', { name: 'Pase con QR' })).toBeDisabled());
    expect(screen.getByRole('radio', { name: 'Por documento' })).toHaveAttribute('aria-checked', 'true');
  });
});

// ── ValidatePassPage: the guard validates a pass ─────────────────────────────

function renderGate() {
  ctx.session = guard;
  render(<MemoryRouter><ValidatePassPage /></MemoryRouter>);
}
const typeCode = (v: string) => fireEvent.change(screen.getByPlaceholderText('SA-7K29-4QX1'), { target: { value: v } });
const validate = () => fireEvent.click(screen.getByRole('button', { name: 'Validar pase' }));

describe('ValidatePassPage', () => {
  it('TC-QR-11 sends the trimmed code scoped to the guard complex and shows a valid pass', async () => {
    let body: Record<string, unknown> = {};
    server.use(http.post(`${API}/access-grants/validate`, async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>;
      return HttpResponse.json({ success: true, data: { valid: true, message: 'Access granted', visitor_name: 'Juan', unit_name: 'A-101' } });
    }));
    renderGate();
    typeCode('  SA-7K29-4QX1 ');
    validate();
    expect(await screen.findByText('✓ Pase válido')).toBeInTheDocument();
    expect(body).toEqual({ complex_id: C1, method: 'QR', qr_code: 'SA-7K29-4QX1' });
  });

  it('TC-QR-12 a second scan of a single-use pass shows the ALREADY_USED reason', async () => {
    server.use(http.post(`${API}/access-grants/validate`, () =>
      HttpResponse.json({ success: true, data: { valid: false, message: 'Access grant has already been used', reason: 'ALREADY_USED' } })));
    renderGate();
    typeCode('SA-USED');
    validate();
    expect(await screen.findByTestId('reject-reason')).toHaveTextContent('Este pase ya fue utilizado (es de un solo uso).');
  });

  it('TC-QR-13 an empty code is rejected locally without calling the API', async () => {
    const called = vi.fn();
    server.use(http.post(`${API}/access-grants/validate`, () => { called(); return HttpResponse.json({}); }));
    renderGate();
    validate();
    expect(await screen.findByText('Escribe o escanea el código del pase')).toBeInTheDocument();
    expect(called).not.toHaveBeenCalled();
  });

  it('TC-QR-14 QR reader down (QR_001) → falls back to document validation', async () => {
    server.use(http.post(`${API}/access-grants/validate`, breakerOpen));
    renderGate();
    typeCode('SA-7K29-4QX1');
    validate();
    expect(await screen.findByText(/Pasamos a validar por documento/)).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: /Documento/ })).toHaveAttribute('aria-selected', 'true');
  });
});
