# MVP Product Requirements Document

## Objective

Launch a one-city marketplace for:

1. Passenger rides
2. Parcel/document delivery
3. Small cargo/moving

Initial vehicle types:
- Motorcycle
- Rickshaw
- Car
- Loader/Suzuki

## User Types

### Customer
Requests rides, deliveries and cargo.

### Driver
Registers vehicles, selects capabilities, accepts jobs and earns money.

### Merchant
Creates and manages deliveries.

### Operations Admin
Manages supply, jobs, pricing, support, verification and incidents.

## Customer App

### Home
Primary actions:
- Ride
- Send
- Move

### Ride Flow
1. Pickup
2. Destination
3. Vehicle type
4. Fare estimate
5. Request
6. Driver assignment
7. Driver/vehicle verification
8. Live tracking
9. Trip completion
10. Payment
11. Rating

### Send Flow
1. Pickup
2. Destination
3. Item category
4. Weight/size
5. Optional photos
6. Vehicle recommendation
7. Price
8. Booking
9. Tracking
10. Proof of delivery

### Move Flow
1. Pickup
2. Destination
3. Cargo type
4. Approximate weight
5. Dimensions
6. Loading assistance
7. Vehicle recommendation
8. Quote
9. Schedule/instant booking
10. Tracking
11. Delivery proof

## Driver App

### Onboarding
- Phone
- Identity
- License
- Vehicle registration
- Vehicle photos
- Required documents
- Capability selection
- Verification

### Driver Home
- Online/offline
- Today's earnings
- Jobs
- Net earnings estimate
- Performance

### Job Card
- Job type
- Pickup
- Destination/route
- Distance
- Estimated duration
- Gross earning
- Estimated net earning
- Customer/merchant information as appropriate
- Accept/reject

### Driver Features
- Navigation
- Job status
- Customer contact with masked number where appropriate
- Proof of pickup
- Proof of delivery
- Earnings
- Wallet/ledger
- Ratings
- Support
- Document expiry reminders

## Merchant MVP

- Merchant onboarding
- Create delivery
- Customer details
- Pickup/dropoff
- COD amount
- Delivery tracking
- Delivery history
- Basic reporting

## Admin MVP

- User management
- Driver verification
- Vehicle verification
- Job monitoring
- Manual dispatch
- Pricing configuration
- Zones
- Support cases
- Disputes
- Payments
- Driver settlement
- Basic analytics

## Non-Functional Requirements

- Realtime job status
- GPS tracking
- Strong audit trail
- Idempotent job/payment operations
- Secure authentication
- Role-based access
- Observability
- Fraud/risk hooks
- Offline-tolerant driver workflow
