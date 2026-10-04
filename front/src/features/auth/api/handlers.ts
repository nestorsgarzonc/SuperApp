import { http, HttpResponse } from 'msw';

let failedAttempts = 0;
let blockedUntil = 0;
const base = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';
export const handlers = [
  http.post(`${base}/auth/login`, async ({ request }) => {
    const body = await request.json() as { email?: string; password?: string };
    if (blockedUntil > Date.now()) return HttpResponse.json({ code: 'account_blocked' }, { status: 423 });
    if (blockedUntil) { blockedUntil = 0; failedAttempts = 0; }
    if (body.email !== 'demo@superapp.co' || body.password !== 'Demo123!') {
      failedAttempts += 1;
      if (failedAttempts >= 5) { blockedUntil = Date.now() + 15 * 60 * 1000; return HttpResponse.json({ code: 'account_blocked' }, { status: 423 }); }
      return HttpResponse.json({ code: 'invalid_credentials' }, { status: 401 });
    }
    failedAttempts = 0;
    return HttpResponse.json({ user: {
      id: 'demo-resident', full_name: 'Camila Ramírez', email: 'demo@superapp.co', roles: ['residente'], permissions: [
        'access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read',
        'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read',
      ],
      complexes: [
        { id: 'parques-del-rio', name: 'Parques del Río', address: 'Calle 18 # 42-16 · Medellín, Antioquia', status: 'Activo' },
        { id: 'torres-de-san-jose', name: 'Torres de San José', address: 'Carrera 12 # 91-24 · Bogotá, Cundinamarca', status: 'Activo' },
      ],
    }, tokens: { access_token: 'mock-access-token', refresh_token: 'mock-refresh-token', expires_in: 900 } });
  }),
  http.post(`${base}/auth/refresh`, () => HttpResponse.json({ access_token: 'mock-refreshed-access-token', expires_in: 900 })),
  http.post(`${base}/auth/select-complex`, async ({ request }) => {
    const body = await request.json() as { complex_id?: string };
    if (body.complex_id !== 'parques-del-rio' && body.complex_id !== 'torres-de-san-jose') return HttpResponse.json({ code: 'forbidden' }, { status: 403 });
    return HttpResponse.json({ active_complex: { id: body.complex_id, role: 'residente', permissions: [
      'access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read',
      'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read',
    ] }, access_token: `mock-access-${body.complex_id}` });
  }),
  http.post(`${base}/auth/logout`, () => new HttpResponse(null, { status: 204 })),
  http.post(`${base}/auth/logout-all`, () => new HttpResponse(null, { status: 204 })),
  http.post(`${base}/auth/forgot-password`, () => new HttpResponse(null, { status: 204 })),
  http.post(`${base}/auth/reset-password`, () => new HttpResponse(null, { status: 204 })),
];
