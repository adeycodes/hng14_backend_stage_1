// Package shared contains all common code: DB connection, models,
// helpers, and CORS middleware. Both Vercel handler files import this.
package shared

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// ─────────────────────────────────────────────────────────────────────────────
// DATABASE  (lazy singleton — safe for serverless cold starts)
// ─────────────────────────────────────────────────────────────────────────────

var (
	db     *sql.DB
	dbOnce sync.Once
)

// DB returns the shared *sql.DB, initialising it exactly once.
// Vercel may reuse the same process across requests, so we cache the
// connection rather than opening a new one every invocation.
func DB() *sql.DB {
	dbOnce.Do(func() {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			dsn = "host=localhost user=postgres password=postgres dbname=hng_stage1 sslmode=disable"
		}

		var err error
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			panic(fmt.Sprintf("DB open failed: %v", err))
		}

		// Serverless-friendly pool settings
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)

		if err = db.Ping(); err != nil {
			panic(fmt.Sprintf("DB ping failed: %v", err))
		}

		// Create table if it doesn't exist yet
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS profiles (
				id                  TEXT PRIMARY KEY,
				name                TEXT UNIQUE NOT NULL,
				gender              TEXT,
				gender_probability  DOUBLE PRECISION,
				sample_size         INTEGER,
				age                 INTEGER,
				age_group           TEXT,
				country_id          TEXT,
				country_probability DOUBLE PRECISION,
				created_at          TEXT NOT NULL
			)
		`)
		if err != nil {
			panic(fmt.Sprintf("Create table failed: %v", err))
		}
	})
	return db
}

// ─────────────────────────────────────────────────────────────────────────────
// MODELS
// ─────────────────────────────────────────────────────────────────────────────

// Profile is the full record — returned by POST and GET /api/profiles/{id}
type Profile struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Gender             string  `json:"gender"`
	GenderProbability  float64 `json:"gender_probability"`
	SampleSize         int     `json:"sample_size"`
	Age                int     `json:"age"`
	AgeGroup           string  `json:"age_group"`
	CountryID          string  `json:"country_id"`
	CountryProbability float64 `json:"country_probability"`
	CreatedAt          string  `json:"created_at"`
}

// ProfileSummary is the slimmer shape returned in the list endpoint
type ProfileSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Gender    string `json:"gender"`
	Age       int    `json:"age"`
	AgeGroup  string `json:"age_group"`
	CountryID string `json:"country_id"`
}

// ─────────────────────────────────────────────────────────────────────────────
// EXTERNAL API RESPONSE TYPES
// ─────────────────────────────────────────────────────────────────────────────

type GenderizeResp struct {
	Gender      *string `json:"gender"`
	Probability float64 `json:"probability"`
	Count       int     `json:"count"`
}

type AgifyResp struct {
	Age *int `json:"age"`
}

type NationalizeCountry struct {
	CountryID   string  `json:"country_id"`
	Probability float64 `json:"probability"`
}

type NationalizeResp struct {
	Country []NationalizeCountry `json:"country"`
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

// WriteJSON sets Content-Type, writes the status code and encodes v as JSON
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ErrJSON writes a standard { status, message } error envelope
func ErrJSON(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"status": "error", "message": message})
}

// ErrUpstream writes the specific 502 shape HNG requires for external API failures
func ErrUpstream(w http.ResponseWriter, apiName string) {
	WriteJSON(w, http.StatusBadGateway, map[string]string{
		"status":  "502",
		"message": apiName + " returned an invalid response",
	})
}

// ClassifyAge maps an age to its age_group string
func ClassifyAge(age int) string {
	switch {
	case age <= 12:
		return "child"
	case age <= 19:
		return "teenager"
	case age <= 59:
		return "adult"
	default:
		return "senior"
	}
}

// IsValidName returns true if s contains at least one alphabetic character
func IsValidName(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// FetchJSON performs a GET and JSON-decodes the response body into v
func FetchJSON(url string, v any) error {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}
	return json.Unmarshal(body, v)
}

// NewUUID generates a UUID v7 string (time-sortable)
func NewUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// NowUTC returns current UTC time in ISO 8601 format
func NowUTC() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// ─────────────────────────────────────────────────────────────────────────────
// CORS MIDDLEWARE
// ─────────────────────────────────────────────────────────────────────────────

// WithCORS adds the required CORS headers and handles OPTIONS preflight.
func WithCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CONCURRENT API ENRICHMENT
// ─────────────────────────────────────────────────────────────────────────────

type EnrichResult struct {
	Genderize *GenderizeResp
	Agify     *AgifyResp
	National  *NationalizeResp
	Errors    map[string]error
}

// EnrichName calls all three APIs concurrently and returns the combined result.
// Using goroutines means total wait time = slowest API, not sum of all three.
func EnrichName(name string) EnrichResult {
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		result EnrichResult
	)
	result.Errors = make(map[string]error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		var g GenderizeResp
		err := FetchJSON("https://api.genderize.io?name="+name, &g)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			result.Errors["Genderize"] = err
		} else {
			result.Genderize = &g
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var a AgifyResp
		err := FetchJSON("https://api.agify.io?name="+name, &a)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			result.Errors["Agify"] = err
		} else {
			result.Agify = &a
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var n NationalizeResp
		err := FetchJSON("https://api.nationalize.io?name="+name, &n)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			result.Errors["Nationalize"] = err
		} else {
			result.National = &n
		}
	}()

	wg.Wait()
	return result
}
