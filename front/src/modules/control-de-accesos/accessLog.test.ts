import { describe, expect, it } from 'vitest';
import { delay, http, HttpResponse } from 'msw';
import { server } from '../../test/server';
import { setApiSession } from '../../shared/api/client';
import { addLogEntry, syncPending } from './accessLog';

describe('access log sync', () => {
  it('sends an entry registered while a previous sync is still in flight', async () => {
    const posted: string[] = [];
    server.use(
      http.post('/api/v1/access-logs', async ({ request }) => {
        const body = (await request.json()) as { movement: string };
        await delay(50); // red lenta: el segundo registro llega con el primero aún en vuelo
        posted.push(body.movement);
        return HttpResponse.json({ success: true, data: {} }, { status: 201 });
      }),
      http.get('/api/v1/access-logs', () => HttpResponse.json({ success: true, data: [] })),
    );
    setApiSession({ token: 't', complexId: 'c1', complexName: 'Conjunto', unitId: '', userId: 'guard-1', role: 'GUARD', name: 'Guardia', subtitle: 'Portería' });
    const base = { result: 'AUTHORIZED' as const, visitor: 'Visita', unit: 'A-101', visitorType: 'Visita', registeredBy: 'Guardia' };
    addLogEntry({ ...base, movement: 'ENTRY' });
    addLogEntry({ ...base, movement: 'EXIT' });
    await expect.poll(() => posted, { timeout: 2000 }).toEqual(['ENTRY', 'EXIT']);
    await syncPending(); // nada pendiente: no reenvía
    expect(posted).toEqual(['ENTRY', 'EXIT']);
  });
});
