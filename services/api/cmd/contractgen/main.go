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

	"github.com/sarmadkung/rideme/services/api/pkg/contract"
	"github.com/sarmadkung/rideme/services/api/pkg/events"
	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
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
