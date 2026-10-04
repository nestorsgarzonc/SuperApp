import { Navigate, Outlet, useLocation } from 'react-router-dom';
import type { ReactNode } from 'react';
import { useAuth } from '../components/AuthContext';

export function ProtectedRoute({ permission, roles }: { permission?: string; roles?: string[] }) {
  const { identity } = useAuth();
  const location = useLocation();
  if (!identity) return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  if (permission && !identity.permissions.includes(permission)) return <Navigate to="/unauthorized" replace />;
  if (roles && !roles.some((role) => identity.roles.includes(role))) return <Navigate to="/unauthorized" replace />;
  return <Outlet />;
}
export function PublicOnly({ children }: { children: ReactNode }) {
  const { identity } = useAuth();
  return identity ? <Navigate to="/dashboard" replace /> : children;
}
