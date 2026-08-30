import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const routingUrl = env.ROUTING_SERVICE_URL || 'http://127.0.0.1:8080';
  const vppUrl = env.VPP_SERVICE_URL || 'http://127.0.0.1:8000';

  const makeProxy = (target: string, serviceName: string) => ({
    target,
    changeOrigin: true,
    configure: (proxy: any) => {
      proxy.on('error', (err: any, req: any, res: any) => {
        if (res && res.writeHead && !res.headersSent) {
          res.writeHead(503, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              error: `${serviceName} unreachable at ${target}`,
              details: err.message,
              hint: `Ensure ${serviceName} is running. If using Docker, run 'pnpm dev:backend'.`,
            })
          );
        }
      });
    },
  });

  return {
    plugins: [react()],
    server: {
      port: 3333,
      host: '0.0.0.0',
      watch: {
        usePolling: process.env.VITE_USE_POLLING === 'true',
      },
      proxy: {
        '/api/v1/route': makeProxy(routingUrl, 'Routing Service (Go)'),
        '/api/v1/weather': makeProxy(routingUrl, 'Routing Service (Go)'),
        '/api/v1/landmask': makeProxy(routingUrl, 'Routing Service (Go)'),
        '/health': makeProxy(routingUrl, 'Routing Service (Go)'),
        '/api/v1/health': makeProxy(routingUrl, 'Routing Service (Go)'),
        '/api/v1/presets': makeProxy(vppUrl, 'VPP Service (Python)'),
        '/api/v1/solve': makeProxy(vppUrl, 'VPP Service (Python)'),
        '/api/v1/plot': makeProxy(vppUrl, 'VPP Service (Python)'),
        '/api/v1/export': makeProxy(vppUrl, 'VPP Service (Python)'),
      },
    },
  };
});
