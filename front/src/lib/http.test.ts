import { describe, expect, it, vi } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '../test/server';
import { api } from '../shared/api/client';
import { setApiSession } from '../shared/api/client';
import { getRefreshToken, setRefreshToken } from './http';

const session = { token: 'expired-token', complexId: 'tenant-1', complexName: 'Conjunto', unitId: '', userId: 'user-1', role: 'RESIDENT' as const, name: 'Persona', subtitle: 'Residente' };

describe('shared API refresh', () => {
  it('refreshes once with the refresh token, keeps the rotated one and retries with the new token', async () => {
    let attempts = 0;
    let sent: unknown;
    server.use(
      http.get('/api/v1/private', ({ request }) => {
        attempts += 1;
        if (request.headers.get('authorization') === 'Bearer new-access-token') return HttpResponse.json({ success: true, data: { ok: true } });
        return HttpResponse.json({ success: false, error: { code: 'unauthorized' } }, { status: 401 });
      }),
      http.post('/api/v1/auth/refresh', async ({ request }) => {
        sent = await request.json();
        return HttpResponse.json({ access_token: 'new-access-token', refresh_token: 'refresh-2' });
      }),
    );
    setRefreshToken('refresh-1');
    setApiSession(session);
    await expect(api<{ ok: boolean }>('/private')).resolves.toEqual({ ok: true });
    expect(attempts).toBe(2);
    expect(sent).toEqual({ refresh_token: 'refresh-1' });
    expect(getRefreshToken()).toBe('refresh-2');
  });

  it('an optional call (notifications) that stays 401 fails without expiring the session', async () => {
    server.use(
      http.get('/api/v1/notifications/x', () => HttpResponse.json({ success: false, error: { code: 'AUTH_001' } }, { status: 401 })),
      http.post('/api/v1/auth/refresh', () => HttpResponse.json({}, { status: 401 })),
    );
    const expired = vi.fn();
    window.addEventListener('superapp:session-expired', expired);
    setRefreshToken('refresh-1');
    setApiSession(session);
    await expect(api('/notifications/x', { optional: true })).rejects.toMatchObject({ status: 401 });
    expect(expired).not.toHaveBeenCalled();
    window.removeEventListener('superapp:session-expired', expired);
  });
});
