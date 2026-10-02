import { Navigate, Route } from 'react-router-dom';
import { useAuth } from '../../shared/auth/AuthContext';
import type { Role } from '../../shared/api/client';
import { HomePage } from './pages/home';
import { AccessPassPage, AuthorizationsPage, NewAuthorizationPage } from './pages/resident';
import { MovementPage, RegisterVisitorPage, ValidatePassPage } from './pages/guard';
import { HistoryPage } from './pages/history';
import { BuildingPage } from './pages/building';
import type { ReactNode } from 'react';

function RequireRole({ roles, children }: { roles: Role[]; children: ReactNode }) {
  const { session } = useAuth();
  return session && roles.includes(session.role) ? <>{children}</> : <Navigate to="/" replace />;
}

const GATE: Role[] = ['GUARD', 'ADMIN'];

/** Rutas del módulo; se montan dentro del AppShell. */
export const accessControlRoutes = (
  <>
    <Route index element={<HomePage />} />
    <Route path="autorizaciones" element={<RequireRole roles={['RESIDENT']}><AuthorizationsPage /></RequireRole>} />
    <Route path="autorizaciones/nueva" element={<RequireRole roles={['RESIDENT']}><NewAuthorizationPage /></RequireRole>} />
    <Route path="autorizaciones/:id" element={<RequireRole roles={['RESIDENT']}><AccessPassPage /></RequireRole>} />
    <Route path="edificio" element={<RequireRole roles={['RESIDENT']}><BuildingPage /></RequireRole>} />
    <Route path="porteria/validar" element={<RequireRole roles={GATE}><ValidatePassPage /></RequireRole>} />
    <Route path="porteria/visitante" element={<RequireRole roles={GATE}><RegisterVisitorPage /></RequireRole>} />
    <Route path="porteria/movimiento" element={<RequireRole roles={GATE}><MovementPage /></RequireRole>} />
    <Route path="historial" element={<RequireRole roles={GATE}><HistoryPage /></RequireRole>} />
  </>
);
