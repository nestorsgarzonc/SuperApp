import { describe, expect, it } from 'vitest';
import { resolveIntercomConfig } from './config';
import { directory, endpoint, session } from './test/fixtures';

describe('Intercom identity configuration', () => {
  it('derives the apartment from the authenticated complex and unit, not its display name', () => {
    const config = resolveIntercomConfig(session, endpoint, JSON.stringify(directory));
    expect(config.client.clientId).toBe('APT_4');
    expect(config.apartments).toHaveLength(1);
    expect(new URL(config.url).searchParams.get('role')).toBe('apartment');
  });
  it.each(['GUARD', 'ADMIN'] as const)('uses the porter identity for %s', (role) => {
    const config = resolveIntercomConfig({ ...session, role, unitId: '' }, endpoint, JSON.stringify(directory));
    expect(new URL(config.url).searchParams.get('clientId')).toBe('PORTERIA');
  });
  it('changes identities when the active complex changes', () => {
    const config = resolveIntercomConfig({ ...session, complexId: 'complex-2', unitId: 'unit-8' }, endpoint, JSON.stringify(directory));
    expect(config.client.clientId).toBe('APT_8');
    expect(config.porter.clientId).toBe('PORTERIA_2');
  });
  it('URL encodes logical IDs and preserves stage query parameters', () => {
    const custom = directory.map((client) => client.clientId === 'APT_4' ? { ...client, clientId: 'APT 4&x=1' } : client);
    const config = resolveIntercomConfig(session, `${endpoint}?version=1`, JSON.stringify(custom));
    const url = new URL(config.url);
    expect(url.searchParams.get('clientId')).toBe('APT 4&x=1');
    expect(url.searchParams.get('x')).toBeNull();
    expect(url.searchParams.get('version')).toBe('1');
  });
  it.each([
    [undefined, '[]'],
    ['https://example.com', '[]'],
    [endpoint, 'invalid-json'],
    [endpoint, '[{}]'],
    [endpoint, JSON.stringify([...directory, directory[1]])],
    [endpoint, JSON.stringify(directory.filter(({ clientId }) => clientId !== 'APT_4'))],
    [endpoint, JSON.stringify(directory.filter(({ role }) => role !== 'porter'))],
  ])('fails explicitly for missing or invalid configuration', (url, clients) => {
    expect(() => resolveIntercomConfig(session, url, clients)).toThrow();
  });
});
