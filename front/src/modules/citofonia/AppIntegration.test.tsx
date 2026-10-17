import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { StrictMode } from 'react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import App from '../../App';
import { AuthProvider as SessionProvider, useAuth as useSession } from '../../shared/auth/AuthContext';
import { AuthProvider, useAuth } from '../../features/auth/components/AuthContext';
import { disablePush } from '../notificaciones/push';
import { directory, endpoint, FakeWebSocket, session } from './test/fixtures';

vi.mock('../notificaciones/push', () => ({ disablePush: vi.fn(), syncPush: vi.fn() }));
vi.mock('../notificaciones/components/NotificationBell', () => ({ NotificationBell: () => null }));

function Login() {
  const { session: current, signIn } = useSession();
  const { signInDevelopment } = useAuth();
  const navigate = useNavigate();
  return !current && <button onClick={() => {
    const guard = { ...session, role: 'GUARD' as const, userId: 'guard-1', unitId: '' };
    signIn(guard);
    signInDevelopment(guard);
    navigate('/citofonia');
  }}>Ingresar para prueba</button>;
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
  vi.stubEnv('VITE_INTERCOM_WS_URL', endpoint);
  vi.stubEnv('VITE_INTERCOM_CLIENTS', JSON.stringify(directory));
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.mocked(disablePush).mockReset();
});

it('integrates desktop/mobile navigation and disconnects before asynchronous logout completes', async () => {
  let finishPush: (() => void) | undefined;
  vi.mocked(disablePush).mockImplementation(() => new Promise<void>((resolve) => { finishPush = resolve; }));
  render(<StrictMode><MemoryRouter initialEntries={['/login']}><SessionProvider><AuthProvider><Login /><App /></AuthProvider></SessionProvider></MemoryRouter></StrictMode>);
  expect(FakeWebSocket.instances).toHaveLength(0);
  fireEvent.click(screen.getByText('Ingresar para prueba'));
  expect(screen.getAllByRole('link', { name: /Citofonía/ })).toHaveLength(2);
  expect(screen.getByRole('heading', { name: 'Citofonía' })).toBeInTheDocument();
  const socket = FakeWebSocket.latest();
  expect(FakeWebSocket.instances.filter((candidate) => candidate.readyState !== FakeWebSocket.CLOSED)).toHaveLength(1);
  act(() => socket.open());
  fireEvent.change(screen.getByLabelText('Apartamento'), { target: { value: 'APT_4' } });
  fireEvent.click(screen.getByRole('button', { name: 'Llamar' }));
  fireEvent.click(screen.getAllByRole('button', { name: 'Cerrar sesión' })[0]!);
  fireEvent.click(screen.getAllByRole('button', { name: 'Cerrar sesión' }).at(-1)!);
  expect(socket.close).toHaveBeenCalledOnce();
  expect(socket.send).toHaveBeenLastCalledWith(expect.stringContaining('"type":"HANGUP"'));
  expect(disablePush).toHaveBeenCalledOnce();
  await act(async () => finishPush?.());
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Inicia sesión' })).toBeInTheDocument());
});
