import { Badge } from '../../../shared/ui';
import { grantView, type AccessGrant } from '../types';
import type { LogResult } from '../accessLog';

export function GrantBadge({ grant }: { grant: AccessGrant }) {
  switch (grantView(grant)) {
    case 'active':
      return <Badge tone="green">✓ Activa</Badge>;
    case 'upcoming':
      return <Badge tone="amber">▲ Pendiente</Badge>;
    case 'cancelled':
      return <Badge tone="red">✕ Revocada</Badge>;
    default:
      return <Badge tone="gray">{grant.status === 'USED' ? 'Usada' : 'Vencida'}</Badge>;
  }
}

export function ResultBadge({ result }: { result: LogResult }) {
  return result === 'AUTHORIZED' ? <Badge tone="green">✓ Autorizado</Badge> : <Badge tone="red">✕ Denegado</Badge>;
}
