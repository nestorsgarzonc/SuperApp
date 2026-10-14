import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';

// El front nunca llama a un ms directo: todo va por el API Gateway. En dev,
// /api se redirige a GATEWAY_URL o, si no está, a ACCESS_URL.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  return {
    // En GitHub Pages el sitio vive en /<repo>/; en dev queda en la raíz.
    base: env.VITE_BASE_PATH || '/',
    plugins: [react()],
    server: {
      port: 5173,
      proxy: { '/api': env.GATEWAY_URL || env.ACCESS_URL || 'http://localhost:8081' },
    },
    test: { environment: 'jsdom', setupFiles: './src/test/setup.ts', restoreMocks: true },
  };
});
