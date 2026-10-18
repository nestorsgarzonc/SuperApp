import { describe, expect, it, vi } from 'vitest';
import { IntercomController } from './controller';
import type { SocketEvent } from './protocol';
import type { SignalingTransport } from './websocket';
import { apartmentConfig, porterConfig } from './test/fixtures';

function setup(porter = true) {
  let listener: ((event: SocketEvent) => void) | undefined;
  const transport: SignalingTransport = {
    connect: vi.fn(), disconnect: vi.fn(), send: vi.fn(),
    subscribe: (next) => { listener = next; return () => { listener = undefined; }; },
  };
  const controller = new IntercomController(transport, { ...(porter ? porterConfig : apartmentConfig) });
  controller.start();
  const emit = (event: SocketEvent) => listener?.(event);
  emit({ kind: 'status', status: 'connected' });
  const receive = (message: Extract<SocketEvent, { kind: 'message' }>['message']) => emit({ kind: 'message', message });
  return { controller, transport, receive, emit };
}

describe('Intercom call state', () => {
  it.each(['CALL_ACCEPTED', 'CALL_REJECTED'] as const)('allows apartments to call portería and receive %s', (type) => {
    const { controller, transport, receive } = setup(false);
    controller.callPorter();
    expect(transport.send).toHaveBeenLastCalledWith({ type: 'CALL', target: 'PORTERIA', message: 'Llamando portería' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'calling', call: { peer: 'PORTERIA', direction: 'outgoing' } });
    receive({ type, from: 'PORTERIA_2', target: 'APT_4' });
    expect(controller.getSnapshot().status).toBe('calling');
    receive({ type, from: 'PORTERIA', target: 'APT_4' });
    expect(controller.getSnapshot().status).toBe(type === 'CALL_ACCEPTED' ? 'accepted' : 'rejected');
    if (type === 'CALL_ACCEPTED') {
      controller.hangup();
      expect(transport.send).toHaveBeenLastCalledWith({ type: 'HANGUP', target: 'PORTERIA', message: 'Llamada finalizada' });
    }
  });
  it.each(['accept', 'reject'] as const)('allows portería to %s apartment calls, ignoring foreign clients', (action) => {
    const { controller, transport, receive } = setup();
    receive({ type: 'CALL', from: 'APT_8', target: 'PORTERIA' });
    receive({ type: 'CALL', from: 'APT_4', target: 'PORTERIA_2' });
    expect(controller.getSnapshot().status).toBe('idle');
    receive({ type: 'CALL', from: 'APT_4', target: 'PORTERIA' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'incoming', call: { label: 'Apartamento 4', peer: 'APT_4' } });
    controller[action]();
    expect(transport.send).toHaveBeenLastCalledWith({
      type: action === 'accept' ? 'CALL_ACCEPTED' : 'CALL_REJECTED',
      target: 'APT_4', message: action === 'accept' ? 'Llamada aceptada' : 'Llamada rechazada',
    });
    if (action === 'accept') {
      receive({ type: 'HANGUP', from: 'APT_4', target: 'PORTERIA' });
      expect(controller.getSnapshot().status).toBe('ended');
    }
  });
  it('rejects another configured apartment while portería is busy without replacing the call', () => {
    const { controller, transport, receive } = setup();
    controller.config.apartments = [...controller.config.apartments, {
      complexId: 'complex-1', unitId: 'unit-5', clientId: 'APT_5', role: 'apartment', label: 'Apartamento 5',
    }];
    controller.callApartment('APT_4');
    receive({ type: 'CALL', from: 'APT_5', target: 'PORTERIA' });
    expect(transport.send).toHaveBeenLastCalledWith({ type: 'CALL_REJECTED', target: 'APT_5', message: 'Hay una llamada en curso.' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'calling', call: { peer: 'APT_4' } });
  });
  it('keeps an internal callId, sends no ID and accepts only the active peer', () => {
    const { controller, receive, transport } = setup();
    controller.callApartment('APT_4');
    const callId = controller.getSnapshot().call!.callId;
    expect(transport.send).toHaveBeenCalledWith({ type: 'CALL', target: 'APT_4', message: 'Llamando apartamento 4' });
    receive({ type: 'DELIVERED', target: 'APT_4' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'calling', delivered: true });
    receive({ type: 'CALL_ACCEPTED', from: 'APT_8', target: 'PORTERIA' });
    receive({ type: 'CALL_ACCEPTED', from: 'APT_4', target: 'PORTERIA_2' });
    expect(controller.getSnapshot().status).toBe('calling');
    receive({ type: 'CALL_ACCEPTED', from: 'APT_4', target: 'PORTERIA' });
    expect(controller.getSnapshot().status).toBe('accepted');
    expect(controller.getSnapshot().call?.callId).toBe(callId);
    controller.hangup();
    expect(transport.send).toHaveBeenLastCalledWith({ type: 'HANGUP', target: 'APT_4', message: 'Llamada finalizada' });
    expect(controller.getSnapshot().status).toBe('ended');
    receive({ type: 'CALL_ACCEPTED', from: 'APT_4', target: 'PORTERIA' });
    expect(controller.getSnapshot().status).toBe('ended');
  });
  it('handles rejection, backend errors and allows a new call with a new UUID', () => {
    const { controller, receive } = setup();
    controller.callApartment('APT_4');
    const firstId = controller.getSnapshot().call!.callId;
    receive({ type: 'CALL_REJECTED', from: 'APT_4', target: 'PORTERIA' });
    expect(controller.getSnapshot().status).toBe('rejected');
    controller.callApartment('APT_4');
    const secondId = controller.getSnapshot().call!.callId;
    expect(secondId).not.toBe(firstId);
    receive({ type: 'ERROR', message: 'Cliente no conectado' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'error', error: 'Cliente no conectado' });
  });
  it.each(['accept', 'reject'] as const)('handles incoming CALL and %s without changing its ID', (action) => {
    const { controller, receive, transport } = setup(false);
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4' });
    expect(controller.getSnapshot().status).toBe('incoming');
    const localCallId = controller.getSnapshot().call?.callId;
    controller[action]();
    expect(transport.send).toHaveBeenLastCalledWith({
      type: action === 'accept' ? 'CALL_ACCEPTED' : 'CALL_REJECTED', target: 'PORTERIA',
      message: action === 'accept' ? 'Llamada aceptada' : 'Llamada rechazada',
    });
    expect(controller.getSnapshot().status).toBe(action === 'accept' ? 'accepted' : 'rejected');
    expect(controller.getSnapshot().call?.callId).toBe(localCallId);
  });
  it('ignores foreign peers and repeated CALL while busy, and allows a new incoming call after hangup', () => {
    const { controller, receive, transport } = setup(false);
    receive({ type: 'CALL', from: 'PORTERIA_2', target: 'APT_4' });
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_8' });
    expect(controller.getSnapshot().status).toBe('idle');
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4' });
    const firstId = controller.getSnapshot().call?.callId;
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4' });
    expect(transport.send).not.toHaveBeenCalled();
    controller.accept();
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4' });
    expect(controller.getSnapshot().status).toBe('accepted');
    expect(controller.getSnapshot().call?.callId).toBe(firstId);
    receive({ type: 'HANGUP', from: 'PORTERIA', target: 'APT_4' });
    expect(controller.getSnapshot().status).toBe('ended');
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4' });
    expect(controller.getSnapshot().status).toBe('incoming');
    expect(controller.getSnapshot().call?.callId).not.toBe(firstId);
  });
  it('interrupts calls on disconnection without resuming after reconnect', () => {
    const { controller, emit, receive } = setup();
    controller.callApartment('APT_4');
    emit({ kind: 'status', status: 'disconnected' });
    emit({ kind: 'status', status: 'connected' });
    receive({ type: 'CALL_ACCEPTED', from: 'APT_4', target: 'PORTERIA' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'error', error: expect.stringContaining('interrumpida') });
  });
  it('does not report success when sending fails, including backend errors after hangup', () => {
    const { controller, receive, transport } = setup(false);
    receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4' });
    vi.mocked(transport.send).mockImplementationOnce(() => { throw new Error('Socket no disponible'); });
    controller.accept();
    expect(controller.getSnapshot()).toMatchObject({ status: 'incoming', error: 'Socket no disponible' });
    controller.accept();
    controller.hangup();
    receive({ type: 'ERROR', message: 'Cliente no conectado' });
    expect(controller.getSnapshot()).toMatchObject({ status: 'error', error: 'Cliente no conectado' });
  });
  it('sends HANGUP and closes the transport on session cleanup', () => {
    const { controller, transport } = setup();
    controller.callApartment('APT_4');
    controller.stop();
    expect(transport.send).toHaveBeenLastCalledWith({ type: 'HANGUP', target: 'APT_4', message: 'Llamada finalizada' });
    expect(transport.disconnect).toHaveBeenCalledOnce();
    expect(controller.getSnapshot()).toMatchObject({ status: 'idle', connection: 'disconnected' });
  });
});
