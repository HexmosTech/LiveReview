package license

import (
	"fmt"
	"testing"
)

func TestGetRazoarPayKeys(t *testing.T) {
	accessKey, secretKey, err := GetRazorpayKeys("test")
	if err != nil {
		t.Errorf("Error getting Razorpay keys: %v", err)
	}

	fmt.Printf("%s %s\n", accessKey, secretKey)

	if accessKey == "" || secretKey == "" {
		t.Error("Razorpay keys should not be empty")
	}
}
