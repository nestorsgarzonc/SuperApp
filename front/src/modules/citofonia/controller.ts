import type { IntercomClient, IntercomConfig } from './config';
import type { ConnectionStatus, IncomingMessage, OutgoingMessage, SocketEvent } from './protocol';
import type { SignalingTransport } from './websocket';

export type CallStatus = 'idle' | 'calling' | 'incoming' | 'accepted' | 'rejected' | 'ended' | 'error';
export interface Call {
  /** Identificador solo local: el backend actual no envía ni correlaciona callId. */
  callId: string;
  peer: string;
  label: string;
  direction: 'incoming' | 'outgoing';
}
export interface IntercomState {
  connection: ConnectionStatus;
  status: CallStatus;
  call: Call | null;
  delivered: boolean;
  error: string | null;
}
const initialState: IntercomState = { connection: 'disconnected', status: 'idle', call: null, delivered: false, error: null };
const active = (status: CallStatus) => status === 'calling' || status === 'incoming' || status === 'accepted';

export class IntercomController {
  private state: IntercomState = initialState;
  private listeners = new Set<() => void>();
  private unsubscribe: (() => void) | null = null;

  constructor(private transport: SignalingTransport, readonly config: IntercomConfig) {}

  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };
  private update(patch: Partial<IntercomState>) {
    this.state = { ...this.state, ...patch };
    this.listeners.forEach((listener) => listener());
  }
  start() {
    if (this.unsubscribe) return;
    this.unsubscribe = this.transport.subscribe((event) => this.onEvent(event));
    this.transport.connect(this.config.url);
  }
  stop() {
    if (this.state.call && active(this.state.status) && this.state.connection === 'connected') {
      this.send('HANGUP', 'Llamada finalizada');
    }
    this.unsubscribe?.();
    this.unsubscribe = null;
    this.transport.disconnect();
    this.update({ ...initialState });
  }
  private send(type: OutgoingMessage['type'], message: string) {
    const call = this.state.call;
    if (!call) return false;
    try {
      this.transport.send({ type, target: call.peer, message });
      return true;
    } catch (error) {
      this.update({ error: error instanceof Error ? error.message : 'No se pudo enviar la señal de Citofonía.' });
      return false;
    }
  }
  callApartment(clientId: string) {
    if (active(this.state.status)) return;
    const apartment = this.config.apartments.find((client) => client.clientId === clientId);
    if (this.config.client.role !== 'porter' || !apartment) {
      this.update({ status: 'error', error: 'Selecciona un apartamento del conjunto activo.' });
      return;
    }
    this.startCall(apartment);
  }
  callPorter() {
    if (active(this.state.status)) return;
    if (this.config.client.role !== 'apartment') {
      this.update({ status: 'error', error: 'Solo los apartamentos pueden llamar a portería.' });
      return;
    }
    this.startCall(this.config.porter);
  }
  private startCall(peer: IntercomClient) {
    const callId = crypto.randomUUID();
    this.update({ call: { callId, peer: peer.clientId, label: peer.label, direction: 'outgoing' }, status: 'calling', error: null, delivered: false });
    if (!this.send('CALL', `Llamando ${peer.label.charAt(0).toLocaleLowerCase()}${peer.label.slice(1)}`)) this.update({ status: 'error' });
  }
  accept() {
    if (this.state.status === 'incoming' && this.send('CALL_ACCEPTED', 'Llamada aceptada')) this.update({ status: 'accepted', error: null });
  }
  reject() {
    if (this.state.status === 'incoming' && this.send('CALL_REJECTED', 'Llamada rechazada')) this.update({ status: 'rejected', error: null });
  }
  hangup() {
    if (active(this.state.status) && this.send('HANGUP', 'Llamada finalizada')) this.update({ status: 'ended', error: null });
  }
  private onEvent(event: SocketEvent) {
    if (event.kind === 'error') {
      this.update({ error: event.message });
    } else if (event.kind === 'status') {
      const interrupted = event.status === 'disconnected' && active(this.state.status);
      this.update({
        connection: event.status,
        ...(event.status === 'connected' && !active(this.state.status) && this.state.status !== 'error' ? { error: null } : {}),
        ...(interrupted ? { status: 'error', error: 'Se perdió la conexión. La llamada fue interrumpida; no se reanudará automáticamente.' } : {}),
      });
    } else {
      this.onMessage(event.message);
    }
  }
  private onMessage(message: IncomingMessage) {
    const { client, porter } = this.config;
    if (message.type === 'CALL') {
      const peer = client.role === 'apartment'
        ? message.from === porter.clientId ? porter : undefined
        : this.config.apartments.find(({ clientId }) => clientId === message.from);
      if (!peer || message.target !== client.clientId) return;
      if (active(this.state.status)) {
        if (this.state.call?.peer !== peer.clientId) {
          try {
            this.transport.send({ type: 'CALL_REJECTED', target: peer.clientId, message: 'Hay una llamada en curso.' });
          } catch (error) {
            this.update({ error: error instanceof Error ? error.message : 'No se pudo rechazar la llamada adicional.' });
          }
        }
        return;
      }
      this.update({ call: { callId: crypto.randomUUID(), peer: peer.clientId, label: peer.label, direction: 'incoming' }, status: 'incoming', delivered: false, error: null });
      return;
    }
    const { call, status } = this.state;
    if (message.type === 'ERROR') {
      this.update({ ...(call ? { status: 'error' } : {}), error: message.message });
      return;
    }
    if (!call || !active(status)) return;
    if (message.type === 'DELIVERED') {
      if (message.target === call.peer) this.update({ delivered: true });
    } else if (message.from === call.peer && message.target === client.clientId) {
      if (message.type === 'HANGUP') this.update({ status: 'ended', error: null });
      else if (status === 'calling' && message.type === 'CALL_ACCEPTED') this.update({ status: 'accepted', error: null });
      else if (status === 'calling' && message.type === 'CALL_REJECTED') this.update({ status: 'rejected', error: null });
    }
  }
}
