// Command contractgen emits the TypeScript types and Zod schemas for the wire
// contract from the Go types that define it (ADR-007).
//
//	make contracts        regenerate
//	make contracts-check  fail if the checked-in output is stale
//
// The registry below is the whole contract. A type that is not registered here
// is not part of it, and a client that needs one must add it here rather than
// hand-writing a matching interface — that is the duplication B-2 removed.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/sarmadkung/rideme/services/api/internal/booking"
	"github.com/sarmadkung/rideme/services/api/internal/driver"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/merchant"
	"github.com/sarmadkung/rideme/services/api/internal/pricing"
	"github.com/sarmadkung/rideme/services/api/pkg/contract"
	"github.com/sarmadkung/rideme/services/api/pkg/events"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
	"github.com/sarmadkung/rideme/services/api/pkg/routing"
)

// Registry builds the contract. It is exported through this package's test so
// the registration list itself can be asserted against the Go constants.
func Registry() *contract.Registry {
	r := contract.New()

	// The error taxonomy. These values are the reason B-2 existed.
	r.Enum("ErrorCode", "ERROR_CODES", reflect.TypeOf(httpx.Code("")),
		string(httpx.CodeNotFound),
		string(httpx.CodeUnauthorized),
		string(httpx.CodeForbidden),
		string(httpx.CodeConflict),
		string(httpx.CodeValidation),
		string(httpx.CodeRateLimited),
		string(httpx.CodeUnavailable),
		string(httpx.CodeInternal),
	)
	r.Enum("HealthStatus", "HEALTH_STATUSES", reflect.TypeOf(health.Status("")),
		string(health.StatusHealthy),
		string(health.StatusDegraded),
		string(health.StatusUnhealthy),
	)
	r.Enum("Currency", "CURRENCIES", reflect.TypeOf(money.Currency("")),
		string(money.PKR),
	)
	// The merchant's vocabulary (documents 70, 72, 74). These are values a
	// dashboard sends — a queue name in a query, an action in an issue body —
	// and branches on. Hand-writing them beside the generated types is the
	// duplication B-2 removed, and a typo in one would 400 at runtime.
	r.Enum("MerchantQueue", "MERCHANT_QUEUES", reflect.TypeOf(merchant.Queue("")),
		string(merchant.QueueNew),
		string(merchant.QueuePreparing),
		string(merchant.QueueReady),
		string(merchant.QueueCompleted),
		string(merchant.QueueCancelled),
	)
	r.Enum("IssueAction", "ISSUE_ACTIONS", reflect.TypeOf(merchant.IssueAction("")),
		string(merchant.ActionSubstitute),
		string(merchant.ActionRemove),
		string(merchant.ActionAsk),
	)
	r.Enum("GroceryOrderStatus", "GROCERY_ORDER_STATUSES", reflect.TypeOf(merchant.OrderStatus("")),
		string(merchant.StatusCart),
		string(merchant.StatusPlaced),
		string(merchant.StatusPaymentPending),
		string(merchant.StatusConfirmed),
		string(merchant.StatusPreparing),
		string(merchant.StatusReadyForPickup),
		string(merchant.StatusPickedUp),
		string(merchant.StatusDelivering),
		string(merchant.StatusDelivered),
		string(merchant.StatusCancelled),
		string(merchant.StatusFailed),
	)
	// Event names are open-ended and validated by shape, not enumerated
	// (document 150 gives examples, not a closed list).
	r.Pattern("EventName", reflect.TypeOf(events.Name("")), events.NamePattern)

	r.Struct("ApiErrorBody", httpx.ErrorBody{})
	r.Struct("PageInfo", httpx.PageInfo{})
	r.Struct("Money", money.Amount{})
	// The bound is not decoration. Past MAX_SAFE_INTEGER a JavaScript client
	// silently loses precision on a value the server still holds exactly, so
	// the client rejects what the server would have rejected (BD-07).
	r.Field("Money", "amount_minor", "number",
		fmt.Sprintf("z.number().int().min(-%d).max(%d)", money.MaxSafeMinor, money.MaxSafeMinor))
	r.Struct("DependencyHealth", health.DependencyResult{})
	r.Struct("HealthResponse", health.Report{})
	r.Struct("AnalyticsEvent", events.Envelope{})

	// The booking surface (documents 14, 35). Registered so the mobile and web
	// clients receive generated models rather than hand-written ones.
	r.Struct("JobStop", booking.StopResponse{})
	r.Struct("Job", booking.JobResponse{})
	r.Struct("QuoteLine", pricing.Line{})
	r.Struct("Quote", booking.QuoteResponse{})
	r.Struct("CancelResult", booking.CancelResponse{})

	// Driver surface.
	r.Struct("DriverProfile", driver.DriverResponse{})
	r.Struct("RejectedFix", driver.RejectedFix{})
	r.Struct("LocationReport", driver.LocationResponse{})
	r.Struct("DriverAssignment", driver.AssignmentResponse{})

	// Place search (documents 93, 94). Registered late — the slice that built
	// it hand-wrote a matching TypeScript interface instead, which is the
	// duplication this file's own comment forbids.
	r.Struct("Point", routing.Point{})
	r.Struct("Place", routing.Place{})

	// Driver earnings. Nested inner-first, so the outer struct's fields
	// resolve to the names registered here rather than to inline shapes.
	r.Struct("EarningsTotal", finance.Earnings{})
	r.Struct("TripEarning", finance.TripEarning{})
	r.Struct("DriverEarnings", driver.Earnings{})

	// The merchant's order surface (documents 72, 74). The dashboard reads the
	// same generated models the mobile apps do. Inner-first, so the order's
	// items and issues resolve to the names registered here rather than to
	// inline shapes.
	r.Struct("MerchantOrderItem", merchant.OrderItemResponse{})
	r.Struct("MerchantOrderIssue", merchant.IssueResponse{})
	r.Struct("MerchantOrder", merchant.OrderResponse{})

	return r
}

func main() {
	root := flag.String("root", "", "repository root")
	flag.Parse()
	if *root == "" {
		fmt.Fprintln(os.Stderr, "contractgen: -root is required")
		os.Exit(2)
	}

	registry := Registry()

	types, err := registry.EmitTypeScript()
	if err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
	schemas, err := registry.EmitZod()
	if err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}

	outputs := map[string]string{
		filepath.Join(*root, "packages", "types", "src", "generated.ts"):      types,
		filepath.Join(*root, "packages", "validation", "src", "generated.ts"): schemas,
	}
	for path, body := range outputs {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "contractgen:", err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
