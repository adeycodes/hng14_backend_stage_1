// Vercel serverless function — handles:
//   GET    /api/profiles/{id}   → get single profile
//   DELETE /api/profiles/{id}   → delete profile
//
// Vercel routes /api/profiles/:id to this file because of the [id] filename.

package handler

import (
	"database/sql"
	"net/http"

	"github.com/adeycodes/hng14_backend_stage_1/internal/shared"
)

// Handler is the Vercel entrypoint — must be named exactly "Handler"
func Handler(w http.ResponseWriter, r *http.Request) {
	shared.WithCORS(route)(w, r)
}

func route(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getProfile(w, r)
	case http.MethodDelete:
		deleteProfile(w, r)
	default:
		shared.ErrJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// ─── GET /api/profiles/{id} ───────────────────────────────────────────────────

func getProfile(w http.ResponseWriter, r *http.Request) {
	// Vercel injects dynamic path params as query params
	// e.g. /api/profiles/abc123 → r.URL.Query().Get("id") = "abc123"
	id := r.URL.Query().Get("id")
	if id == "" {
		shared.ErrJSON(w, http.StatusBadRequest, "Missing profile ID")
		return
	}

	db := shared.DB()

	var profile shared.Profile
	err := db.QueryRow(`
		SELECT id, name, gender, gender_probability, sample_size,
		       age, age_group, country_id, country_probability, created_at
		FROM profiles WHERE id = $1
	`, id).Scan(
		&profile.ID, &profile.Name, &profile.Gender, &profile.GenderProbability,
		&profile.SampleSize, &profile.Age, &profile.AgeGroup,
		&profile.CountryID, &profile.CountryProbability, &profile.CreatedAt,
	)
	if err == sql.ErrNoRows {
		shared.ErrJSON(w, http.StatusNotFound, "Profile not found")
		return
	}
	if err != nil {
		shared.ErrJSON(w, http.StatusInternalServerError, "Database error")
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"data":   profile,
	})
}

// ─── DELETE /api/profiles/{id} ────────────────────────────────────────────────

func deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		shared.ErrJSON(w, http.StatusBadRequest, "Missing profile ID")
		return
	}

	db := shared.DB()

	result, err := db.Exec(`DELETE FROM profiles WHERE id = $1`, id)
	if err != nil {
		shared.ErrJSON(w, http.StatusInternalServerError, "Database error")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		shared.ErrJSON(w, http.StatusNotFound, "Profile not found")
		return
	}

	// 204 No Content — no body required
	w.WriteHeader(http.StatusNoContent)
}
