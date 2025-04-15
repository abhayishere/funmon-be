package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abhayyadav/funnyMoney/be/config"
	"github.com/abhayyadav/funnyMoney/be/types"
	"golang.org/x/net/html"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type AppError struct {
	Code int
	Msg  string
}

func (e *AppError) Error() string {
	return e.Msg
}

type GmailService struct {
	service *gmail.Service
	config  *config.Config
}

func NewGmailServiceWithClient(cfg *config.Config, client *http.Client) (*GmailService, error) {
	ctx := context.Background()

	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create Gmail service: %v", err)
	}

	return &GmailService{
		service: srv,
		config:  cfg,
	}, nil
}

func (gs *GmailService) FetchTransactions(days int) ([]types.Transaction, error) {

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	query := fmt.Sprintf("after:%s before:%s subject:(transaction OR payment OR purchase OR UPI txn)",
		startDate.Format("2006/01/02"),
		endDate.Format("2006/01/02"))

	messages, err := gs.service.Users.Messages.List("me").Q(query).Do()
	if err != nil {
		if gErr, ok := err.(*googleapi.Error); ok && (gErr.Code == 403 || gErr.Code == 401) {
			return nil, &AppError{
				Code: http.StatusUnauthorized,
				Msg:  fmt.Sprintf("unauthorized: insufficient authentication scopes: %v", err),
			}
		}

		return nil, &AppError{
			Code: http.StatusInternalServerError,
			Msg:  fmt.Sprintf("unable to retrieve messages: %v", err),
		}
	}
	var transactions []types.Transaction
	for _, msg := range messages.Messages {

		message, err := gs.service.Users.Messages.Get("me", msg.Id).Format("full").Do()
		if err != nil {
			log.Printf("Error getting message %s: %v", msg.Id, err)
			continue
		}

		transaction, err := gs.parseTransactionEmail(message)
		if err != nil {
			log.Printf("Error parsing message %s: %v", msg.Id, err)
			continue
		}

		if transaction != nil {
			transactions = append(transactions, *transaction)
		}
	}

	return transactions, nil
}

func (gs *GmailService) parseTransactionEmail(msg *gmail.Message) (*types.Transaction, error) {

	body := extractMessageContent(msg.Payload)
	if body == "" {
		return nil, fmt.Errorf("no suitable content found in email")
	}

	body = stripHTMLTags(body)

	amountPattern := regexp.MustCompile(`(?i)Rs.\s*\$?([0-9,.]+)`)
	datePattern := regexp.MustCompile(`on\s+(\d{2}-\d{2}-\d{2})`)

	amountMatch := amountPattern.FindStringSubmatch(body)
	dateMatch := datePattern.FindStringSubmatch(body)

	if len(amountMatch) < 2 || len(dateMatch) < 2 {
		return nil, fmt.Errorf("could not parse transaction details")
	}

	amountStr := strings.ReplaceAll(amountMatch[1], ",", "")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse amount: %v", err)
	}

	parsedDate, err := time.Parse("02-01-06", dateMatch[1])
	if err != nil {
		return nil, fmt.Errorf("could not parse date: %v", err)
	}

	return &types.Transaction{
		Date:        parsedDate.Format("2006-01-02"),
		Amount:      amount,
		Description: "Transaction from HTML email",
	}, nil
}
func stripHTMLTags(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {

		return htmlContent
	}

	var sb strings.Builder
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)
	return sb.String()
}

func extractMessageContent(part *gmail.MessagePart) string {

	if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	if part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
		data, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return string(data)
		}
	}

	for _, nestedPart := range part.Parts {
		if content := extractMessageContent(nestedPart); content != "" {
			return content
		}
	}
	return ""
}

func TokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func SaveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
