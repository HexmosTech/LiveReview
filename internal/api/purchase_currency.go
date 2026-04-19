package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/livereview/internal/license/payment"
)

var purchaseCurrencies = []string{payment.CurrencyUSD, payment.CurrencyINR}

func supportedPurchaseCurrencies() []string {
	out := make([]string, len(purchaseCurrencies))
	copy(out, purchaseCurrencies)
	return out
}

func resolvePurchaseCurrency(raw string, r *http.Request) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultPurchaseCurrencyForRequest(r), nil
	}
	return payment.NormalizeCurrency(trimmed)
}

func defaultPurchaseCurrencyForRequest(r *http.Request) string {
	if strings.EqualFold(requestCountryCode(r), "IN") {
		return payment.CurrencyINR
	}

	acceptLanguage := strings.ToUpper(strings.TrimSpace(r.Header.Get("Accept-Language")))
	acceptLanguage = strings.ReplaceAll(acceptLanguage, "_", "-")
	if strings.Contains(acceptLanguage, "-IN") {
		return payment.CurrencyINR
	}

	return payment.CurrencyUSD
}

func requestCountryCode(r *http.Request) string {
	if r == nil {
		return ""
	}

	for _, headerName := range []string{"CF-IPCountry", "CloudFront-Viewer-Country", "X-Country-Code", "X-Country"} {
		value := strings.ToUpper(strings.TrimSpace(r.Header.Get(headerName)))
		if value != "" {
			return value
		}
	}

	return ""
}

func currencyErrorMessage(err error) string {
	if err == nil {
		return "invalid currency"
	}
	return fmt.Sprintf("invalid currency: %v", err)
}
