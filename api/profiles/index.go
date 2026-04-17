package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	_ "github.com/lib/pq"
)

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

type GenderizeResponse struct {
	Name        string  `json:"name"`
	Gender      string  `json:"gender"`
	Probability float64 `json:"probability"`
	Count       int     `json:"count"`
}

type AgifyResponse struct {
	Name string `json:"name"`
	Age  *int   `json:"age"`
}

type NationalizeResponse struct {
	Name    string `json:"name"`
	Country []struct {
		CountryID   string  `json:"country_id"`
		Probability float64 `json:"probability"`
	} `json:"country"`
}

func getAgeGroup(age int) string {
	if age <= 12 {
		return "child"
	} else if age <= 19 {
		return "teenager"
	} else if age <= 59 {
		return "adult"
	} else {
		return "senior"
	}
}

func callAPI(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(target)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		http.Error(w, `{"status":"error","message":"Database not configured"}`, 500)
		return
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		http.Error(w, `{"status":"error","message":"Database connection failed"}`, 500)
		return
	}
	defer db.Close()

	if r.Method == "POST" {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"status":"error","message":"Invalid JSON"}`, 400)
			return
		}

		nameValue, ok := body["name"]
		if !ok {
			http.Error(w, `{"status":"error","message":"Missing or empty name"}`, 400)
			return
		}

		nameStr, ok := nameValue.(string)
		if !ok {
			http.Error(w, `{"status":"error","message":"Invalid type"}`, 422)
			return
		}
		if strings.TrimSpace(nameStr) == "" {
			http.Error(w, `{"status":"error","message":"Missing or empty name"}`, 400)
			return
		}
		name := strings.ToLower(strings.TrimSpace(nameStr))

		// Check if exists
		var existing Profile
		err = db.QueryRow("SELECT id, name, gender, gender_probability, sample_size, age, age_group, country_id, country_probability, created_at FROM profiles WHERE name = $1", name).Scan(&existing.ID, &existing.Name, &existing.Gender, &existing.GenderProbability, &existing.SampleSize, &existing.Age, &existing.AgeGroup, &existing.CountryID, &existing.CountryProbability, &existing.CreatedAt)
		if err == nil {
			response := map[string]interface{}{
				"status":  "success",
				"message": "Profile already exists",
				"data":    existing,
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		if err != nil && err != sql.ErrNoRows {
			http.Error(w, `{"status":"error","message":"Database query failed"}`, 500)
			return
		}

		// Call APIs
		var genderResp GenderizeResponse
		if err := callAPI(fmt.Sprintf("https://api.genderize.io?name=%s", name), &genderResp); err != nil || genderResp.Gender == "" || genderResp.Count == 0 {
			http.Error(w, `{"status":"error","message":"Genderize returned an invalid response"}`, 502)
			return
		}

		var agifyResp AgifyResponse
		if err := callAPI(fmt.Sprintf("https://api.agify.io?name=%s", name), &agifyResp); err != nil || agifyResp.Age == nil {
			http.Error(w, `{"status":"error","message":"Agify returned an invalid response"}`, 502)
			return
		}

		var natResp NationalizeResponse
		if err := callAPI(fmt.Sprintf("https://api.nationalize.io?name=%s", name), &natResp); err != nil || len(natResp.Country) == 0 {
			http.Error(w, `{"status":"error","message":"Nationalize returned an invalid response"}`, 502)
			return
		}

		// Find top country
		topCountry := natResp.Country[0]
		for _, c := range natResp.Country {
			if c.Probability > topCountry.Probability {
				topCountry = c
			}
		}

		age := *agifyResp.Age
		ageGroup := getAgeGroup(age)

		id, _ := uuid.NewV7()
		profile := Profile{
			ID:                 id.String(),
			Name:               name,
			Gender:             genderResp.Gender,
			GenderProbability:  genderResp.Probability,
			SampleSize:         genderResp.Count,
			Age:                age,
			AgeGroup:           ageGroup,
			CountryID:          topCountry.CountryID,
			CountryProbability: topCountry.Probability,
			CreatedAt:          time.Now().UTC().Format(time.RFC3339),
		}

		_, err = db.Exec("INSERT INTO profiles (id, name, gender, gender_probability, sample_size, age, age_group, country_id, country_probability, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)", profile.ID, profile.Name, profile.Gender, profile.GenderProbability, profile.SampleSize, profile.Age, profile.AgeGroup, profile.CountryID, profile.CountryProbability, profile.CreatedAt)
		if err != nil {
			http.Error(w, `{"status":"error","message":"Failed to save profile"}`, 500)
			return
		}

		response := map[string]interface{}{
			"status": "success",
			"data":   profile,
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(response)

	} else if r.Method == "GET" {
		query := "SELECT id, name, gender, age, age_group, country_id FROM profiles"
		args := []interface{}{}
		conditions := []string{}

		if gender := r.URL.Query().Get("gender"); gender != "" {
			conditions = append(conditions, "gender = $"+strconv.Itoa(len(args)+1))
			args = append(args, strings.ToLower(gender))
		}
		if countryID := r.URL.Query().Get("country_id"); countryID != "" {
			conditions = append(conditions, "country_id = $"+strconv.Itoa(len(args)+1))
			args = append(args, strings.ToUpper(countryID))
		}
		if ageGroup := r.URL.Query().Get("age_group"); ageGroup != "" {
			conditions = append(conditions, "age_group = $"+strconv.Itoa(len(args)+1))
			args = append(args, strings.ToLower(ageGroup))
		}

		if len(conditions) > 0 {
			query += " WHERE " + strings.Join(conditions, " AND ")
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, `{"status":"error","message":"Failed to fetch profiles"}`, 500)
			return
		}
		defer rows.Close()

		var profiles []map[string]interface{}
		for rows.Next() {
			var id, name, gender, ageGroup, countryID string
			var age int
			err := rows.Scan(&id, &name, &gender, &age, &ageGroup, &countryID)
			if err != nil {
				continue
			}
			profiles = append(profiles, map[string]interface{}{
				"id":         id,
				"name":       name,
				"gender":     gender,
				"age":        age,
				"age_group":  ageGroup,
				"country_id": countryID,
			})
		}

		response := map[string]interface{}{
			"status": "success",
			"count":  len(profiles),
			"data":   profiles,
		}
		json.NewEncoder(w).Encode(response)

	} else {
		http.Error(w, `{"status":"error","message":"Method not allowed"}`, 405)
	}
}
