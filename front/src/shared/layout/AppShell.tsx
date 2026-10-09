import { useState, type ReactNode } from 'react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import type { Role } from '../api/client';
import { useAuth as useIdentity } from '../../features/auth/components/AuthContext';

interface NavItem {
  to: string;
  label: string;
  icon: string;
  /** Etiqueta para la barra inferior (móvil), si difiere. */
  short?: string;
  end?: boolean;
  /** Sin permiso: visible para cualquier usuario autenticado. */
  permission?: string;
}

const HOME: NavItem = { to: '/', label: 'Inicio', icon: '\u2302', end: true, permission: 'access.home.read' };
const AUTHORIZATIONS: NavItem = { to: '/autorizaciones', label: 'Accesos', icon: '\u25c7', permission: 'access.authorizations.read' };
const BUILDING: NavItem = { to: '/edificio', label: 'Edificio', icon: '\u21c4', permission: 'access.building.read' };
const HISTORY: NavItem = { to: '/historial', label: 'Historial', icon: '\u2630', permission: 'access.history.read' };
const NOTIFICATIONS: NavItem = { to: '/notificaciones', label: 'Notificaciones', short: 'Avisos', icon: '\u2709' };
const NAV: Record<Role, NavItem[]> = {
  RESIDENT: [HOME, AUTHORIZATIONS, BUILDING, NOTIFICATIONS],
  OWNER: [HOME, AUTHORIZATIONS, BUILDING, NOTIFICATIONS],
  GUARD: [HOME, { to: '/porteria/validar', label: 'Validar', icon: '\u25c7', permission: 'access.pass.validate' }, { to: '/porteria/movimiento', label: 'Ingreso/Salida', icon: '\u21c4', permission: 'access.movement.register' }, HISTORY, NOTIFICATIONS],
  ADMIN: [HOME, HISTORY, NOTIFICATIONS],
};

/**
 * `complexSlot`: el chip/selector del conjunto activo (lo aporta el módulo de auth).
 * `bellSlot`: la campana de notificaciones (la aporta el módulo de notificaciones).
 */
export function AppShell({ complexSlot, bellSlot, onLogout }: { complexSlot?: ReactNode; bellSlot?: ReactNode; onLogout?: (all?: boolean) => Promise<void> }) {
  const { profile, session } = useAuth();
  const { identity } = useIdentity();
  const nav = useNavigate();
  const { pathname } = useLocation();
  const [confirmLogout, setConfirmLogout] = useState(false);
  const [logoutAll, setLogoutAll] = useState(false);
  if (!profile) return null;
  const items = NAV[profile.role].filter(({ permission }) => !permission || identity?.permissions.includes(permission));

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          SuperApp<small>Comunicación y gestión residencial</small>
        </div>
        <nav className="side-nav">
          {items.map((i) => (
            <NavLink key={i.to} to={i.to} end={i.end} className="side-link">
              {i.label}
            </NavLink>
          ))}
        </nav>
        <div className="side-foot">
          <button onClick={() => setConfirmLogout(true)}>Cerrar sesión</button>
        </div>
      </aside>

      <div className="main">
        <header className="topbar">
          {pathname !== '/' && (
            <button className="back-btn mobile-only" aria-label="Volver" onClick={() => nav(-1)}>
              ‹
            </button>
          )}
          {complexSlot ?? <span className="chip">{session?.complexName}</span>}
          <span className="spacer" />
          {bellSlot}
          <div className="user">
            <div className="who">
              {profile.name}
              <small>{profile.subtitle}</small>
            </div>
            <button className="avatar" title="Cerrar sesión" onClick={() => setConfirmLogout(true)}>
              {profile.name[0]}
            </button>
          </div>
        </header>
        <main className="content">
          <Outlet />
        </main>
      </div>

      <nav className="bottom-nav">
        {items.map((i) => (
          <NavLink key={i.to} to={i.to} end={i.end}>
            <span className="ico">{i.icon}</span>
            {i.short ?? i.label}
          </NavLink>
        ))}
      </nav>
      {confirmLogout && <div className="dialog-backdrop" role="presentation"><section className="dialog" role="dialog" aria-modal="true" aria-labelledby="logout-title"><h2 id="logout-title">¿Quieres cerrar sesión?</h2><p>Podrás volver a ingresar en cualquier momento.</p><label className="check-row"><input type="checkbox" checked={logoutAll} onChange={(event) => setLogoutAll(event.target.checked)}/><span className="check-box" aria-hidden="true"/><span>Cerrar sesión en todos mis dispositivos</span></label><div className="dialog__actions"><button className="button button--secondary" onClick={() => setConfirmLogout(false)}>Cancelar</button><button className="button button--danger" onClick={() => { void onLogout?.(logoutAll); }}>Cerrar sesión</button></div></section></div>}
    </div>
  );
}
