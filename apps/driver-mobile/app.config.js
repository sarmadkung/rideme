// Expo dynamic configuration.
//
// app.json holds everything static. This file exists for one reason: the Google
// Maps key must reach the native build without being committed. This repository
// is public and app.json is tracked, so the key is read from the gitignored
// root .env.local at build time and injected here instead.
//
// The value still ends up inside the shipped binary — that is unavoidable for a
// map SDK key, and is why MAPS_MOBILE_API_KEY must be restricted by Android
// package name + SHA-1 and iOS bundle id rather than kept secret.

const fs = require('node:fs');
const path = require('node:path');

/**
 * Reads one variable from the repository-root .env.local.
 *
 * Deliberately not dotenv: the workspace does not depend on it, and a build
 * config that needs a single value should not add a dependency to get it.
 * Expo loads .env files relative to the app directory, not the repository
 * root, so the root file is read explicitly.
 */
function fromRootEnvLocal(key) {
  if (process.env[key]) return process.env[key];

  const envPath = path.resolve(__dirname, '../../.env.local');
  let contents;
  try {
    contents = fs.readFileSync(envPath, 'utf8');
  } catch {
    return undefined;
  }

  for (const line of contents.split('\n')) {
    const trimmed = line.trim();
    if (trimmed === '' || trimmed.startsWith('#')) continue;
    const separator = trimmed.indexOf('=');
    if (separator === -1) continue;
    if (trimmed.slice(0, separator).trim() !== key) continue;
    return trimmed
      .slice(separator + 1)
      .trim()
      .replace(/^["']|["']$/g, '');
  }
  return undefined;
}

module.exports = ({ config }) => {
  const mapsKey = fromRootEnvLocal('MAPS_MOBILE_API_KEY');

  // Absent is a legitimate state — the map is not built yet, and a placeholder
  // string would make an unconfigured build look configured.
  if (!mapsKey) return config;

  return {
    ...config,
    ios: { ...config.ios, config: { ...config.ios?.config, googleMapsApiKey: mapsKey } },
    android: {
      ...config.android,
      config: { ...config.android?.config, googleMaps: { apiKey: mapsKey } },
    },
  };
};
