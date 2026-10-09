import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { loadEnv, type ProxyOptions } from 'vite';

// Rutas públicas del ms de notificaciones (docs/api-gateway.md, tabla de rutas).
const NOTIFICATION_PATHS = [
  '/api/v1/users/me/notifications',
  '/api/v1/notifications',
  '/api/v1/me/notification-preferences',
  '/api/v1/me/devices',
];

/** Lee los claims del JWT sin validarlo: solo para emular el gateway en dev. */
function claims(authorization?: string): Record<string, unknown> {
  try {
    const payload = authorization?.replace(/^Bearer\s+/i, '').split('.')[1] ?? '';
    return JSON.parse(Buffer.from(payload, 'base64url').toString('utf8'));
  } catch {
    return {};
  }
}

/**
 * Mientras el API Gateway no exista, el dev server hace su papel para
 * notificaciones: quita /api/v1 e inyecta la identidad (X-User-ID,
 * X-Complex-Role) desde el token, igual que lo hará el gateway. El ms debe
 * correr con ENV=development (sin GATEWAY_SECRET).
 */
function notificationsProxy(target: string): ProxyOptions {
  return {
    target,
    changeOrigin: true,
    rewrite: (path) => path.replace(/^\/api\/v1/, ''),
    configure: (proxy) => {
      proxy.on('proxyReq', (proxyReq, req) => {
        const c = claims(req.headers.authorization);
        proxyReq.removeHeader('x-user-id');
        proxyReq.removeHeader('x-complex-role');
        const userId = c.user_id ?? c.sub;
        if (typeof userId === 'string') proxyReq.setHeader('X-User-ID', userId);
        if (typeof c.role === 'string') proxyReq.setHeader('X-Complex-Role', c.role);
      });
    },
  };
}

// El front nunca llama a un ms directo: en producción todo va por el API
// Gateway. En dev, /api se redirige a GATEWAY_URL; mientras el gateway no
// exista, al ms de control-de-accesos (ACCESS_URL) y, para las rutas de
// notificaciones, al ms de notificaciones (NOTIFICACIONES_URL).
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const gateway = env.GATEWAY_URL;
  const notifications = gateway
    ? {}
    : Object.fromEntries(
        NOTIFICATION_PATHS.map((p) => [p, notificationsProxy(env.NOTIFICACIONES_URL || 'http://localhost:8083')]),
      );
  return {
    plugins: [react()],
    server: {
      port: 5173,
      proxy: { ...notifications, '/api': gateway || env.ACCESS_URL || 'http://localhost:8081' },
    },
    test: { environment: 'jsdom', setupFiles: './src/test/setup.ts', restoreMocks: true },
  };
});
