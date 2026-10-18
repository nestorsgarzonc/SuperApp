import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { IntercomWebSocket } from './websocket';
import { FakeWebSocket, endpoint } from './test/fixtures';

beforeEach(() => {
  vi.useFakeTimers();
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('Intercom WebSocket transport', () => {
  it('maintains one connection and emits connection and validated JSON events', () => {
    const service = new IntercomWebSocket();
    const listener = vi.fn();
    service.subscribe(listener);
    service.connect(endpoint);
    service.connect(endpoint);
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(listener).toHaveBeenCalledWith({ kind: 'status', status: 'connecting' });
    const socket = FakeWebSocket.latest();
    socket.open();
    expect(listener).toHaveBeenCalledWith({ kind: 'status', status: 'connected' });
    const call = { type: 'CALL', from: 'PORTERIA', target: 'APT_4', message: 'Llamando apartamento 4' };
    socket.receive(call);
    expect(listener).toHaveBeenCalledWith({ kind: 'message', message: call });
    service.send({ type: 'CALL_ACCEPTED', target: 'PORTERIA', message: 'Llamada aceptada' });
    expect(JSON.parse(socket.send.mock.calls[0]![0])).toEqual({ action: 'seendMessage', type: 'CALL_ACCEPTED', target: 'PORTERIA', message: 'Llamada aceptada' });
    service.disconnect();
  });
  it('surfaces malformed JSON, unknown types and missing peer fields', () => {
    const service = new IntercomWebSocket();
    const listener = vi.fn();
    service.subscribe(listener);
    service.connect(endpoint);
    const socket = FakeWebSocket.latest();
    socket.open();
    socket.onmessage?.(new MessageEvent('message', { data: '{broken' }));
    socket.receive({ type: 'CALL' });
    socket.receive({ type: 'UNKNOWN' });
    expect(listener.mock.calls.filter(([event]) => event.kind === 'error')).toHaveLength(3);
    service.disconnect();
  });
  it('reconnects with backoff without replaying messages and stops retrying on logout', () => {
    const service = new IntercomWebSocket();
    service.connect(endpoint);
    const first = FakeWebSocket.latest();
    first.open();
    service.send({ type: 'CALL', target: 'APT_4', message: 'Llamando apartamento 4' });
    first.close();
    expect(() => service.send({ type: 'HANGUP', target: 'APT_4', message: 'Llamada finalizada' })).toThrow(/no está conectada/);
    vi.advanceTimersByTime(999);
    expect(FakeWebSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(1);
    const second = FakeWebSocket.latest();
    expect(second.send).not.toHaveBeenCalled();
    second.close();
    vi.advanceTimersByTime(1999);
    expect(FakeWebSocket.instances).toHaveLength(2);
    service.disconnect();
    vi.advanceTimersByTime(60000);
    expect(FakeWebSocket.instances).toHaveLength(2);
  });
  it('closes old connections and ignores their late events when identity changes', () => {
    const service = new IntercomWebSocket();
    const listener = vi.fn();
    service.subscribe(listener);
    service.connect(`${endpoint}?clientId=APT_4`);
    const old = FakeWebSocket.latest();
    const lateMessage = old.onmessage;
    service.connect(`${endpoint}?clientId=PORTERIA`);
    expect(old.close).toHaveBeenCalledOnce();
    listener.mockClear();
    lateMessage?.(new MessageEvent('message', { data: JSON.stringify({ type: 'ERROR', message: 'Late' }) }));
    expect(listener).not.toHaveBeenCalled();
    service.disconnect();
  });
  it('surfaces socket errors and unsubscribes listeners', () => {
    const service = new IntercomWebSocket();
    const listener = vi.fn();
    const unsubscribe = service.subscribe(listener);
    service.connect(endpoint);
    FakeWebSocket.latest().onerror?.(new Event('error'));
    expect(listener).toHaveBeenCalledWith(expect.objectContaining({ kind: 'error' }));
    unsubscribe();
    listener.mockClear();
    service.disconnect();
    expect(listener).not.toHaveBeenCalled();
  });
});
