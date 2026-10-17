import { vi } from 'vitest';
import type { Session } from '../../../shared/api/client';
import type { IntercomConfig } from '../config';

export const directory = [
  { complexId: 'complex-1', clientId: 'PORTERIA', role: 'porter', label: 'Portería' },
  { complexId: 'complex-1', unitId: 'unit-4', clientId: 'APT_4', role: 'apartment', label: 'Apartamento 4' },
  { complexId: 'complex-2', clientId: 'PORTERIA_2', role: 'porter', label: 'Portería 2' },
  { complexId: 'complex-2', unitId: 'unit-8', clientId: 'APT_8', role: 'apartment', label: 'Apartamento 8' },
] as const;
export const endpoint = 'wss://example.execute-api.us-east-2.amazonaws.com/dev';
export const session: Session = {
  token: '', complexId: 'complex-1', complexName: 'Los Pinos', unitId: 'unit-4', userId: 'resident-4',
  role: 'RESIDENT', name: 'Residente', subtitle: 'Apartamento 4',
};
export const apartmentConfig: IntercomConfig = {
  url: `${endpoint}?clientId=APT_4&role=apartment`,
  client: directory[1], porter: directory[0], apartments: [directory[1]],
};
export const porterConfig: IntercomConfig = { ...apartmentConfig, client: directory[0] };

export class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];
  readyState = FakeWebSocket.CONNECTING;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<unknown>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  send = vi.fn<(data: string) => void>();
  close = vi.fn(() => {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close'));
  });
  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this);
  }
  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.(new Event('open'));
  }
  receive(message: unknown) {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(message) }));
  }
  static latest() {
    const socket = this.instances.at(-1);
    if (!socket) throw new Error('No se creó un WebSocket.');
    return socket;
  }
}
