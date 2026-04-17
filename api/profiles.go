// Vercel serverless function — handles:
//   POST   /api/profiles   → create profile
//   GET    /api/profiles   → list profiles (filterable)
//
// Vercel routes requests to this file based on its path: api/profiles.go

package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/adeycodes/hng14_backend_stage_1/internal/shared"
)

// Handler is the Vercel entrypoint — must be named exactly "Handler"
func Handler(w http.ResponseWriter, r *http.Request) {
	shared.WithCORS(route)(w, r)
}

func route(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		createProfile(w, r)
	case http.MethodGet:
		listProfiles(w, r)
	default:
		shared.ErrJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// ─── POST /api/profiles ───────────────────────────────────────────────────────

func createProfile(w http.ResponseWriter, r *http.Request) {
	// 1. Parse body
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		shared.ErrJSON(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	name := strings.TrimSpace(body.Name)

	if name == "" {
		shared.ErrJSON(w, http.StatusBadRequest, "Missing or empty 'name' field")
		return
	}
	if !shared.IsValidName(name) {
		shared.ErrJSON(w, http.StatusUnprocessableEntity, "Invalid 'name': must contain alphabetic characters")
		return
	}

	db := shared.DB()

	// 2. Idempotency — return existing record if name already stored
	var existing shared.Profile
	err := db.QueryRow(`
		SELECT id, name, gender, gender_probability, sample_size,
		       age, age_group, country_id, country_probability, created_at
		FROM profiles WHERE LOWER(name) = LOWER($1)
	`, name).Scan(
		&existing.ID, &existing.Name, &existing.Gender, &existing.GenderProbability,
		&existing.SampleSize, &existing.Age, &existing.AgeGroup,
		&existing.CountryID, &existing.CountryProbability, &existing.CreatedAt,
	)
	if err == nil {
		// Found — skip all external API calls, return cached data
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "success",
			"message": "Profile already exists",
			"data":    existing,
		})
		return
	}
	if err != sql.ErrNoRows {
		shared.ErrJSON(w, http.StatusInternalServerError, "Database lookup failed")
		return
	}

	// 3. Enrich via all three external APIs (concurrent)
	enriched := shared.EnrichName(name)

	// Check for any API-level errors
	for apiName, apiErr := range enriched.Errors {
		if apiErr != nil {
			shared.ErrUpstream(w, apiName)
			return
		}
	}

	// 4. Validate edge cases — do not store null/empty responses
	g := enriched.Genderize
	if g == nil || g.Gender == nil || g.Count == 0 {
		shared.ErrUpstream(w, "Genderize")
		return
	}

	a := enriched.Agify
	if a == nil || a.Age == nil {
		shared.ErrUpstream(w, "Agify")
		return
	}

	n := enriched.National
	if n == nil || len(n.Country) == 0 {
		shared.ErrUpstream(w, "Nationalize")
		return
	}

	// 5. Pick the country with the highest probability
	topCountry := n.Country[0]
	for _, c := range n.Country {
		if c.Probability > topCountry.Probability {
			topCountry = c
		}
	}

	// 6. Build profile
	profile := shared.Profile{
		ID:                 shared.NewUUID(),
		Name:               name,
		Gender:             *g.Gender,
		GenderProbability:  g.Probability,
		SampleSize:         g.Count,
		Age:                *a.Age,
		AgeGroup:           shared.ClassifyAge(*a.Age),
		CountryID:          topCountry.CountryID,
		CountryProbability: topCountry.Probability,
		CreatedAt:          shared.NowUTC(),
	}

	// 7. Persist
	_, err = db.Exec(`
		INSERT INTO profiles
			(id, name, gender, gender_probability, sample_size,
			 age, age_group, country_id, country_probability, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		profile.ID, profile.Name, profile.Gender, profile.GenderProbability,
		profile.SampleSize, profile.Age, profile.AgeGroup,
		profile.CountryID, profile.CountryProbability, profile.CreatedAt,
	)
	if err != nil {
		shared.ErrJSON(w, http.StatusInternalServerError, "Failed to store profile")
		return
	}

	shared.WriteJSON(w, http.StatusCreated, map[string]any{
		"status": "success",
		"data":   profile,
	})
}

// ─── GET /api/profiles ────────────────────────────────────────────────────────

func listProfiles(w http.ResponseWriter, r *http.Request) {
	db := shared.DB()

	// Build dynamic WHERE clause from optional query params
	query := `SELECT id, name, gender, age, age_group, country_id FROM profiles WHERE 1=1`
	args := []any{}
	argIdx := 1

	if gender := r.URL.Query().Get("gender"); gender != "" {
		query += fmt.Sprintf(" AND LOWER(gender) = LOWER($%d)", argIdx)
		args = append(args, gender)
		argIdx++
	}
	if countryID := r.URL.Query().Get("country_id"); countryID != "" {
		query += fmt.Sprintf(" AND LOWER(country_id) = LOWER($%d)", argIdx)
		args = append(args, countryID)
		argIdx++
	}
	if ageGroup := r.URL.Query().Get("age_group"); ageGroup != "" {
		query += fmt.Sprintf(" AND LOWER(age_group) = LOWER($%d)", argIdx)
		args = append(args, ageGroup)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		shared.ErrJSON(w, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()

	profiles := []shared.ProfileSummary{}
	for rows.Next() {
		var p shared.ProfileSummary
		if err := rows.Scan(&p.ID, &p.Name, &p.Gender, &p.Age, &p.AgeGroup, &p.CountryID); err != nil {
			shared.ErrJSON(w, http.StatusInternalServerError, "Failed to read profiles")
			return
		}
		profiles = append(profiles, p)
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"count":  len(profiles),
		"data":   profiles,
	})
}
