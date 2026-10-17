import { z } from 'zod';
import type { Session } from '../../shared/api/client';

const clientSchema = z.object({
  complexId: z.string().trim().min(1),
  clientId: z.string().trim().min(1),
  role: z.enum(['apartment', 'porter']),
  unitId: z.string().trim().min(1).optional(),
  label: z.string().trim().min(1),
}).superRefine((client, context) => {
  if (client.role === 'apartment' && !client.unitId) {
    context.addIssue({ code: z.ZodIssueCode.custom, message: 'Cada apartamento necesita unitId.' });
  }
});
export type IntercomClient = z.infer<typeof clientSchema>;
export interface IntercomConfig {
  url: string;
  client: IntercomClient;
  porter: IntercomClient;
  apartments: IntercomClient[];
}

export function parseIntercomClients(directory: string | undefined): IntercomClient[] {
  if (!directory?.trim()) throw new Error('Configura VITE_INTERCOM_CLIENTS con las identidades del conjunto.');
  let decoded: unknown;
  try {
    decoded = JSON.parse(directory);
  } catch {
    throw new Error('VITE_INTERCOM_CLIENTS debe contener un directorio JSON válido.');
  }
  const parsed = z.array(clientSchema).safeParse(decoded);
  if (!parsed.success) throw new Error('Directorio de Citofonía inválido: se requieren complexId, clientId, role, label y unitId para apartamentos.');
  return parsed.data;
}

/** Adaptador temporal: sustituir el directorio por identidades validadas del backend. */
export function resolveIntercomConfig(
  session: Pick<Session, 'complexId' | 'unitId' | 'role'>,
  endpoint: string | undefined,
  directory: string | undefined,
): IntercomConfig {
  if (!endpoint?.trim()) throw new Error('Configura VITE_INTERCOM_WS_URL para habilitar Citofonía.');
  const url = new URL(endpoint.trim());
  if (url.protocol !== 'wss:' || url.username || url.password || url.hash) {
    throw new Error('VITE_INTERCOM_WS_URL debe ser una URL wss:// válida, sin credenciales ni fragmento.');
  }
  const clients = parseIntercomClients(directory);
  if (new Set(clients.map(({ clientId }) => clientId)).size !== clients.length) {
    throw new Error('Los clientId de Citofonía deben ser únicos en todo el endpoint.');
  }
  const local = clients.filter(({ complexId }) => complexId === session.complexId);
  const porters = local.filter(({ role }) => role === 'porter');
  if (porters.length !== 1) throw new Error('Configura exactamente una identidad de portería para el conjunto activo.');
  const porter = porters[0]!;
  const apartments = local.filter(({ role }) => role === 'apartment');
  if (new Set(apartments.map(({ unitId }) => unitId)).size !== apartments.length) {
    throw new Error('Cada unidad debe tener una sola identidad de Citofonía.');
  }
  const isPorter = session.role === 'GUARD' || session.role === 'ADMIN';
  const client = isPorter ? porter : apartments.find(({ unitId }) => unitId === session.unitId);
  if (!client) throw new Error('Tu unidad no tiene una identidad de Citofonía configurada.');
  url.searchParams.set('clientId', client.clientId);
  url.searchParams.set('role', client.role);
  return { url: url.toString(), client, porter, apartments };
}
