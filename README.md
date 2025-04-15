# Spending Dashboard Backend

A RESTful API backend for a spending dashboard application, built with Go.

## Features

- RESTful API endpoints for transaction data
- CORS support for frontend integration
- Simulated transaction data generation
- Support for different time-based filters

## API Endpoints

### GET /transactions
Returns transaction data based on the specified filter.

Query Parameters:
- `filter`: Time period filter (daily|weekly|monthly|all)

Example Response:
```json
{
  "summary": {
    "total": 1234.56,
    "changePercentage": 15.5
  },
  "details": [
    {
      "date": "2024-03-20",
      "amount": 99.99,
      "description": "Transaction 1-1"
    }
  ]
}
```

### GET /refresh
Triggers a data refresh process and returns the latest daily transactions.

Example Response:
```json
{
  "summary": {
    "total": 1234.56,
    "changePercentage": 15.5
  },
  "details": [
    {
      "date": "2024-03-20",
      "amount": 99.99,
      "description": "Transaction 1-1"
    }
  ]
}
```

## Setup and Running

1. Install Go dependencies:
```bash
go mod tidy
```

2. Run the server:
```bash
go run main.go
```

The server will start on port 8080.

## Future Improvements

- Integration with Gmail API for actual transaction data
- Persistent storage for transaction history
- Authentication and authorization
- Rate limiting
- More sophisticated transaction categorization
- Real-time updates using WebSockets 