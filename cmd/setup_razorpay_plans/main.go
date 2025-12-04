// Package main provides a one-time utility to create Razorpay Team plans
// and display their IDs for manual configuration.
//
// Since this is a utility that needs to import internal/license package
// which has files in subdirectories, it's simpler to create this as a
// test file in the license/payment package itself.
//
// Instead, run the setup from the payment package test:
// go test -v ./internal/license/payment -run TestSetupPlans
//
// Or use this script approach:
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("This utility has been moved to internal/license/payment/")
	fmt.Println()
	fmt.Println("To create Razorpay Team plans, run:")
	fmt.Println()
	fmt.Println("  cd /home/shrsv/bin/LiveReview")
	fmt.Println("  go test -v ./internal/license/payment -run TestSetupPlans")
	fmt.Println()
	fmt.Println("Or run the setup function directly from the payment package.")
	os.Exit(1)
}
