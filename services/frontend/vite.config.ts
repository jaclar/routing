import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3333,
    host: '0.0.0.0',
    proxy: {
      '/api/v1/route': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/api/v1/weather': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/api/v1/presets': {
        target: 'http://127.0.0.1:8000',
        changeOrigin: true,
      },
      '/api/v1/solve': {
        target: 'http://127.0.0.1:8000',
        changeOrigin: true,
      },
      '/api/v1/plot': {
        target: 'http://127.0.0.1:8000',
        changeOrigin: true,
      },
    },
  },
});
