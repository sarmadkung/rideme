package merchant

import (
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/money"
)

func amount(t *testing.T, minor int64) money.Amount {
	t.Helper()
	return money.MustNew(minor, money.PKR)
}

func TestPriceDifferenceReachesTheCustomerInBothDirections(t *testing.T) {
	// BD-11: the customer pays what the substitute costs. A dearer substitute
	// costs them more; a cheaper one costs them less. The platform does not
	// keep the saving.
	dearer, err := PriceDifference(amount(t, 15000), amount(t, 18000), 2)
	if err != nil {
		t.Fatalf("PriceDifference: %v", err)
	}
	if dearer.Minor != 6000 {
		t.Fatalf("two units 30 rupees dearer gave %d minor units, want 6000", dearer.Minor)
	}

	cheaper, err := PriceDifference(amount(t, 15000), amount(t, 12000), 2)
	if err != nil {
		t.Fatalf("PriceDifference: %v", err)
	}
	if cheaper.Minor != -6000 {
		t.Fatalf("two units 30 rupees cheaper gave %d minor units, want -6000", cheaper.Minor)
	}
}

func TestOnlySettledSubstitutionsReprice(t *testing.T) {
	price := amount(t, 18000)
	cases := []struct {
		name  string
		issue Issue
		want  bool
	}{
		{"customer accepted", Issue{Action: ActionSubstitute, SubstitutePrice: &price,
			Resolution: ResolutionCustomerAccepted}, true},
		{"auto applied under ALLOW", Issue{Action: ActionSubstitute, SubstitutePrice: &price,
			Resolution: ResolutionAutoApplied}, true},
		// The customer has not answered. Charging now would bill them for a
		// proposal they may decline.
		{"still pending", Issue{Action: ActionSubstitute, SubstitutePrice: &price,
			Resolution: ResolutionPending}, false},
		{"customer declined", Issue{Action: ActionSubstitute, SubstitutePrice: &price,
			Resolution: ResolutionCustomerDeclined}, false},
		// A removal has no substitute to charge for; the line drops out of the
		// total by its status instead.
		{"removal", Issue{Action: ActionRemove, Resolution: ResolutionAutoApplied}, false},
		{"substitute with no price", Issue{Action: ActionSubstitute,
			Resolution: ResolutionCustomerAccepted}, false},
	}
	for _, c := range cases {
		if got := repricesLine(c.issue); got != c.want {
			t.Fatalf("%s: repricesLine = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCustomerPreferenceStillOverridesTheMerchant(t *testing.T) {
	// BD-11 decided who pays, not who decides. A customer who refused
	// substitutions still gets a removal, whatever the merchant proposes.
	if got := ResolveIssue(PreferDoNotAllow, ActionSubstitute); got != ActionRemove {
		t.Fatalf("DO_NOT_ALLOW produced %s, want %s", got, ActionRemove)
	}
	if got := ResolveIssue(PreferAsk, ActionSubstitute); got != ActionAsk {
		t.Fatalf("ASK_ME produced %s, want %s", got, ActionAsk)
	}
}
