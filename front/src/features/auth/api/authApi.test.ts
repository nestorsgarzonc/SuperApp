import { describe, expect, it } from 'vitest';
import { complexesFromMemberships } from './authApi';

describe('complexesFromMemberships', () => {
  it('builds one complex per membership with the permissions of its role', () => {
    const [complex] = complexesFromMemberships([
      { complex_id: 'c1', complex_name: 'Los Pinos', role: 'RESIDENT', unit_id: 'u1', unit_name: 'A-101' },
    ]);
    expect(complex).toMatchObject({ id: 'c1', name: 'Los Pinos', role: 'RESIDENT', unitId: 'u1', unitName: 'A-101' });
    expect(complex?.permissions).toContain('access.authorizations.create');
    expect(complex?.permissions).not.toContain('access.pass.validate');
  });

  it('keeps the highest role when a person holds several in the same complex', () => {
    const complexes = complexesFromMemberships([
      { complex_id: 'c1', complex_name: 'Los Pinos', role: 'RESIDENT', unit_id: 'u1', unit_name: 'A-101' },
      { complex_id: 'c1', complex_name: 'Los Pinos', role: 'GUARD' },
      { complex_id: 'c2', complex_name: 'Torres', role: 'RESIDENT', unit_id: 'u2', unit_name: 'B-202' },
    ]);
    expect(complexes).toHaveLength(2);
    expect(complexes.find(({ id }) => id === 'c1')?.role).toBe('GUARD');
    expect(complexes.find(({ id }) => id === 'c1')?.permissions).toContain('access.pass.validate');
  });

  it('gives no permissions to an unknown role', () => {
    expect(complexesFromMemberships([{ complex_id: 'c1', complex_name: 'X', role: 'OTHER' }])[0]?.permissions).toEqual([]);
  });
});
