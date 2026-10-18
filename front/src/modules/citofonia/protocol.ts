import { z } from 'zod';

const id = z.string().trim().min(1);
const peer = { from: id, target: id, message: z.string().optional() };
const control = z.enum(['CALL_ACCEPTED', 'CALL_REJECTED', 'HANGUP']);

export const incomingMessageSchema = z.union([
  z.object({ type: z.literal('CALL'), ...peer }),
  z.object({ type: control, ...peer }),
  z.object({ type: z.literal('DELIVERED'), target: id }),
  z.object({ type: z.literal('ERROR'), message: z.string().min(1) }),
]);

// Ruta actual del prototipo AWS. Corregir solo al migrar el backend coordinadamente.
export const INTERCOM_ACTION = 'seendMessage';
export type IncomingMessage = z.infer<typeof incomingMessageSchema>;
export interface OutgoingMessage {
  type: 'CALL' | 'CALL_ACCEPTED' | 'CALL_REJECTED' | 'HANGUP';
  target: string;
  message: string;
}
export type ConnectionStatus = 'connected' | 'connecting' | 'disconnected';
export type SocketEvent =
  | { kind: 'status'; status: ConnectionStatus }
  | { kind: 'message'; message: IncomingMessage }
  | { kind: 'error'; message: string };
