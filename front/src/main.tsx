import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import App from './App';
import { AuthProvider } from './features/auth/components/AuthContext';
import { AuthProvider as SessionProvider } from './shared/auth/AuthContext';
import './shared/styles.css';
import './styles/global.css';

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } } });
const start = async () => {
  if (import.meta.env.VITE_USE_MOCK_API === 'true') {
    try {
      const { worker } = await import('./test/browser');
      await worker.start({ onUnhandledRequest: 'bypass' });
    } catch {
      // La pantalla sigue accesible si un navegador local bloquea el worker mock.
    }
  }
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode><QueryClientProvider client={queryClient}><BrowserRouter><SessionProvider><AuthProvider><App /></AuthProvider></SessionProvider></BrowserRouter></QueryClientProvider></React.StrictMode>,
  );
};
void start();
