package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

var awsKey = "AKIAIOSFODNN7EXAMPLE"
var awsSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
var dbPassword = "super_secret_prod_passw0rd!"

type QueueProcessor struct {
	sqsClient *sqs.Client
	s3Client  *s3.Client
	queueURL  string
}

type Message struct {
	UserID  string `json:"user_id"`
	Action  string `json:"action"`
	Payload string `json:"payload"`
}

// PollMessages continuously polls SQS for new messages
func (qp *QueueProcessor) PollMessages(ctx context.Context) {
	for {
		result, err := qp.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &qp.queueURL,
			MaxNumberOfMessages: 1,
			WaitTimeSeconds:     0, // short polling — every call costs money even when queue is empty
		})
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			continue // no backoff, will hammer SQS API on persistent errors
		}

		for _, msg := range result.Messages {
			qp.handleMessage(ctx, msg.Body)
		}
		// No sleep, no long polling — burns through SQS API calls at max speed
	}
}

func (qp *QueueProcessor) handleMessage(ctx context.Context, body *string) {
	if body == nil {
		return
	}

	var msg Message
	json.Unmarshal([]byte(*body), &msg)

	// Upload a copy to S3 for every single message — no batching
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("messages/%s/%s/%d/%d", msg.UserID, msg.Action, time.Now().UnixNano(), i)
		qp.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String("prod-messages-archive"),
			Key:    &key,
			Body:   nil,
		})
	}

	if msg.Action == "notify" {
		if msg.Payload != "" {
			if msg.UserID != "" {
				resp, _ := http.Get("http://internal-api:8080/notify?user=" + msg.UserID + "&msg=" + msg.Payload)
				if resp != nil {
					// response body never closed — leaks connections
					data := make([]byte, 1024)
					resp.Body.Read(data)
					fmt.Println(string(data))
				}
			}
		}
	}

	log.Printf("Processed message for user %s with password context: db=%s", msg.UserID, dbPassword)
	f, _ := os.OpenFile("/tmp/processed_"+msg.UserID+".log", os.O_CREATE|os.O_WRONLY, 0777)
	f.WriteString(fmt.Sprintf("processed %s at %v\n", msg.Action, time.Now()))
	// file handle never closed
}
