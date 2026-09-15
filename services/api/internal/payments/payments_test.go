package payments_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sarmadkung/rideme/services/api/internal/payments"
	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

// A provider that exists, for the cases where one must.
type fakeProvider struct {
	name    string
	methods []payments.Method
	verify  error
}

func (p fakeProvider) Name() string               { return p.name }
func (p fakeProvider) Methods() []payments.Method { return p.methods }

func (p fakeProvider) Authorize(context.Context, payments.AuthorizeRequest) (payments.Authorization, error) {
	return payments.Authorization{ProviderReference: "ref-1"}, nil
}

func (p fakeProvider) Capture(context.Context, string, money.Amount) error { return nil }
func (p fakeProvider) VerifyWebhook([]byte, string) error                 { return p.verify }

func configured() []payments.Option {
	return []payments.Option{
		{Method: payments.MethodCash, Label: "Cash", Provider: "cash", Order: 1},
		{Method: payments.MethodCard, Label: "Card", Provider: "somebank", Order: 2},
		{Method: payments.MethodWallet, Label: "Mobile wallet", Provider: "somewallet", Order: 3},
	}
}

// The rule the whole package exists to enforce. A customer shown a card button
// that cannot charge a card finds out at the worst possible moment: standing
// next to a driver at the end of a trip.
func TestAMethodWithNoProviderIsNotOffered(t *testing.T) {
	gateway := payments.NewGateway() // the platform's actual state today

	offered, misconfigured := gateway.Available(configured())

	if len(offered) != 1 || offered[0].Method != payments.MethodCash {
		t.Fatalf("offered %+v, want cash only", offered)
	}
	// Enabled in the database with nothing behind them. The customer correctly
	// does not see them, and somebody needs to be told why.
	if len(misconfigured) != 2 {
		t.Errorf("%d misconfigured methods reported, want 2", len(misconfigured))
	}
}

// Cash needs no provider, cannot fail asynchronously, and is settled by the
// time anyone asks. It is the exception that shapes the rest of the package.
func TestCashIsAlwaysOffered(t *testing.T) {
	offered, _ := payments.NewGateway().Available(
		[]payments.Option{{Method: payments.MethodCash, Label: "Cash", Order: 1}})
	if len(offered) != 1 {
		t.Fatal("cash was filtered out")
	}
	if payments.MethodCash.Digital() {
		t.Error("cash is treated as needing a provider")
	}
	for _, method := range []payments.Method{payments.MethodCard, payments.MethodWallet, payments.MethodBank} {
		if !method.Digital() {
			t.Errorf("%s is treated as needing no provider", method)
		}
	}
}

// A registered provider lights its methods up without any other change.
func TestARegisteredProviderMakesItsMethodsAvailable(t *testing.T) {
	gateway := payments.NewGateway(fakeProvider{
		name: "somewallet", methods: []payments.Method{payments.MethodWallet},
	})

	offered, misconfigured := gateway.Available(configured())

	var sawWallet, sawCard bool
	for _, option := range offered {
		sawWallet = sawWallet || option.Method == payments.MethodWallet
		sawCard = sawCard || option.Method == payments.MethodCard
	}
	if !sawWallet {
		t.Error("a registered wallet provider did not make wallet available")
	}
	if sawCard {
		t.Error("card was offered with no provider registered for it")
	}
	if len(misconfigured) != 1 || misconfigured[0] != payments.MethodCard {
		t.Errorf("misconfigured %+v, want [CARD]", misconfigured)
	}
}

// The available list is a hint to the client; Check is the rule. A client that
// renders correctly is not a guarantee.
func TestCheckRefusesWhatIsNotOffered(t *testing.T) {
	gateway := payments.NewGateway()
	options := configured()

	if err := gateway.Check(payments.MethodCash, options); err != nil {
		t.Errorf("cash refused: %v", err)
	}
	if err := gateway.Check(payments.MethodCard, options); !errors.Is(err, payments.ErrUnavailable) {
		t.Errorf("card error %v, want ErrUnavailable", err)
	}
	if err := gateway.Check("BITCOIN", options); !errors.Is(err, payments.ErrUnknownMethod) {
		t.Errorf("unknown method error %v, want ErrUnknownMethod", err)
	}
}

// A method the business has not switched on is refused even when a provider
// for it is compiled in. The database is what says what the business wants.
func TestAProviderDoesNotEnableAMethodOnItsOwn(t *testing.T) {
	gateway := payments.NewGateway(fakeProvider{
		name: "somebank", methods: []payments.Method{payments.MethodCard},
	})
	onlyCash := []payments.Option{{Method: payments.MethodCash, Label: "Cash", Order: 1}}

	if err := gateway.Check(payments.MethodCard, onlyCash); !errors.Is(err, payments.ErrUnavailable) {
		t.Errorf("card was accepted though the business has it switched off: %v", err)
	}
}

// The order the customer sees is the order the business configured, not
// whatever order the rows came back in.
func TestOptionsKeepTheirConfiguredOrder(t *testing.T) {
	gateway := payments.NewGateway(fakeProvider{
		name: "p", methods: []payments.Method{payments.MethodCard, payments.MethodWallet},
	})
	scrambled := []payments.Option{
		{Method: payments.MethodWallet, Order: 3},
		{Method: payments.MethodCash, Order: 1},
		{Method: payments.MethodCard, Order: 2},
	}

	offered, _ := gateway.Available(scrambled)
	want := []payments.Method{payments.MethodCash, payments.MethodCard, payments.MethodWallet}
	for i, method := range want {
		if offered[i].Method != method {
			t.Errorf("position %d is %s, want %s", i, offered[i].Method, method)
		}
	}
}
