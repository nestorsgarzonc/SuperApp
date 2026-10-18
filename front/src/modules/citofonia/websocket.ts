import { incomingMessageSchema, INTERCOM_ACTION, type OutgoingMessage, type SocketEvent } from './protocol';

export interface SignalingTransport {
  subscribe(listener: (event: SocketEvent) => void): () => void;
  connect(url: string): void;
  send(message: OutgoingMessage): void;
  disconnect(): void;
}

export class IntercomWebSocket implements SignalingTransport {
  private socket: WebSocket | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private listeners = new Set<(event: SocketEvent) => void>();
  private url: string | null = null;
  private attempts = 0;

  subscribe(listener: (event: SocketEvent) => void) {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  private emit(event: SocketEvent) {
    this.listeners.forEach((listener) => listener(event));
  }

  connect(url: string) {
    if (this.url === url && (this.socket || this.timer)) return;
    this.disconnect();
    this.url = url;
    this.attempts = 0;
    this.open();
  }

  private open() {
    if (!this.url) return;
    this.emit({ kind: 'status', status: 'connecting' });
    let socket: WebSocket;
    try {
      socket = new WebSocket(this.url);
    } catch {
      this.emit({ kind: 'error', message: 'No se pudo abrir el WebSocket de Citofonía.' });
      this.emit({ kind: 'status', status: 'disconnected' });
      this.reconnect();
      return;
    }
    this.socket = socket;
    socket.onopen = () => {
      if (this.socket !== socket) return;
      this.attempts = 0;
      this.emit({ kind: 'status', status: 'connected' });
    };
    socket.onmessage = (event: MessageEvent<unknown>) => {
      if (this.socket !== socket) return;
      let message;
      try {
        if (typeof event.data !== 'string') throw new Error('Mensaje no textual');
        message = incomingMessageSchema.parse(JSON.parse(event.data));
      } catch {
        this.emit({ kind: 'error', message: 'Citofonía recibió un mensaje JSON inválido o incompatible con el contrato.' });
        return;
      }
      this.emit({ kind: 'message', message });
    };
    socket.onerror = () => {
      if (this.socket !== socket) return;
      this.emit({ kind: 'error', message: 'Error de conexión con Citofonía. Comprueba la red y el endpoint WebSocket.' });
    };
    socket.onclose = () => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.emit({ kind: 'status', status: 'disconnected' });
      this.reconnect();
    };
  }

  private reconnect() {
    if (!this.url || this.timer) return;
    const delay = Math.min(1000 * 2 ** Math.min(this.attempts++, 5), 30000);
    this.timer = setTimeout(() => {
      this.timer = null;
      this.open();
    }, delay);
  }

  send(message: OutgoingMessage) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      throw new Error('Citofonía no está conectada. Espera a recuperar la conexión.');
    }
    // No se encolan llamadas: reconectar nunca debe reenviar señalización antigua.
    this.socket.send(JSON.stringify({
      action: INTERCOM_ACTION,
      type: message.type,
      target: message.target,
      message: message.message,
    }));
  }

  disconnect() {
    this.url = null;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onclose = null;
      socket.close();
    }
    this.emit({ kind: 'status', status: 'disconnected' });
  }
}
