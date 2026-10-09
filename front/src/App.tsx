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
import { AuthComplexSwitcher, AuthSessionBridge } from './features/auth/components/AuthIntegration';

function App() {
  const { identity, activeComplex, signOut } = useAuth();
  const { session, logout } = useSession();
  const onLogout = async (all = false) => { try { await signOut(all); } finally { logout(); } };
  if (identity && identity.complexes.length === 0) return <UnauthorizedPage />;
  if (identity && !activeComplex) return <Routes><Route path="/select-complex" element={<SelectComplexPage />} /><Route path="/session-expired" element={<SessionExpiredPage />} /><Route path="*" element={<Navigate to="/select-complex" replace />} /></Routes>;
  if (identity && activeComplex) {
    if (!session) return <AuthSessionBridge />;
    return <Routes>
      <Route element={<AppShell complexSlot={<AuthComplexSwitcher />} bellSlot={<NotificationBell />} onLogout={onLogout} />}>{accessControlRoutes}{notificationRoutes}</Route>
      <Route path="/unauthorized" element={<UnauthorizedPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>;
  }
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
