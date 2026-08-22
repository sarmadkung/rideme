# 147 — Admin React Dashboard Architecture

## Technology
```text
ReactJS
TypeScript
React Router
TanStack Query
Zustand only for client/UI state where justified
Tailwind/CSS design system
WebSocket client
Playwright
```

## Application Areas
```text
Operations
Drivers
Vehicles
Merchants
Orders
Dispatch
Pricing
Zones
Reviews
Incidents
Support
Reports
Configuration
Audit
```

## State
TanStack Query:
- server state
- lists
- details
- mutations
- cache

Local state:
- filters
- dialogs
- temporary UI state

## Permissions
Route and action visibility may use permissions, but backend authorization is authoritative.

## Definition of Done
The admin platform is a React SPA independent of the customer Next.js/web application.
