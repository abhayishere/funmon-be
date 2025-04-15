#!/bin/bash

# Load environment variables from .env file
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Set default values if not set
export GMAIL_CLIENT_ID=${GMAIL_CLIENT_ID:-"your_client_id_here"}
export GMAIL_CLIENT_SECRET=${GMAIL_CLIENT_SECRET:-"your_client_secret_here"}

# Run the application
go run main.go 