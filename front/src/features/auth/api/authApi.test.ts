import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { server } from '../../../test/server';
import { getAccessToken, getRefreshToken } from '../../../lib/http';
import { login } from './authApi';

const base = import.meta.env.VITE_API_BASE_URL ?? '/api/v1';

describe('login', () => {
  it('builds the complexes from the real memberships, not a local directory', async () => {
    const identity = await login('demo@superapp.co', 'Demo123!', false);
    expect(identity.complexes).toEqual([
      expect.objectContaining({ id: 'parques-del-rio', name: 'Parques del Río', role: 'RESIDENT', unitId: 'parques-del-rio-apt-302', unitName: 'Apto 302' }),
      expect.objectContaining({ id: 'torres-de-san-jose', role: 'ADMIN' }),
    ]);
    expect(identity.complexes[1]!.permissions).toContain('access.pass.validate');
    expect(identity.roles).toEqual(['RESIDENT', 'ADMIN']);
  });

  it('does not keep the tokens when the memberships cannot be loaded', async () => {
    server.use(http.get(`${base}/me/memberships`, () => HttpResponse.json({ success: false }, { status: 503 })));
    await expect(login('demo@superapp.co', 'Demo123!', false)).rejects.toThrow();
    expect(getAccessToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
  });
});
