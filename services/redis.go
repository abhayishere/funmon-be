package services

import (
	"context"
	"log"
	"os"

	"github.com/go-redis/redis/v8"
)

var ctx = context.Background()

func InitRedis() *redis.Client {
	redisURL := os.Getenv("REDIS_ADDRESS")
	if redisURL == "" {
		log.Fatalf("REDIS_ADDRESS environment variable is not set")
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}
	log.Printf("Connecting to Redis at %s", opt.Addr)
	client := redis.NewClient(opt)

	// Check connectivity
	_, err = client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully!")
	return client
}
