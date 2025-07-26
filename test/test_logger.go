package main

import (
	"fmt"
	"time"

	"github.com/livereview/internal/logging"
)

func main() {
	fmt.Println("Testing review logger...")

	reviewID := fmt.Sprintf("test-%d", time.Now().Unix())
	logger, err := logging.StartReviewLogging(reviewID)
	if err != nil {
		fmt.Printf("Error starting logger: %v\n", err)
		return
	}
	defer logger.Close()

	logger.Log("This is a test message")
	logger.LogSection("TEST SECTION")
	logger.Log("Another test message")

	fmt.Printf("Logger test completed for review ID: %s\n", reviewID)
}
