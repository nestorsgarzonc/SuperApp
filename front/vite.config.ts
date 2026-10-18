import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { loadEnv, type ProxyOptions } from 'vite';

// El front nunca llama a un ms directo: todo va por el API Gateway. En dev,
// /api se redirige a GATEWAY_URL o, si no está, a ACCESS_URL.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const apiProxy: Record<string, string | ProxyOptions> = {};
  if (env.GATEWAY_URL) {
    apiProxy['/api'] = { target: env.GATEWAY_URL, changeOrigin: true };
  } else {
    apiProxy['/api/v1/auth'] = { target: env.AUTH_URL || 'http://localhost:8081', changeOrigin: true };
    // Igual que el gateway: /api/v1/notifications/{proxy+} → ms de notificaciones /{proxy} (sin prefijo).
    apiProxy['/api/v1/notifications'] = {
      target: env.NOTIFICATIONS_URL || 'http://localhost:8083',
      changeOrigin: true,
      rewrite: (p) => p.replace(/^\/api\/v1\/notifications/, ''),
    };
    apiProxy['/api'] = { target: env.ACCESS_URL || 'http://localhost:8081', changeOrigin: true };
  }
  return {
    // En GitHub Pages el sitio vive en /<repo>/; en dev queda en la raíz.
    base: env.VITE_BASE_PATH || '/',
    plugins: [react()],
    server: { port: 5173, proxy: apiProxy },
    test: { environment: 'jsdom', setupFiles: './src/test/setup.ts', restoreMocks: true },
  };
});
