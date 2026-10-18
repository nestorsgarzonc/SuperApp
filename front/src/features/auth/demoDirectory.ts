export type DemoRole = 'RESIDENT' | 'GUARD' | 'ADMIN';

export const permissionsForRole: Record<DemoRole, string[]> = {
  RESIDENT: ['access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read'],
  GUARD: ['access.home.read', 'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read'],
  ADMIN: ['access.home.read', 'access.authorizations.read', 'access.authorizations.create', 'access.building.read', 'access.pass.validate', 'access.visitor.register', 'access.movement.register', 'access.history.read'],
};

const C1 = { id: '00000000-0000-0000-0000-000000000001', name: 'Los Pinos' };
const C2 = { id: '00000000-0000-0000-0000-000000000002', name: 'Alto del Bosque' };

export const demoMemberships: Record<string, Array<{ id: string; name: string; role: DemoRole }>> = {
  'ana.garcia@demo.test': [{ ...C1, role: 'RESIDENT' }],
  'pedro.gomez@demo.test': [{ ...C2, role: 'RESIDENT' }],
  'luis.mora@demo.test': [{ ...C1, role: 'GUARD' }],
  'marta.ruiz@demo.test': [{ ...C2, role: 'GUARD' }],
  'carlos.rojas@demo.test': [{ ...C1, role: 'ADMIN' }],
  'sofia.diaz@demo.test': [{ ...C1, role: 'RESIDENT' }, { ...C2, role: 'RESIDENT' }],
  'andres.vega@demo.test': [{ ...C1, role: 'RESIDENT' }, { ...C2, role: 'GUARD' }],
  'admin@superapp.com': [{ ...C1, role: 'ADMIN' }],
};
