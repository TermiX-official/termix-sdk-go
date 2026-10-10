package termix

import (
	"fmt"
	"math/big"
	"strings"
)

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ToBaseUnits converts a decimal string to integer base units (never float).
func ToBaseUnits(amount string, decimals int) (*big.Int, error) {
	neg := strings.HasPrefix(amount, "-")
	s := strings.TrimPrefix(amount, "-")
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) || (frac != "" && !isDigits(frac)) {
		return nil, fmt.Errorf("invalid decimal amount: %s", amount)
	}
	if len(frac) > decimals {
		return nil, fmt.Errorf("amount %s has more precision than %d decimals", amount, decimals)
	}
	frac += strings.Repeat("0", decimals-len(frac))
	v, ok := new(big.Int).SetString(intPart+frac, 10)
	if !ok {
		return nil, fmt.Errorf("cannot parse amount: %s", amount)
	}
	if neg {
		v.Neg(v)
	}
	return v, nil
}

// FromBaseUnits converts integer base units to a canonical decimal string.
func FromBaseUnits(v *big.Int, decimals int) string {
	neg := v.Sign() < 0
	abs := new(big.Int).Abs(v).String()
	sign := ""
	if neg {
		sign = "-"
	}
	if decimals == 0 {
		return sign + abs
	}
	for len(abs) <= decimals {
		abs = "0" + abs
	}
	intPart := abs[:len(abs)-decimals]
	frac := strings.TrimRight(abs[len(abs)-decimals:], "0")
	if frac == "" {
		return sign + intPart
	}
	return sign + intPart + "." + frac
}
