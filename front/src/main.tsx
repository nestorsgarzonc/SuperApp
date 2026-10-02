import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from './shared/auth/AuthContext';
import { DevLoginPage } from './modules/control-de-accesos/dev/DevLoginPage';
import { AppShell } from './shared/layout/AppShell';
import { ComplexSwitcher } from './modules/control-de-accesos/dev/ComplexSwitcher';
import { DevPanel } from './modules/control-de-accesos/dev/DevPanel';
import { accessControlRoutes } from './modules/control-de-accesos/routes';
import './shared/styles.css';

function App() {
  const { session } = useAuth();
  // Entrada de PRUEBAS de control-de-accesos; se reemplaza por el login del módulo auth.
  if (!session) return <DevLoginPage />;
  return (
    <Routes>
      <Route element={<AppShell complexSlot={<ComplexSwitcher />} />}>{accessControlRoutes}</Route>
      <Route path="*" element={<p style={{ padding: 24 }}>Página no encontrada</p>} />
    </Routes>
  );
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <App />
        {import.meta.env.DEV && <DevPanel />}
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
);
