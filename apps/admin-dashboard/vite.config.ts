import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  // Environment lives at the repository root so one .env.local serves every surface.
  envDir: '../../',
  server: { port: 5173 },
  test: {
    environment: 'jsdom',
    env: {
      VITE_APP_ENV: 'test',
      VITE_API_BASE_URL: 'http://localhost:8080',
    },
  },
});
