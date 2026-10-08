import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { loadEnv, type ProxyOptions } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const apiProxy: Record<string, string | ProxyOptions> = {};
  if (env.GATEWAY_URL) {
    apiProxy['/api'] = { target: env.GATEWAY_URL, changeOrigin: true };
  } else {
    apiProxy['/api/v1/auth'] = { target: env.AUTH_URL || 'http://localhost:8081', changeOrigin: true };
    apiProxy['/api'] = { target: env.ACCESS_URL || 'http://localhost:8081', changeOrigin: true };
  }
  return {
    plugins: [react()],
    server: { port: 5173, proxy: apiProxy },
    test: { environment: 'jsdom', setupFiles: './src/test/setup.ts', restoreMocks: true },
  };
});
