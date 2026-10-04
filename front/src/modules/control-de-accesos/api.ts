import { api } from '../../shared/api/client';
import type { AccessGrant, CreateGrantInput, ValidateResult } from './types';

export type BuildingStatus = 'INSIDE' | 'OUTSIDE' | 'UNKNOWN';

export interface ResidentMovement {
  client_id: string;
  movement: 'ENTRY' | 'EXIT';
  unit_label: string;
  visitor_name: string;
  occurred_at: string;
}

export interface ResidentStatus {
  status: BuildingStatus;
  since?: string;
  movements: ResidentMovement[];
}

export interface Presence {
  user_id: string;
  name: string;
  unit_label: string;
  since: string;
}

export interface GrantSummary {
  total: number;
  active: number;
  qr: number;
  pre_auth: number;
}

export const accessApi = {
  /** Portería/administración: solo conteos de pases del conjunto, nunca códigos ni documentos. */
  summary: () => api<GrantSummary>('/access/summary'),

  unitGrants: (unitId: string) => api<AccessGrant[]>(`/units/${unitId}/access-grants`),

  getGrant: (id: string) => api<AccessGrant>(`/access-grants/${id}`),

  createGrant: (input: CreateGrantInput) =>
    api<AccessGrant>('/access-grants', { method: 'POST', body: input }),

  cancelGrant: (id: string) => api<{ message: string }>(`/access-grants/${id}/cancel`, { method: 'PATCH' }),

  /** Portería: consume el pase (un solo uso). Solo GUARD/ADMIN. */
  validateQR: (complexId: string, code: string) =>
    api<ValidateResult>('/access-grants/validate', {
      method: 'POST',
      body: { complex_id: complexId, method: 'QR', qr_code: code.trim() },
    }),

  validatePreAuth: (complexId: string, idNumber: string) =>
    api<ValidateResult>('/access-grants/validate', {
      method: 'POST',
      body: { complex_id: complexId, method: 'PRE_AUTH', visitor_id_number: idNumber.trim() },
    }),
};

/** Entradas/salidas de residentes al edificio. */
export const residentApi = {
  /** Mi estado actual (dentro/fuera) y mis últimos movimientos. */
  me: () => api<ResidentStatus>('/resident-movements/me?limit=20'),

  /** Marca que entro (ENTRY) o salgo (EXIT). `clientId` hace el reintento idempotente. */
  move: (movement: 'ENTRY' | 'EXIT', clientId: string) =>
    api<ResidentMovement>('/resident-movements', { method: 'POST', body: { client_id: clientId, movement } }),

  /** Portería/administración: residentes dentro ahora. */
  presence: () => api<Presence[]>('/resident-movements/presence'),
};
