export type VisitorType = 'VISITOR' | 'DELIVERY' | 'SERVICE' | 'CONTRACTOR' | 'GUEST';
export type GrantStatus = 'ACTIVE' | 'USED' | 'EXPIRED' | 'CANCELLED';

export interface AccessGrant {
  id: string;
  complex_id: string;
  unit_id: string;
  created_by: string;
  method: 'QR' | 'PRE_AUTH';
  visitor_type: VisitorType;
  visitor_name: string;
  visitor_id_number?: string;
  qr_code_hash?: string;
  details?: string;
  status: GrantStatus;
  valid_from?: string;
  valid_until?: string;
  used_at?: string;
  created_at: string;
  single_use: boolean;
  creator_name?: string;
  unit_name?: string;
  qr_code_url?: string;
}

export interface CreateGrantInput {
  unit_id: string;
  method: 'QR' | 'PRE_AUTH';
  visitor_type: VisitorType;
  visitor_name: string;
  visitor_id_number?: string;
  details?: string;
  valid_from?: string;
  valid_until: string;
}

export type RejectReason =
  | 'NOT_FOUND'
  | 'NO_AUTHORIZATION'
  | 'NOT_YET_VALID'
  | 'EXPIRED'
  | 'ALREADY_USED'
  | 'CANCELLED';

export interface ValidateResult {
  valid: boolean;
  message: string;
  reason?: RejectReason;
  valid_from?: string;
  valid_until?: string;
  access_grant?: AccessGrant;
  unit_id?: string;
  unit_name?: string;
  visitor_name?: string;
  visitor_type?: VisitorType;
}

export const VISITOR_TYPE_LABEL: Record<VisitorType, string> = {
  VISITOR: 'Visita',
  DELIVERY: 'Domicilio',
  SERVICE: 'Personal técnico',
  CONTRACTOR: 'Contratista',
  GUEST: 'Invitado',
};

export const VISITOR_TYPES = Object.keys(VISITOR_TYPE_LABEL) as VisitorType[];

/** Estado visible en la lista de autorizaciones (el backend no tiene "pendiente"). */
export type GrantView = 'active' | 'upcoming' | 'expired' | 'cancelled';

export function grantView(g: AccessGrant, now = Date.now()): GrantView {
  if (g.status === 'CANCELLED') return 'cancelled';
  if (g.status === 'USED' || g.status === 'EXPIRED') return 'expired';
  if (g.valid_until && new Date(g.valid_until).getTime() < now) return 'expired';
  if (g.valid_from && new Date(g.valid_from).getTime() > now) return 'upcoming';
  return 'active';
}

/** Motivo del rechazo, en palabras para el guardia (null si el backend no dio uno). */
export function rejectText(r: Pick<ValidateResult, 'reason' | 'valid_from' | 'valid_until'>, when: (iso: string) => string): string | null {
  switch (r.reason) {
    case 'NOT_YET_VALID':
      return r.valid_from ? `El pase aún no está vigente. Empieza ${when(r.valid_from)}.` : 'El pase aún no está vigente.';
    case 'EXPIRED':
      return r.valid_until ? `El pase venció ${when(r.valid_until)}.` : 'El pase está vencido.';
    case 'ALREADY_USED':
      return 'Este pase ya fue utilizado (es de un solo uso).';
    case 'CANCELLED':
      return 'El residente revocó este pase.';
    case 'NOT_FOUND':
      // También cubre un código de otro conjunto: no se debe distinguir (no revelar que existe).
      return 'No se pudo validar el pase. Verifica el código o valídalo por documento.';
    case 'NO_AUTHORIZATION':
      return 'No hay una autorización vigente para este documento.';
    default:
      return null;
  }
}
