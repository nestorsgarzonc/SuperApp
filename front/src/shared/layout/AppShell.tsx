import type { ReactNode } from 'react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import type { Role } from '../api/client';

interface NavItem {
  to: string;
  label: string;
  icon: string;
  end?: boolean;
}

const NAV: Record<Role, NavItem[]> = {
  RESIDENT: [
    { to: '/', label: 'Inicio', icon: '⌂', end: true },
    { to: '/autorizaciones', label: 'Accesos', icon: '◇' },
    { to: '/edificio', label: 'Edificio', icon: '⇄' },
  ],
  GUARD: [
    { to: '/', label: 'Inicio', icon: '⌂', end: true },
    { to: '/porteria/validar', label: 'Validar', icon: '◇' },
    { to: '/porteria/movimiento', label: 'Ingreso/Salida', icon: '⇄' },
    { to: '/historial', label: 'Historial', icon: '☰' },
  ],
  ADMIN: [
    { to: '/', label: 'Inicio', icon: '⌂', end: true },
    { to: '/historial', label: 'Historial', icon: '☰' },
  ],
};

/** `complexSlot`: el chip/selector del conjunto activo (lo aporta el módulo de auth). */
export function AppShell({ complexSlot }: { complexSlot?: ReactNode }) {
  const { profile, session, logout } = useAuth();
  const nav = useNavigate();
  const { pathname } = useLocation();
  if (!profile) return null;
  const items = NAV[profile.role];

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
          <button onClick={logout}>Cerrar sesión</button>
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
          <div className="user">
            <div className="who">
              {profile.name}
              <small>{profile.subtitle}</small>
            </div>
            <button className="avatar" style={{ border: 0, cursor: 'pointer' }} title="Cerrar sesión" onClick={logout}>
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
            {i.label}
          </NavLink>
        ))}
      </nav>
    </div>
  );
}
