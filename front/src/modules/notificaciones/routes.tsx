import { Route } from 'react-router-dom';
import { NotificationCenterPage } from './pages/center';
import { NotificationPreferencesPage } from './pages/preferences';

/** Rutas del módulo; se montan dentro del AppShell. Disponibles para todos los roles. */
export const notificationRoutes = (
  <>
    <Route path="notificaciones" element={<NotificationCenterPage />} />
    <Route path="notificaciones/preferencias" element={<NotificationPreferencesPage />} />
  </>
);
