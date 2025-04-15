package config

import (
	"fmt"
	"os"
)

type Config struct {
	GmailClientID     string
	GmailClientSecret string
	GmailTokenFile    string
}

func LoadConfig() *Config {
	fmt.Println("client id= ", os.Getenv("GMAIL_CLIENT_ID"), "client secret= ", os.Getenv("GMAIL_CLIENT_SECRET"))
	return &Config{
		GmailClientID:     os.Getenv("GMAIL_CLIENT_ID"),
		GmailClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
		GmailTokenFile:    "token.json",
	}
}
