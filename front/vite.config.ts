import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

// El front nunca llama a un ms directo: en producción todo va por el API
// Gateway. En dev, /api se redirige a GATEWAY_URL; mientras el gateway no
// exista, al ms de control-de-accesos (ACCESS_URL).
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  return {
    plugins: [react()],
    server: {
      port: 5173,
      proxy: { '/api': env.GATEWAY_URL || env.ACCESS_URL || 'http://localhost:8081' },
    },
  };
});
