package config

import (
	"os"
)

type Config struct {
	GmailClientID     string
	GmailClientSecret string
	GmailTokenFile    string
}

func LoadConfig() *Config {
	return &Config{
		GmailClientID:     os.Getenv("GMAIL_CLIENT_ID"),
		GmailClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
		GmailTokenFile:    "token.json",
	}
}
