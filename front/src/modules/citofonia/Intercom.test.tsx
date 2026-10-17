import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Link, MemoryRouter, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from '../../shared/auth/AuthContext';
import { IntercomProvider, useIntercom } from './IntercomContext';
import { IntercomPage } from './pages/intercom';
import { directory, endpoint, FakeWebSocket, session } from './test/fixtures';

const dialogMethods = {
  showModal: Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, 'showModal'),
  close: Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, 'close'),
};

function Harness() {
  const { session: current, signIn, logout } = useAuth();
  return <>
    <button onClick={() => signIn(session)}>Entrar residente</button>
    <button onClick={() => signIn({ ...session, role: 'GUARD', userId: 'guard-1', unitId: '' })}>Entrar portería</button>
    <button onClick={() => signIn({ ...session, complexId: 'complex-2', unitId: 'unit-8' })}>Cambiar conjunto</button>
    <button onClick={logout}>Cerrar sesión</button>
    {current && <IntercomProvider>
      <ImmediateLogout />
      <Link to="/">Inicio de prueba</Link><Link to="/citofonia">Citofonía</Link>
      <Routes>
        <Route path="/" element={<h1>Inicio de prueba</h1>} />
        <Route path="/citofonia" element={<IntercomPage />} />
      </Routes>
    </IntercomProvider>}
  </>;
}
function ImmediateLogout() {
  const { controller } = useIntercom();
  return <button onClick={() => controller?.stop()}>Iniciar cierre de sesión</button>;
}
function setup() {
  return render(<MemoryRouter><AuthProvider><Harness /></AuthProvider></MemoryRouter>);
}
function incoming() {
  act(() => FakeWebSocket.latest().receive({ type: 'CALL', from: 'PORTERIA', target: 'APT_4', message: 'Llamando apartamento 4' }));
}
function openSocket() {
  act(() => FakeWebSocket.latest().open());
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
  vi.stubEnv('VITE_INTERCOM_WS_URL', endpoint);
  vi.stubEnv('VITE_INTERCOM_CLIENTS', JSON.stringify(directory));
  Object.defineProperties(HTMLDialogElement.prototype, {
    showModal: { configurable: true, value: function (this: HTMLDialogElement) { this.setAttribute('open', ''); } },
    close: { configurable: true, value: function (this: HTMLDialogElement) { this.removeAttribute('open'); } },
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  for (const name of ['showModal', 'close'] as const) {
    const original = dialogMethods[name];
    if (original) Object.defineProperty(HTMLDialogElement.prototype, name, original);
    else Reflect.deleteProperty(HTMLDialogElement.prototype, name);
  }
});

describe('Intercom session UI', () => {
  it('receives a call outside the page, accepts, retains it during navigation and hangs up', () => {
    setup();
    fireEvent.click(screen.getByText('Entrar residente'));
    openSocket();
    incoming();
    const dialog = screen.getByRole('dialog', { name: 'Llamada entrante de portería' });
    expect(within(dialog).getByText('Portería está llamando')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Aceptar' }));
    expect(FakeWebSocket.latest().send).toHaveBeenLastCalledWith(JSON.stringify({ action: 'seendMessage', type: 'CALL_ACCEPTED', target: 'PORTERIA', message: 'Llamada aceptada' }));
    expect(screen.getByText('Llamada aceptada')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('link', { name: 'Abrir Citofonía' }));
    expect(screen.getByText('Llamada aceptada')).toBeInTheDocument();
    expect(FakeWebSocket.instances).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: 'Finalizar llamada' }));
    expect(screen.getByText('Llamada finalizada')).toBeInTheDocument();
    expect(FakeWebSocket.latest().send).toHaveBeenLastCalledWith(JSON.stringify({ action: 'seendMessage', type: 'HANGUP', target: 'PORTERIA', message: 'Llamada finalizada' }));
  });
  it('rejects incoming calls with the exact current AWS contract', () => {
    setup();
    fireEvent.click(screen.getByText('Entrar residente'));
    openSocket();
    incoming();
    fireEvent.click(screen.getByRole('button', { name: 'Rechazar' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(FakeWebSocket.latest().send).toHaveBeenLastCalledWith(JSON.stringify({ action: 'seendMessage', type: 'CALL_REJECTED', target: 'PORTERIA', message: 'Llamada rechazada' }));
  });
  it('calls a selected apartment and handles rejection, errors and remote hangup', () => {
    setup();
    fireEvent.click(screen.getByText('Entrar portería'));
    fireEvent.click(screen.getByRole('link', { name: 'Citofonía' }));
    expect(screen.getByRole('button', { name: 'Llamar' })).toBeDisabled();
    openSocket();
    fireEvent.change(screen.getByLabelText('Apartamento'), { target: { value: 'APT_4' } });
    fireEvent.click(screen.getByRole('button', { name: 'Llamar' }));
    const socket = FakeWebSocket.latest();
    expect(JSON.parse(socket.send.mock.calls[0]![0])).toEqual({ action: 'seendMessage', type: 'CALL', target: 'APT_4', message: 'Llamando apartamento 4' });
    expect(screen.getByText('Esperando respuesta...')).toBeInTheDocument();
    act(() => socket.receive({ type: 'CALL_REJECTED', from: 'APT_4', target: 'PORTERIA' }));
    expect(screen.getByText('Llamada rechazada')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Llamar' }));
    act(() => socket.receive({ type: 'ERROR', message: 'Cliente no conectado' }));
    expect(screen.getByText('Cliente no conectado')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Llamar' }));
    act(() => socket.receive({ type: 'CALL_ACCEPTED', from: 'APT_4', target: 'PORTERIA' }));
    expect(screen.getByText('Llamada aceptada')).toBeInTheDocument();
    act(() => socket.receive({ type: 'HANGUP', from: 'APT_4', target: 'PORTERIA' }));
    expect(screen.getByText('Llamada finalizada')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Llamar' })).toBeEnabled();
  });
  it('changes identities on complex switch, closes on logout and can log in again', () => {
    setup();
    fireEvent.click(screen.getByText('Entrar residente'));
    const first = FakeWebSocket.latest();
    openSocket();
    incoming();
    fireEvent.click(screen.getByText('Cambiar conjunto'));
    expect(first.close).toHaveBeenCalledOnce();
    expect(first.send).toHaveBeenLastCalledWith(JSON.stringify({ action: 'seendMessage', type: 'HANGUP', target: 'PORTERIA', message: 'Llamada finalizada' }));
    expect(new URL(FakeWebSocket.latest().url).searchParams.get('clientId')).toBe('APT_8');
    const second = FakeWebSocket.latest();
    fireEvent.click(screen.getByText('Cerrar sesión'));
    expect(second.close).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByText('Entrar residente'));
    expect(FakeWebSocket.instances).toHaveLength(3);
  });
  it('closes immediately before asynchronous logout finishes', () => {
    setup();
    fireEvent.click(screen.getByText('Entrar residente'));
    const socket = FakeWebSocket.latest();
    fireEvent.click(screen.getByText('Iniciar cierre de sesión'));
    expect(socket.close).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole('link', { name: 'Citofonía' }));
    expect(FakeWebSocket.instances).toHaveLength(1);
  });
  it('shows missing configuration without affecting the rest of the app', () => {
    vi.stubEnv('VITE_INTERCOM_WS_URL', '');
    setup();
    fireEvent.click(screen.getByText('Entrar residente'));
    expect(FakeWebSocket.instances).toHaveLength(0);
    fireEvent.click(screen.getByRole('link', { name: 'Citofonía' }));
    expect(screen.getByText(/Configura VITE_INTERCOM_WS_URL/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Inicio de prueba' })).toBeInTheDocument();
  });
});
