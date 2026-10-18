import { useEffect } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { ProtectedRoute, PublicOnly } from './features/auth/routes/guards';
import { LoginPage } from './features/auth/pages/LoginPage';
import { BlockedPage, ForgotPasswordPage, ForgotSentPage, ResetPasswordPage, ResetSuccessPage, SelectComplexPage, SessionExpiredPage, SplashPage, UnauthorizedPage } from './features/auth/pages/FlowPages';
import { useAuth } from './features/auth/components/AuthContext';
import { useAuth as useSession } from './shared/auth/AuthContext';
import { AppShell } from './shared/layout/AppShell';
import { accessControlRoutes } from './modules/control-de-accesos/routes';
import { notificationRoutes } from './modules/notificaciones/routes';
import { NotificationBell } from './modules/notificaciones/components/NotificationBell';
import { disablePush, syncPush } from './modules/notificaciones/push';
import { AuthComplexSwitcher, AuthSessionBridge } from './features/auth/components/AuthIntegration';
import { DevLoginPage } from './modules/control-de-accesos/dev/DevLoginPage';
import { DEV_LOGIN_ENABLED } from './modules/control-de-accesos/dev/devApi';
import { ComplexSwitcher } from './modules/control-de-accesos/dev/ComplexSwitcher';
import { DevPanel } from './modules/control-de-accesos/dev/DevPanel';
import { IntercomProvider, useIntercom } from './modules/citofonia/IntercomContext';
import { intercomRoutes } from './modules/citofonia/routes';
import { isIntercomDemo } from './modules/citofonia/dev/demo';
import { DemoLoginPage } from './modules/citofonia/dev/DemoLoginPage';

function SessionShell({ onLogout }: { onLogout: (all?: boolean) => Promise<void> }) {
  const { controller } = useIntercom();
  const { developmentSession } = useAuth();
  const demo = isIntercomDemo();
  return <AppShell intercomOnly={demo} complexSlot={developmentSession && !demo ? <ComplexSwitcher /> : <AuthComplexSwitcher />} bellSlot={demo ? undefined : <NotificationBell />} onLogout={async (all) => {
    controller?.stop();
    await onLogout(all);
  }} />;
}

function App() {
  const { identity, activeComplex, developmentSession, signOut } = useAuth();
  const { session, logout } = useSession();
  // Antes de signOut: el unregister del push necesita la sesión vigente.
  const demo = isIntercomDemo();
  const onLogout = async (all = false) => { try { if (!demo) await disablePush(); await signOut(all); } finally { logout(); } };
  const userId = session?.userId;
  useEffect(() => { if (userId && !demo) void syncPush(); }, [userId, demo]);
  if (identity && identity.complexes.length === 0) return <UnauthorizedPage />;
  if (identity && !activeComplex) return <Routes><Route path="/select-complex" element={<SelectComplexPage />} /><Route path="/session-expired" element={<SessionExpiredPage />} /><Route path="*" element={<Navigate to="/select-complex" replace />} /></Routes>;
  if (identity && activeComplex) {
    if (!session) return <AuthSessionBridge />;
    return <IntercomProvider><Routes>
      <Route element={<SessionShell onLogout={onLogout} />}>{!demo && accessControlRoutes}{!demo && notificationRoutes}{intercomRoutes}</Route>
      <Route path="/unauthorized" element={<UnauthorizedPage />} />
      <Route path="*" element={<Navigate to={demo ? '/citofonia' : '/'} replace />} />
    </Routes>
    {import.meta.env.DEV && developmentSession && !demo && <DevPanel />}
    </IntercomProvider>;
  }
  if (demo) return <DemoLoginPage />;
  return <>
    <Routes>
      <Route path="/" element={<Navigate to="/splash" replace />} />
      <Route path="/splash" element={<SplashPage />} />
      <Route path="/login" element={<PublicOnly><LoginPage /></PublicOnly>} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/forgot-password/sent" element={<ForgotSentPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/reset-password/success" element={<ResetSuccessPage />} />
      <Route path="/session-expired" element={<SessionExpiredPage />} />
      {DEV_LOGIN_ENABLED && <Route path="/dev-login" element={<DevLoginPage />} />}
      <Route path="/unauthorized" element={<UnauthorizedPage />} />
      <Route path="/account-blocked" element={<BlockedPage />} />
      <Route element={<ProtectedRoute />}>
        <Route path="/select-complex" element={<SelectComplexPage />} />
        <Route path="/dashboard" element={<Navigate to="/" replace />} />
      </Route>
      <Route path="*" element={<Navigate to="/login" replace />} />
    </Routes>
  </>;
}
export default App;
