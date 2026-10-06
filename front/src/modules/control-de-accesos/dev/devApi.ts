// ───────────────────────────────────────────────────────────────────────────
// SOLO PRUEBAS del módulo control-de-accesos (carpeta `dev/`).
//
// Reemplaza al login mientras el módulo `auth` no exista: usa los endpoints
// /dev/users y /dev/token del ms de control-de-accesos (solo ENV=development).
// No toca nada de auth ni asume su contrato. Cuando el módulo `auth` aporte
// la sesión real, esta carpeta se elimina y main.tsx deja de importarla.
// ───────────────────────────────────────────────────────────────────────────
import { api, type Membership, type Role, type Session } from '../../../shared/api/client';

interface DevMembership {
  complex_id: string;
  complex_name: string;
  role: Role;
  unit_id?: string;
  unit_name?: string;
}

export interface DevUser {
  userId: string;
  name: string;
  memberships: Membership[];
}

export const ROLE_LABEL: Record<Role, string> = {
  RESIDENT: 'Residente',
  OWNER: 'Propietario',
  GUARD: 'Portería',
  ADMIN: 'Administración',
};

const fromDev = (m: DevMembership): Membership => ({
  complexId: m.complex_id,
  complexName: m.complex_name,
  role: m.role,
  unitId: m.unit_id,
  unitName: m.unit_name,
});

export const membershipLabel = (m: Membership) => [ROLE_LABEL[m.role], m.unitName].filter(Boolean).join(' · ');

/** Personas, conjuntos y roles, leídos de la base del ms (nada fijo en el front). */
export async function listDevUsers(): Promise<DevUser[]> {
  const rows = await api<{ user_id: string; name: string; memberships: DevMembership[] }[]>('/dev/users', { auth: false });
  return rows.map((u) => ({ userId: u.user_id, name: u.name, memberships: u.memberships.map(fromDev) }));
}

/** Token de una persona actuando en una de sus membresías (conjunto + rol). */
export async function loginDemo(userId: string, m: Membership): Promise<Session> {
  const res = await api<{
    token: string;
    user_id: string;
    name: string;
    complex_id: string;
    complex_name: string;
    role: Role;
    unit_id?: string;
    unit_name?: string;
    memberships: DevMembership[];
  }>(`/dev/token?user_id=${userId}&complex_id=${m.complexId}&role=${m.role}`, { auth: false });
  return {
    token: res.token,
    userId: res.user_id,
    name: res.name,
    complexId: res.complex_id,
    complexName: res.complex_name,
    unitId: res.unit_id ?? '',
    unitName: res.unit_name,
    role: res.role,
    subtitle: [ROLE_LABEL[res.role], res.unit_name].filter(Boolean).join(' · '),
    memberships: res.memberships.map(fromDev),
  };
}
