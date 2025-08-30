# LiveReview API

This directory contains the implementation of the LiveReview API server using the Echo framework.

## Endpoints

### Health Check
- `GET /health` - Returns the health status of the API

### API v1

#### Reviews
- `GET /api/v1/reviews` - List all reviews
- `POST /api/v1/reviews` - Create a new review
- `GET /api/v1/reviews/:id` - Get a specific review by ID

## Development

To add new endpoints, modify the `setupRoutes` function in `server.go` and implement the corresponding handler methods.
