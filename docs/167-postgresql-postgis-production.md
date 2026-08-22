# 167 — PostgreSQL/PostGIS Production

## Objective
Operate PostgreSQL and PostGIS as the primary transactional/geospatial database.

## Data Domains
- users
- drivers
- vehicles
- merchants
- orders
- jobs
- payments metadata
- geographic entities

## PostGIS
Use for:
- service zones
- geofences
- geographic queries
- location proximity

## Production
Plan for:
- automated backups
- point-in-time recovery
- replication/high availability as justified
- connection pooling
- migrations
- monitoring

## Performance
Use:
- appropriate indexes
- query analysis
- partitioning only when justified
- connection limits

## Definition of Done
Database reliability and recovery targets are documented and tested.
