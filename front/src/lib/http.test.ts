import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../test/server';
import { api } from '../shared/api/client';
import { setApiSession } from '../shared/api/client';

describe('shared API refresh', () => {
  it('refreshes once and retries a protected request with the new token', async () => {
    let attempts = 0;
    server.use(
      http.get('/api/v1/private', ({ request }) => {
        attempts += 1;
        if (request.headers.get('authorization') === 'Bearer new-access-token') return HttpResponse.json({ success: true, data: { ok: true } });
        return HttpResponse.json({ success: false, error: { code: 'unauthorized' } }, { status: 401 });
      }),
      http.post('/api/v1/auth/refresh', () => HttpResponse.json({ access_token: 'new-access-token' })),
    );
    setApiSession({ token: 'expired-token', complexId: 'tenant-1', complexName: 'Conjunto', unitId: '', userId: 'user-1', role: 'RESIDENT', name: 'Persona', subtitle: 'Residente' });
    await expect(api<{ ok: boolean }>('/private')).resolves.toEqual({ ok: true });
    expect(attempts).toBe(2);
  });
});
