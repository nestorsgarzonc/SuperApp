import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { StrictMode } from 'react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import App from '../../App';
import { AuthProvider as SessionProvider, useAuth as useSession } from '../../shared/auth/AuthContext';
import { AuthProvider, useAuth } from '../../features/auth/components/AuthContext';
import { disablePush } from '../notificaciones/push';
import { directory, endpoint, FakeWebSocket, session } from './test/fixtures';
import type { Role } from '../../shared/api/client';

vi.mock('../notificaciones/push', () => ({ disablePush: vi.fn(), syncPush: vi.fn() }));
vi.mock('../notificaciones/components/NotificationBell', () => ({ NotificationBell: () => null }));
vi.mock('../control-de-accesos/dev/DevPanel', () => ({ DevPanel: () => <aside aria-label="Herramientas de desarrollo" /> }));
vi.mock('../control-de-accesos/pages/home', () => ({ HomePage: () => <h1>Inicio existente</h1> }));
vi.mock('../control-de-accesos/pages/resident', () => ({
  AuthorizationsPage: () => <h1>Autorizaciones existentes</h1>,
  NewAuthorizationPage: () => <h1>Nueva autorización existente</h1>,
  AccessPassPage: () => <h1>Pase existente</h1>,
}));
vi.mock('../control-de-accesos/pages/building', () => ({ BuildingPage: () => <h1>Edificio existente</h1> }));
vi.mock('../control-de-accesos/pages/guard', () => ({
  ValidatePassPage: () => <h1>Validación existente</h1>,
  RegisterVisitorPage: () => <h1>Visitante existente</h1>,
  MovementPage: () => <h1>Movimiento existente</h1>,
}));
vi.mock('../control-de-accesos/pages/history', () => ({ HistoryPage: () => <h1>Historial existente</h1> }));
vi.mock('../notificaciones/pages/center', () => ({ NotificationCenterPage: () => <h1>Notificaciones existentes</h1> }));
vi.mock('../notificaciones/pages/preferences', () => ({ NotificationPreferencesPage: () => <h1>Preferencias existentes</h1> }));

function Login({ role = 'GUARD', path = '/citofonia' }: { role?: Role; path?: string }) {
  const { session: current, signIn } = useSession();
  const { signInDevelopment } = useAuth();
  const navigate = useNavigate();
  return !current && <button onClick={() => {
    const user = {
      ...session, role,
      memberships: [
        { complexId: session.complexId, complexName: session.complexName, role, unitId: session.unitId },
        { complexId: 'complex-2', complexName: 'Otro conjunto', role, unitId: 'unit-8' },
      ],
    };
    signIn(user);
    signInDevelopment(user);
    navigate(path);
  }}>Ingresar para prueba</button>;
}

beforeEach(() => {
  FakeWebSocket.instances = [];
  vi.stubGlobal('WebSocket', FakeWebSocket);
  vi.stubEnv('VITE_INTERCOM_WS_URL', endpoint);
  vi.stubEnv('VITE_INTERCOM_CLIENTS', JSON.stringify(directory));
  vi.stubEnv('VITE_INTERCOM_DEMO', 'false');
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  vi.mocked(disablePush).mockReset();
});

it.each([
  ['RESIDENT', '/', 'Inicio existente'],
  ['RESIDENT', '/autorizaciones', 'Autorizaciones existentes'],
  ['RESIDENT', '/autorizaciones/nueva', 'Nueva autorización existente'],
  ['RESIDENT', '/autorizaciones/pass-1', 'Pase existente'],
  ['RESIDENT', '/edificio', 'Edificio existente'],
  ['GUARD', '/porteria/validar', 'Validación existente'],
  ['GUARD', '/porteria/visitante', 'Visitante existente'],
  ['GUARD', '/porteria/movimiento', 'Movimiento existente'],
  ['GUARD', '/historial', 'Historial existente'],
  ['RESIDENT', '/notificaciones', 'Notificaciones existentes'],
  ['RESIDENT', '/notificaciones/preferencias', 'Preferencias existentes'],
] as const)('preserves the existing %s route %s in normal mode', (role, path, heading) => {
  render(<MemoryRouter initialEntries={['/login']}><SessionProvider><AuthProvider><Login role={role} path={path} /><App /></AuthProvider></SessionProvider></MemoryRouter>);
  fireEvent.click(screen.getByText('Ingresar para prueba'));
  expect(screen.getByRole('heading', { name: heading })).toBeInTheDocument();
  expect(screen.getByRole('combobox', { name: 'Conjunto activo' })).toBeInTheDocument();
  expect(screen.getByRole('complementary', { name: 'Herramientas de desarrollo' })).toBeInTheDocument();
  expect(screen.getAllByRole('link', { name: /Citofonía/ })).toHaveLength(2);
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
