package phone_test

import (
	"errors"
	"testing"

	"github.com/sarmadkung/rideme/services/api/pkg/phone"
)

func TestNormalizeAcceptsTheFormsPeopleActuallyType(t *testing.T) {
	// Every one of these is the same person. Document 28 requires they resolve
	// to one account, not several.
	const want = "+923001234567"
	for _, input := range []string{
		"03001234567",
		"0300 1234567",
		"0300-123-4567",
		"+923001234567",
		"+92 300 1234567",
		"+92 (300) 123 4567",
		"00923001234567",
		"0092-300-1234567",
		"923001234567",
		"  03001234567  ",
	} {
		got, err := phone.Normalize(input, phone.Pakistan)
		if err != nil {
			t.Errorf("%q: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("%q normalised to %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeRejectsWhatItCannotBeSureOf(t *testing.T) {
	cases := map[string]error{
		"":             phone.ErrEmpty,
		"   ":          phone.ErrEmpty,
		"0300123456":   phone.ErrLength, // one digit short
		"030012345678": phone.ErrLength, // one digit long
		"0300abc4567":  phone.ErrNotDigits,
		"0300/1234567": phone.ErrNotDigits,
	}
	for input, want := range cases {
		if _, err := phone.Normalize(input, phone.Pakistan); !errors.Is(err, want) {
			t.Errorf("%q: want %v, got %v", input, want, err)
		}
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	// Storing a normalised number and normalising it again must not change it,
	// or lookups drift over time.
	once, err := phone.Normalize("03001234567", phone.Pakistan)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := phone.Normalize(once, phone.Pakistan)
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Fatalf("not idempotent: %q then %q", once, twice)
	}
}

func TestMaskHidesTheSubscriberDigits(t *testing.T) {
	if got := phone.Mask("+923001234567"); got != "+92300****567" {
		t.Fatalf("got %q", got)
	}
	if got := phone.Mask("short"); got != "****" {
		t.Fatalf("got %q", got)
	}
}
