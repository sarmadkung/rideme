# Payments, Wallet & Settlement

## Initial Methods

- Cash
- Raast
- Local payment gateways
- Cards where supported

## Flow

```text
Job -> Quote -> Payment Intent -> Completion -> Final Amount -> Ledger -> Settlement
```

The ledger is append-only. Never mutate historical financial entries.

Example:

```text
Driver earning       +500
Platform fee          -50
Compensation          +100
Settlement            -550
```

COD flow: order -> driver collection -> COD record -> delivery confirmation -> merchant balance -> settlement.

Reconcile internal ledger against providers, bank/Raast records and cash records. Never store raw card data.
