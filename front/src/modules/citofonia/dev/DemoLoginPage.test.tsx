import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import App from '../../../App';
import { AuthProvider } from '../../../features/auth/components/AuthContext';
import { AuthProvider as SessionProvider } from '../../../shared/auth/AuthContext';
import { createDemoSession, isIntercomDemo } from './demo';
import { directory, endpoint, FakeWebSocket } from '../test/fixtures';

function renderApp(path = '/') {
  return render(<MemoryRouter initialEntries={[path]}><SessionProvider><AuthProvider><App /></AuthProvider></SessionProvider></MemoryRouter>);
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
  vi.stubEnv('VITE_INTERCOM_DEMO', 'true');
  vi.stubEnv('VITE_INTERCOM_WS_URL', endpoint);
  vi.stubEnv('VITE_INTERCOM_CLIENTS', JSON.stringify(directory));
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe('standalone intercom demo', () => {
  it.each([
    ['Portería', 'PORTERIA', 'porter'],
    ['Apartamento 4', 'APT_4', 'apartment'],
  ])('creates a %s session without any HTTP requests and logs out locally', async (label, clientId, role) => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch');
    renderApp();
    expect(screen.getByRole('heading', { name: 'Citofonía · Prueba' })).toBeInTheDocument();
    expect(FakeWebSocket.instances).toHaveLength(0);
    fireEvent.click(screen.getByRole('button', { name: new RegExp(`${label}.*${clientId}$`) }));
    expect(screen.getByRole('heading', { name: 'Citofonía' })).toBeInTheDocument();
    const socket = FakeWebSocket.latest();
    const params = new URL(socket.url).searchParams;
    expect(params.get('clientId')).toBe(clientId);
    expect(params.get('role')).toBe(role);
    act(() => socket.open());
    expect(screen.queryByTestId('notification-bell')).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /Historial|Accesos|Avisos/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getAllByRole('button', { name: 'Cerrar sesión' })[0]!);
    fireEvent.click(screen.getAllByRole('button', { name: 'Cerrar sesión' }).at(-1)!);
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Citofonía · Prueba' })).toBeInTheDocument());
    expect(socket.close).toHaveBeenCalledOnce();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('prevents non-intercom routes from mounting HTTP-dependent modules', () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch');
    renderApp('/notificaciones');
    fireEvent.click(screen.getByRole('button', { name: /Portería.*PORTERIA$/ }));
    expect(screen.getByRole('heading', { name: 'Citofonía' })).toBeInTheDocument();
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it('shows configuration errors without connecting', () => {
    vi.stubEnv('VITE_INTERCOM_CLIENTS', 'not-json');
    renderApp();
    expect(screen.getByText(/directorio JSON válido/)).toBeInTheDocument();
    expect(FakeWebSocket.instances).toHaveLength(0);
  });

  it('does not expose demo login in a normal run', () => {
    vi.stubEnv('VITE_INTERCOM_DEMO', 'false');
    renderApp('/login');
    expect(screen.getByRole('heading', { name: 'Inicia sesión' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Citofonía · Prueba' })).not.toBeInTheDocument();
    expect(() => createDemoSession(directory[0])).toThrow(/no está habilitado/);
  });

  it('cannot enable demo sessions in a production build even with the flag', () => {
    vi.stubEnv('DEV', false);
    expect(isIntercomDemo()).toBe(false);
    expect(() => createDemoSession(directory[0])).toThrow(/no está habilitado/);
  });
});
