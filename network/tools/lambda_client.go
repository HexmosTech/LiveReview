package tools

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// LambdaClient interface wraps the Invoke method of the AWS SDK Lambda client.
// This allows mocking the client in tests.
type LambdaClient interface {
	Invoke(ctx context.Context, params *lambda.InvokeInput, optFns ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
}

// InvokeTool calls a Lambda function by its ARN, passing a payload, and returns the response body.
func InvokeTool(ctx context.Context, client LambdaClient, arn string, payload []byte) ([]byte, error) {
	if client == nil {
		return nil, fmt.Errorf("aws lambda client is nil")
	}

	input := &lambda.InvokeInput{
		FunctionName: aws.String(arn),
		Payload:      payload,
	}

	output, err := client.Invoke(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("aws lambda invoke failed: %w", err)
	}

	if output.FunctionError != nil {
		return nil, fmt.Errorf("lambda returned function error: %s", *output.FunctionError)
	}

	return output.Payload, nil
}
