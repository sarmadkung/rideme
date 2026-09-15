package credit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/credit"
	"github.com/sarmadkung/rideme/services/api/internal/finance"
	"github.com/sarmadkung/rideme/services/api/internal/identity"
	"github.com/sarmadkung/rideme/services/api/internal/providers"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// --- fakes -------------------------------------------------------------------

type fakeLedger struct {
	recorded []finance.Settlement
	limits   map[string]finance.CreditLimit
	standing finance.Standing
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{
		limits: map[string]finance.CreditLimit{
			"MOTORCYCLE": {
				VehicleType: "MOTORCYCLE",
				Cap:         money.MustNew(80000, money.PKR),
				Warn:        money.MustNew(50000, money.PKR),
				Version:     1,
			},
		},
		standing: finance.Standing{
			Owed: money.MustNew(90000, money.PKR), Cap: money.MustNew(80000, money.PKR),
			Warn: money.MustNew(50000, money.PKR), Clearing: money.MustNew(40000, money.PKR),
			Blocked: true, Warning: true, Limited: true,
		},
	}
}

func (l *fakeLedger) StandingOf(context.Context, string, string) (finance.Standing, error) {
	return l.standing, nil
}

func (l *fakeLedger) RecordSettlement(_ context.Context, set finance.Settlement, key string) (finance.Settlement, error) {
	for _, existing := range l.recorded {
		if key != "" && existing.Reference == key {
			return existing, nil
		}
	}
	set.ID = "settlement-1"
	set.Reference = key
	l.recorded = append(l.recorded, set)
	return set, nil
}

func (l *fakeLedger) SettlementsOf(context.Context, string, int) ([]finance.Settlement, error) {
	return l.recorded, nil
}

func (l *fakeLedger) SetCreditLimit(_ context.Context, limit finance.CreditLimit) error {
	l.limits[limit.VehicleType] = limit
	return nil
}

func (l *fakeLedger) LimitFor(_ context.Context, vehicleType, _ string) (finance.CreditLimit, error) {
	limit, ok := l.limits[vehicleType]
	if !ok {
		return finance.CreditLimit{}, finance.ErrNoCap
	}
	return limit, nil
}

type fakeDrivers struct{ missing bool }

func (d fakeDrivers) DriverByID(_ context.Context, id string) (providers.Driver, error) {
	if d.missing {
		return providers.Driver{}, providers.ErrDriverNotFound
	}
	return providers.Driver{ID: id, ActiveVehicleID: "vehicle-1"}, nil
}

func (d fakeDrivers) DriverByUserID(_ context.Context, userID string) (providers.Driver, error) {
	return providers.Driver{ID: "driver-1", ActiveVehicleID: "vehicle-1"}, nil
}

func (d fakeDrivers) VehicleByID(_ context.Context, id string) (providers.Vehicle, error) {
	return providers.Vehicle{ID: id, Type: "MOTORCYCLE"}, nil
}

func serve(t *testing.T, ledger *fakeLedger, drivers fakeDrivers, roles ...identity.Role) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	credit.NewHandler(ledger, drivers).Routes(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := identity.Principal{UserID: "agent-1", SessionID: "s1", Roles: roles}
			next.ServeHTTP(w, r.WithContext(identity.ContextWithPrincipal(r.Context(), principal)))
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func post(t *testing.T, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// --- tests -------------------------------------------------------------------

// The gap this package closes: a driver over their cap could not work and
// nothing could record that they had paid, so the only way back was SQL.
func TestAnAgentRecordsCashAndTheDriverIsUnblocked(t *testing.T) {
	ledger := newFakeLedger()
	server := serve(t, ledger, fakeDrivers{}, identity.RoleAdmin)

	// The driver hands in enough to clear the block.
	ledger.standing = finance.Standing{
		Owed: money.MustNew(50000, money.PKR), Cap: money.MustNew(80000, money.PKR),
		Warn: money.MustNew(50000, money.PKR), Clearing: money.MustNew(0, money.PKR),
		Blocked: false, Warning: true, Limited: true,
	}

	resp := post(t, server.URL+"/api/v1/admin/drivers/driver-1/settlements",
		`{"amount_minor":40000,"method":"CASH","note":"handed in at Gulberg office"}`, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201", resp.StatusCode)
	}
	var body credit.RecordSettlementResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Settlement.Amount.Minor != 40000 {
		t.Errorf("recorded %d, want 40000", body.Settlement.Amount.Minor)
	}
	// The agent can tell the driver they are back on the road without a
	// second request.
	if body.Standing.Blocked {
		t.Error("the response still reports the driver as blocked")
	}
	if len(ledger.recorded) != 1 {
		t.Errorf("%d settlements recorded, want 1", len(ledger.recorded))
	}
	if ledger.recorded[0].RecordedBy != "agent-1" {
		t.Errorf("recorded by %q, want agent-1 — who took the cash must be stored",
			ledger.recorded[0].RecordedBy)
	}
}

// The role split is the point of this surface. An agent who can mark a debt
// settled without cash changing hands is a fraud path with a help-desk login.
func TestSupportCanSeeABalanceAndCannotSettleIt(t *testing.T) {
	ledger := newFakeLedger()
	server := serve(t, ledger, fakeDrivers{}, identity.RoleSupport)

	seen, err := http.Get(server.URL + "/api/v1/admin/drivers/driver-1/balance")
	if err != nil {
		t.Fatal(err)
	}
	defer seen.Body.Close()
	if seen.StatusCode != http.StatusOK {
		t.Errorf("support reading a balance got %d, want 200 — it is the call they take", seen.StatusCode)
	}

	settled := post(t, server.URL+"/api/v1/admin/drivers/driver-1/settlements",
		`{"amount_minor":40000}`, nil)
	defer settled.Body.Close()
	if settled.StatusCode == http.StatusCreated {
		t.Error("support recorded a settlement; only admins may say money arrived")
	}

	capped := post(t, server.URL+"/api/v1/admin/credit-limits",
		`{"vehicle_type":"CAR","cap_minor":1,"warn_minor":1}`, nil)
	defer capped.Body.Close()
	if capped.StatusCode == http.StatusNoContent {
		t.Error("support changed a credit cap")
	}
	if len(ledger.recorded) != 0 {
		t.Error("a settlement was recorded by someone who may not record one")
	}
}

// Document 035's header, for the reason it exists: an agent whose tap did not
// appear to work must not credit a driver twice by trying again.
func TestARepeatedIdempotencyKeyRecordsOneSettlement(t *testing.T) {
	ledger := newFakeLedger()
	server := serve(t, ledger, fakeDrivers{}, identity.RoleAdmin)
	headers := map[string]string{"Idempotency-Key": "counter-7"}

	for i := 0; i < 3; i++ {
		resp := post(t, server.URL+"/api/v1/admin/drivers/driver-1/settlements",
			`{"amount_minor":40000}`, headers)
		resp.Body.Close()
	}

	if len(ledger.recorded) != 1 {
		t.Errorf("%d settlements recorded for one key, want 1", len(ledger.recorded))
	}
}

// A typo in a uuid must not create a settlement nobody can find and a ledger
// entry against a subject that does not exist.
func TestASettlementForAnUnknownDriverIsRefused(t *testing.T) {
	ledger := newFakeLedger()
	server := serve(t, ledger, fakeDrivers{missing: true}, identity.RoleAdmin)

	resp := post(t, server.URL+"/api/v1/admin/drivers/nobody/settlements",
		`{"amount_minor":40000}`, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
	if len(ledger.recorded) != 0 {
		t.Error("money was recorded against a driver who does not exist")
	}
}

// A vehicle type with no cap has no limit at all, and that is the fact an
// operator most needs to see. Inferring it from absence is how it gets missed.
func TestUncappedVehicleTypesAreNamed(t *testing.T) {
	server := serve(t, newFakeLedger(), fakeDrivers{}, identity.RoleAdmin)

	resp, err := http.Get(server.URL + "/api/v1/admin/credit-limits")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body struct {
		Limits   []finance.CreditLimit `json:"limits"`
		Uncapped []string              `json:"uncapped"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Limits) != 1 {
		t.Errorf("%d configured limits, want 1", len(body.Limits))
	}
	if len(body.Uncapped) != len(credit.VehicleTypes)-1 {
		t.Errorf("%d uncapped types, want %d", len(body.Uncapped), len(credit.VehicleTypes)-1)
	}
	for _, vehicleType := range body.Uncapped {
		if vehicleType == "MOTORCYCLE" {
			t.Error("a configured type is listed as uncapped")
		}
	}
}

// A warning above the cap would mean a driver is warned only after being
// stopped, which is not a warning.
func TestAWarningAboveTheCapIsRefused(t *testing.T) {
	ledger := newFakeLedger()
	server := serve(t, ledger, fakeDrivers{}, identity.RoleAdmin)

	req, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/admin/credit-limits",
		strings.NewReader(`{"vehicle_type":"CAR","cap_minor":50000,"warn_minor":90000}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		t.Error("a warning threshold above the cap was accepted")
	}
	if _, set := ledger.limits["CAR"]; set {
		t.Error("an invalid limit was stored")
	}
}
