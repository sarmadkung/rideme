import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  // Environment lives at the repository root so one .env.local serves every surface.
  envDir: '../../',
  // 5173 is the admin console. A merchant and an operator are different people
  // with different sessions, and running both at once is the normal case.
  server: { port: 5174 },
  test: {
    environment: 'jsdom',
    env: {
      VITE_APP_ENV: 'test',
      VITE_API_BASE_URL: 'http://localhost:8080',
    },
  },
});
