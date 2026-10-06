import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  return {
    plugins: [react()],
    server: {
      port: 5173,
      // Con gateway todo va a GATEWAY_URL. Sin él, auth y control-de-accesos son servicios distintos:
      // /api/v1/auth/* va a AUTH_URL y el resto a ACCESS_URL.
      proxy: env.GATEWAY_URL
        ? { '/api': env.GATEWAY_URL }
        : {
            ...(env.AUTH_URL ? { '/api/v1/auth': env.AUTH_URL } : {}),
            '/api': env.ACCESS_URL || 'http://localhost:8081',
          },
    },
    test: { environment: 'jsdom', setupFiles: './src/test/setup.ts', restoreMocks: true },
  };
});
