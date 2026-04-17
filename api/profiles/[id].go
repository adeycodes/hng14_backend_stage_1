package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"

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

func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get id from path, assuming /api/profiles/{id}
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Error(w, `{"status":"error","message":"Invalid path"}`, 400)
		return
	}
	id := parts[2]

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

	if r.Method == "GET" {
		var profile Profile
		err := db.QueryRow("SELECT id, name, gender, gender_probability, sample_size, age, age_group, country_id, country_probability, created_at FROM profiles WHERE id = $1", id).Scan(&profile.ID, &profile.Name, &profile.Gender, &profile.GenderProbability, &profile.SampleSize, &profile.Age, &profile.AgeGroup, &profile.CountryID, &profile.CountryProbability, &profile.CreatedAt)
		if err != nil {
			http.Error(w, `{"status":"error","message":"Profile not found"}`, 404)
			return
		}
		response := map[string]interface{}{
			"status": "success",
			"data":   profile,
		}
		json.NewEncoder(w).Encode(response)

	} else if r.Method == "DELETE" {
		result, err := db.Exec("DELETE FROM profiles WHERE id = $1", id)
		if err != nil {
			http.Error(w, `{"status":"error","message":"Failed to delete"}`, 500)
			return
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			http.Error(w, `{"status":"error","message":"Failed to delete"}`, 500)
			return
		}
		if rowsAffected == 0 {
			http.Error(w, `{"status":"error","message":"Profile not found"}`, 404)
			return
		}
		w.WriteHeader(204)

	} else {
		http.Error(w, `{"status":"error","message":"Method not allowed"}`, 405)
	}
}
