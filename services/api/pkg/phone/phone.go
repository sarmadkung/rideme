// Package phone normalises phone numbers to E.164.
//
// Document 28: "Normalize phone numbers before lookup." Without it the same
// person reaches the platform as 03001234567, +923001234567 and 0092 300
// 1234567, and creates three accounts — which is also one of the fraud signals
// document 20 lists.
package phone

import (
	"errors"
	"fmt"
	"strings"
)

// Region is a default dialling context for numbers written in local form.
type Region struct {
	// Code is the ISO country code, for messages.
	Code string
	// CallingCode is the international prefix, without '+'.
	CallingCode string
	// TrunkPrefix is the digit local numbers carry that E.164 drops — '0' in
	// Pakistan, as in 0300 1234567 for +92 300 1234567.
	TrunkPrefix string
	// NationalDigits is how many digits a valid national number has after the
	// trunk prefix is removed.
	NationalDigits int
}

// Pakistan is the platform's launch market (documents 02, 06, 19 price in PKR
// and settle over Raast). It is the default region, not a hardcoded
// assumption: Normalize takes the region as an argument.
var Pakistan = Region{Code: "PK", CallingCode: "92", TrunkPrefix: "0", NationalDigits: 10}

var (
	ErrEmpty     = errors.New("phone: number is required")
	ErrNotDigits = errors.New("phone: number contains unexpected characters")
	ErrLength    = errors.New("phone: number has the wrong number of digits")
)

// Normalize converts a number written any of the usual ways into E.164.
//
//	0300 1234567    -> +923001234567
//	+92 300 1234567 -> +923001234567
//	0092-300-1234567 -> +923001234567
//
// It is deliberately strict about the result: a number that cannot be
// normalised confidently is rejected rather than stored in a shape that will
// not match at the next lookup.
func Normalize(input string, region Region) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ErrEmpty
	}

	// Strip the separators people actually type. Anything else is a mistake
	// worth reporting rather than silently discarding.
	var digits strings.Builder
	hasPlus := strings.HasPrefix(trimmed, "+")
	for i, r := range trimmed {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' && i == 0:
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", fmt.Errorf("%w: %q", ErrNotDigits, input)
		}
	}
	national := digits.String()
	if national == "" {
		return "", ErrEmpty
	}

	switch {
	case hasPlus:
		// Already international; the calling code is whatever was given.
		national = strings.TrimPrefix(national, region.CallingCode)
	case strings.HasPrefix(national, "00"+region.CallingCode):
		national = strings.TrimPrefix(national, "00"+region.CallingCode)
	case strings.HasPrefix(national, region.CallingCode) &&
		len(national) == len(region.CallingCode)+region.NationalDigits:
		national = strings.TrimPrefix(national, region.CallingCode)
	}

	national = strings.TrimPrefix(national, region.TrunkPrefix)

	if len(national) != region.NationalDigits {
		return "", fmt.Errorf("%w: %q has %d national digits, want %d",
			ErrLength, input, len(national), region.NationalDigits)
	}
	return "+" + region.CallingCode + national, nil
}

// Mask renders a number for logs and for display where the full number would
// be over-disclosure — an OTP screen confirming which number was used.
//
//	+923001234567 -> +92300****567
func Mask(e164 string) string {
	if len(e164) < 8 {
		return "****"
	}
	keepHead := len(e164) - 7
	return e164[:keepHead] + "****" + e164[len(e164)-3:]
}
