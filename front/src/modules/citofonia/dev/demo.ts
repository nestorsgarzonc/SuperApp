import type { Session } from '../../../shared/api/client';
import { resolveIntercomConfig, type IntercomClient } from '../config';

export function isIntercomDemo() {
  return import.meta.env.DEV && import.meta.env.VITE_INTERCOM_DEMO === 'true';
}

export function createDemoSession(client: IntercomClient): Session {
  if (!isIntercomDemo()) throw new Error('El modo de prueba de Citofonía no está habilitado.');
  const session: Session = {
    token: '',
    complexId: client.complexId,
    complexName: 'Citofonía · Prueba sin backend local',
    unitId: client.unitId ?? '',
    userId: `intercom-demo-${client.clientId}`,
    role: client.role === 'porter' ? 'GUARD' : 'RESIDENT',
    name: client.label,
    subtitle: 'Sesión temporal de prueba',
  };
  resolveIntercomConfig(session, import.meta.env.VITE_INTERCOM_WS_URL, import.meta.env.VITE_INTERCOM_CLIENTS);
  return session;
}
