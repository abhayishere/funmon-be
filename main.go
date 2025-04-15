package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/abhayyadav/funnyMoney/be/config"
	"github.com/abhayyadav/funnyMoney/be/services"
	"github.com/abhayyadav/funnyMoney/be/types"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
)

type CustomClaims struct {
	UserID       string `json:"user_id"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    int64  `json:"expiresAt"`
	jwt.StandardClaims
}

type Summary struct {
	Total            float64 `json:"total"`
	Previously       float64 `json:"previously"`
	ChangePercentage float64 `json:"changePercentage"`
}

type TransactionsResponse struct {
	Summary Summary             `json:"summary"`
	Details []types.Transaction `json:"details"`
}

var (
	gmailService *services.GmailService
	oauthConfig  *oauth2.Config
	redisClient  *redis.Client
	cfg          *config.Config
	ctx          = context.Background()
)

func getCacheKey(filter string) string {
	return fmt.Sprintf("transactions:%s", filter)
}

func respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := map[string]interface{}{
		"error": message,
		"code":  statusCode,
	}
	json.NewEncoder(w).Encode(resp)
}

type TokenRequest struct {
	AccessToken string `json:"access_token"`
}

func calculateSummary(transactions []types.Transaction, period string) (Summary, error) {
	switch period {
	case "daily":

		var maxDate time.Time
		dateTotals := make(map[string]float64)
		layout := "2006-01-02"
		for _, txn := range transactions {
			t, err := time.Parse(layout, txn.Date)
			if err != nil {
				continue
			}

			dateTotals[txn.Date] += txn.Amount
			if t.After(maxDate) {
				maxDate = t
			}
		}
		if maxDate.IsZero() {
			return Summary{}, fmt.Errorf("no valid transaction dates found")
		}

		currentDay := maxDate.Format(layout)
		previousDay := maxDate.AddDate(0, 0, -1).Format(layout)
		currentTotal := dateTotals[currentDay]
		previousTotal := dateTotals[previousDay]
		var change float64
		if previousTotal != 0 {
			change = ((currentTotal - previousTotal) / previousTotal) * 100
		} else {
			change = 0
		}
		return Summary{
			Total:            currentTotal,
			Previously:       previousTotal,
			ChangePercentage: change,
		}, nil

	case "weekly":

		var maxDate time.Time
		dateTotals := make(map[string]float64)
		layout := "2006-01-02"
		for _, txn := range transactions {
			t, err := time.Parse(layout, txn.Date)
			if err != nil {
				continue
			}
			dateKey := t.Format(layout)
			dateTotals[dateKey] += txn.Amount
			if t.After(maxDate) {
				maxDate = t
			}
		}
		if maxDate.IsZero() {
			return Summary{}, fmt.Errorf("no valid transaction dates found")
		}

		currentWeekTotal := 0.0
		previousWeekTotal := 0.0

		for i := 0; i < 14; i++ {
			day := maxDate.AddDate(0, 0, -i)
			dayStr := day.Format(layout)
			amount := dateTotals[dayStr]
			if i < 7 {
				currentWeekTotal += amount
			} else {
				previousWeekTotal += amount
			}
		}
		var change float64
		if previousWeekTotal != 0 {
			change = ((currentWeekTotal - previousWeekTotal) / previousWeekTotal) * 100
		} else {
			change = 0
		}
		return Summary{
			Total:            currentWeekTotal,
			Previously:       previousWeekTotal,
			ChangePercentage: change,
		}, nil

	case "monthly":

		monthTotals := make(map[string]float64)
		layout := "2006-01-02"
		for _, txn := range transactions {
			t, err := time.Parse(layout, txn.Date)
			if err != nil {
				continue
			}
			monthKey := t.Format("2006-01")
			monthTotals[monthKey] += txn.Amount
		}

		var months []string
		for m := range monthTotals {
			months = append(months, m)
		}
		if len(months) == 0 {
			return Summary{}, fmt.Errorf("no valid transaction months found")
		}
		sort.Strings(months)

		currentMonth := months[len(months)-1]
		var previousMonth string
		if len(months) >= 2 {
			previousMonth = months[len(months)-2]
		} else {

			previousMonth = ""
		}
		currentTotal := monthTotals[currentMonth]
		previousTotal := 0.0
		if previousMonth != "" {
			previousTotal = monthTotals[previousMonth]
		}
		var change float64
		if previousTotal != 0 {
			change = ((currentTotal - previousTotal) / previousTotal) * 100
		} else {
			change = 0
		}
		return Summary{
			Total:            currentTotal,
			Previously:       previousTotal,
			ChangePercentage: change,
		}, nil

	default:

		var total float64
		for _, t := range transactions {
			total += t.Amount
		}
		return Summary{
			Total:            total,
			ChangePercentage: 0,
		}, nil
	}
}

func transactionsHandler(w http.ResponseWriter, r *http.Request) {
	frontendURL := os.Getenv("FRONTEND_URL")

	w.Header().Set("Access-Control-Allow-Origin", frontendURL)
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" {
		respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}
	key := getCacheKey(filter)
	var response TransactionsResponse

	cached, err := redisClient.Get(ctx, key).Result()
	if err == nil {
		err = json.Unmarshal([]byte(cached), &response)
		if err == nil {
			log.Printf("Cache hit for filter: %s", filter)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}
	log.Printf("Cache miss for filter: %s; calling Gmail API", filter)

	accessToken := r.URL.Query().Get("access_token")
	if strings.TrimSpace(accessToken) == "" {
		respondError(w, http.StatusUnauthorized, "Missing access token in query string")
		return
	}

	oauthToken := &oauth2.Token{
		AccessToken: accessToken,
	}

	tokenSource := oauthConfig.TokenSource(ctx, oauthToken)
	client := oauth2.NewClient(ctx, tokenSource)
	gmailService, err = services.NewGmailServiceWithClient(cfg, client)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Gmail service error: %v", err))
		return
	}

	var days int
	switch filter {
	case "daily":
		days = 2
	case "weekly":
		days = 14
	case "monthly":
		days = 60
	case "all":
		days = 90
	default:
		respondError(w, http.StatusBadRequest, "Invalid filter")
		return
	}

	transactions, err := gmailService.FetchTransactions(days)
	if err != nil {
		if appErr, ok := err.(*services.AppError); ok {
			respondError(w, appErr.Code, appErr.Msg)
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	if filter == "daily" {

	}
	summary, err := calculateSummary(transactions, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	response = TransactionsResponse{
		Summary: summary,
		Details: transactions,
	}
	respJSON, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshalling response: %v", err)
	} else {
		err = redisClient.Set(ctx, key, respJSON, 10*time.Minute).Err()
		if err != nil {
			log.Printf("Error setting Redis cache: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	frontendURL := os.Getenv("FRONTEND_URL")
	w.Header().Set("Access-Control-Allow-Origin", frontendURL)
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "GET" && r.Method != "POST" {
		respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	accessToken := r.URL.Query().Get("access_token")
	if strings.TrimSpace(accessToken) == "" {
		respondError(w, http.StatusUnauthorized, "Missing access token in query string")
		return
	}

	oauthToken := &oauth2.Token{
		AccessToken: accessToken,
	}

	tokenSource := oauthConfig.TokenSource(ctx, oauthToken)
	client := oauth2.NewClient(ctx, tokenSource)

	var err error
	gmailService, err = services.NewGmailServiceWithClient(cfg, client)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Gmail service error: %v", err))
		return
	}

	dailyTxns, err := gmailService.FetchTransactions(2)
	if err != nil {
		if appErr, ok := err.(*services.AppError); ok {
			respondError(w, appErr.Code, appErr.Msg)
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	weeklyTxns, err := gmailService.FetchTransactions(14)
	if err != nil {
		if appErr, ok := err.(*services.AppError); ok {
			respondError(w, appErr.Code, appErr.Msg)
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	monthlyTxns, err := gmailService.FetchTransactions(60)
	if err != nil {
		if appErr, ok := err.(*services.AppError); ok {
			respondError(w, appErr.Code, appErr.Msg)
		} else {
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	dailySummary, err := calculateSummary(dailyTxns, "daily")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	weeklySummary, err := calculateSummary(weeklyTxns, "weekly")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	monthlySummary, err := calculateSummary(monthlyTxns, "monthly")
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dailyResponse := TransactionsResponse{
		Summary: dailySummary,
		Details: dailyTxns,
	}
	weeklyResponse := TransactionsResponse{
		Summary: weeklySummary,
		Details: weeklyTxns,
	}
	monthlyResponse := TransactionsResponse{
		Summary: monthlySummary,
		Details: monthlyTxns,
	}

	if data, err := json.Marshal(dailyResponse); err == nil {
		redisClient.Set(ctx, getCacheKey("daily"), data, 20*time.Minute)
	} else {
		log.Printf("Error marshalling daily response: %v", err)
	}
	if data, err := json.Marshal(weeklyResponse); err == nil {
		redisClient.Set(ctx, getCacheKey("weekly"), data, 20*time.Minute)
	} else {
		log.Printf("Error marshalling weekly response: %v", err)
	}
	if data, err := json.Marshal(monthlyResponse); err == nil {
		redisClient.Set(ctx, getCacheKey("monthly"), data, 20*time.Minute)
	} else {
		log.Printf("Error marshalling monthly response: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default port
	}
	cfg = config.LoadConfig()
	redisClient = services.InitRedis()

	oauthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GMAIL_CLIENT_ID"),
		ClientSecret: os.Getenv("GMAIL_CLIENT_SECRET"),
		Scopes:       []string{gmail.GmailReadonlyScope},
	}

	r := mux.NewRouter()
	r.HandleFunc("/transactions", transactionsHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/refresh", refreshHandler).Methods("POST", "OPTIONS")

	fmt.Println("Server starting on port" + port + "...")
	log.Fatal(http.ListenAndServe(":"+port, r))
}
