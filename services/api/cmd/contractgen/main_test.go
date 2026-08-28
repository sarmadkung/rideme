package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/health"
	"github.com/sarmadkung/rideme/services/api/pkg/httpx"
)

// The generator removes hand-maintained duplication, but the registration list
// itself is still written by hand. These tests guard that last hand-written
// step: if a Go constant is added and not registered, they fail here rather
// than silently shipping a client that cannot name it.

func TestEveryErrorCodeIsRegistered(t *testing.T) {
	// Every code httpx maps to a status must reach clients. StatusFor's
	// default arm means an unregistered code would map silently to 500.
	all := []httpx.Code{
		httpx.CodeNotFound, httpx.CodeUnauthorized, httpx.CodeForbidden,
		httpx.CodeConflict, httpx.CodeValidation, httpx.CodeUnavailable,
		httpx.CodeInternal,
	}
	emitted, err := Registry().EmitTypeScript()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range all {
		if !strings.Contains(emitted, "'"+string(code)+"'") {
			t.Errorf("error code %q is not in the generated contract", code)
		}
	}
}

func TestEveryHealthStatusIsRegistered(t *testing.T) {
	emitted, err := Registry().EmitTypeScript()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []health.Status{health.StatusHealthy, health.StatusDegraded, health.StatusUnhealthy} {
		if !strings.Contains(emitted, "'"+string(status)+"'") {
			t.Errorf("health status %q is not in the generated contract", status)
		}
	}
}

func TestGeneratedTypeScriptCoversEveryServedShape(t *testing.T) {
	want := []string{
		"ApiErrorBody", "PageInfo", "Money", "DependencyHealth",
		"HealthResponse", "AnalyticsEvent", "ErrorCode", "HealthStatus",
		"Currency", "EventName",
	}
	got := Registry().Names()
	for _, name := range want {
		found := false
		for _, have := range got {
			if have == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s is not registered; registry has %v", name, got)
		}
	}
}

func TestOptionalGoFieldsBecomeOptionalTypeScriptFields(t *testing.T) {
	emitted, err := Registry().EmitTypeScript()
	if err != nil {
		t.Fatal(err)
	}
	// `details` is omitempty in Go, so it must be optional in TypeScript —
	// and explicitly `| undefined`, which exactOptionalPropertyTypes requires.
	if !strings.Contains(emitted, "details?: Record<string, string> | undefined;") {
		t.Error("omitempty did not produce an optional field")
	}
	// `message` is not omitempty, so it must be required.
	if !strings.Contains(emitted, "message: string;") {
		t.Error("a required field was emitted as optional")
	}
}

func TestGenerationFailsOnAnUnmappedType(t *testing.T) {
	// The generator must refuse a shape it cannot express rather than emit
	// something approximate. This is what keeps the contract honest as new
	// types are registered.
	type unmapped struct {
		Ch chan int `json:"ch"`
	}
	r := Registry()
	r.Struct("Unmapped", unmapped{})

	if _, err := r.EmitTypeScript(); err == nil {
		t.Fatal("expected generation to fail on an unmappable type")
	}
	if _, err := r.EmitZod(); err == nil {
		t.Fatal("expected Zod generation to fail on an unmappable type")
	}
}

func TestBothEmittersDescribeTheSameFields(t *testing.T) {
	// Types and schemas are generated from one registry, so they cannot
	// disagree about which fields exist. This asserts the property directly.
	r := Registry()
	types, err := r.EmitTypeScript()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := r.EmitZod()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"amount_minor", "currency", "request_id", "next_cursor", "correlation_id", "checked_at"} {
		if !strings.Contains(types, field) {
			t.Errorf("%s missing from generated types", field)
		}
		if !strings.Contains(schemas, field) {
			t.Errorf("%s missing from generated schemas", field)
		}
	}
}

func TestTimestampsAreEmittedAsStrings(t *testing.T) {
	// time.Time crosses the wire as RFC 3339 (documents 13, 26, 150), never as
	// a number whose units a client would have to guess.
	types, err := Registry().EmitTypeScript()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(types, "timestamp: string;") || !strings.Contains(types, "checked_at: string;") {
		t.Error("a timestamp was not emitted as a string")
	}
	if reflect.TypeOf(health.Report{}).NumField() == 0 {
		t.Fatal("health.Report has no fields; the registry would be vacuous")
	}
}
