-- The platform is location-native: every job has stops, every driver has a
-- position, and dispatch is a geospatial query (documents 04, 13). PostGIS is a
-- prerequisite of the schema, not an optimisation, so it is migration one.
CREATE EXTENSION IF NOT EXISTS postgis;

-- Deterministic, collision-resistant identifiers for the domain tables that
-- arrive in Phase 5.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
