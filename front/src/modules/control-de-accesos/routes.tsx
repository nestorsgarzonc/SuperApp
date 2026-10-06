import { Navigate, Route } from 'react-router-dom';
import { useAuth } from '../../shared/auth/AuthContext';
import type { Role } from '../../shared/api/client';
import { HomePage } from './pages/home';
import { AccessPassPage, AuthorizationsPage, NewAuthorizationPage } from './pages/resident';
import { MovementPage, RegisterVisitorPage, ValidatePassPage } from './pages/guard';
import { HistoryPage } from './pages/history';
import { BuildingPage } from './pages/building';
import type { ReactNode } from 'react';
import { useAuth as useIdentity } from '../../features/auth/components/AuthContext';

function RequireRole({ roles, permission, children }: { roles: Role[]; permission: string; children: ReactNode }) {
  const { session } = useAuth();
  const { identity } = useIdentity();
  return session && roles.includes(session.role) && identity?.permissions.includes(permission) ? <>{children}</> : <Navigate to="/unauthorized" replace />;
}

const GATE: Role[] = ['GUARD', 'ADMIN'];

/** Rutas del módulo; se montan dentro del AppShell. */
export const accessControlRoutes = (
  <>
    <Route index element={<RequireRole roles={['RESIDENT', 'OWNER', 'GUARD', 'ADMIN']} permission="access.home.read"><HomePage /></RequireRole>} />
    <Route path="autorizaciones" element={<RequireRole roles={['RESIDENT', 'OWNER']} permission="access.authorizations.read"><AuthorizationsPage /></RequireRole>} />
    <Route path="autorizaciones/nueva" element={<RequireRole roles={['RESIDENT', 'OWNER']} permission="access.authorizations.create"><NewAuthorizationPage /></RequireRole>} />
    <Route path="autorizaciones/:id" element={<RequireRole roles={['RESIDENT', 'OWNER']} permission="access.authorizations.read"><AccessPassPage /></RequireRole>} />
    <Route path="edificio" element={<RequireRole roles={['RESIDENT', 'OWNER']} permission="access.building.read"><BuildingPage /></RequireRole>} />
    <Route path="porteria/validar" element={<RequireRole roles={GATE} permission="access.pass.validate"><ValidatePassPage /></RequireRole>} />
    <Route path="porteria/visitante" element={<RequireRole roles={GATE} permission="access.visitor.register"><RegisterVisitorPage /></RequireRole>} />
    <Route path="porteria/movimiento" element={<RequireRole roles={GATE} permission="access.movement.register"><MovementPage /></RequireRole>} />
    <Route path="historial" element={<RequireRole roles={GATE} permission="access.history.read"><HistoryPage /></RequireRole>} />
  </>
);
