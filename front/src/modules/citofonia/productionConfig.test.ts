import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { demoMemberships, permissionsForRole } from '../../features/auth/demoDirectory';
import { createSession } from '../../features/auth/components/AuthIntegration';
import type { Complex } from '../../features/auth/api/authApi';
import { resolveIntercomConfig } from './config';

describe('deployed intercom test directory', () => {
  it.each([
    ['ana.garcia@demo.test', 'APT_4', 'apartment'],
    ['luis.mora@demo.test', 'PORTERIA', 'porter'],
  ])('resolves the authenticated membership of %s using the production environment', (email, clientId, role) => {
    const membership = demoMemberships[email]![0]!;
    const complex: Complex = {
      ...membership, address: '', status: 'Activo',
      permissions: permissionsForRole[membership.role],
    };
    const session = createSession({
      id: email, name: email, email, roles: [membership.role],
      permissions: complex.permissions ?? [], complexes: [complex],
    }, complex);
    const env = readFileSync('.env.production', 'utf8');
    const endpoint = env.match(/^VITE_INTERCOM_WS_URL=(.+)$/m)?.[1]?.trim();
    const clients = env.match(/^VITE_INTERCOM_CLIENTS=(.+)$/m)?.[1]?.trim();
    const config = resolveIntercomConfig(session, endpoint, clients);
    expect(config.client.clientId).toBe(clientId);
    expect(config.client.role).toBe(role);
    expect(config.porter.clientId).toBe('PORTERIA');
    expect(config.apartments.map(({ clientId }) => clientId)).toEqual(['APT_4']);
    expect(new URL(config.url).searchParams.get('clientId')).toBe(clientId);
    expect(new URL(config.url).searchParams.get('role')).toBe(role);
    if (role === 'apartment') {
      expect(session.unitName).toBe('A-101');
      expect(session.unitId).toBe('10000000-0000-0000-0000-000000000101');
      expect(config.client.unitId).toBe(session.unitId);
    }
  });
});
