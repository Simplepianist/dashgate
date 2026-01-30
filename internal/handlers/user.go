package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"dashgate/internal/auth"
	"dashgate/internal/server"
)

// UserPreferencesHandler handles GET (load) and PUT (save) for user preferences.
func UserPreferencesHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetAuthenticatedUser(app, r)
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Get user ID from database
		var userID int
		err := app.DB.QueryRow("SELECT id FROM users WHERE username = ?", user.Username).Scan(&userID)
		if err != nil {
			// User might be from LDAP/OIDC, create a preferences record with username as key
			userID = 0 // Will use username-based lookup
		}

		switch r.Method {
		case http.MethodGet:
			var preferences string
			if userID > 0 {
				err = app.DB.QueryRow("SELECT preferences FROM user_preferences WHERE user_id = ?", userID).Scan(&preferences)
			} else {
				// For external users (LDAP/OIDC), use username column lookup
				err = app.DB.QueryRow("SELECT preferences FROM user_preferences WHERE username = ?",
					user.Username).Scan(&preferences)
			}

			if err != nil {
				// Return empty preferences
				preferences = "{}"
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(preferences))

		case http.MethodPut:
			var prefs map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			prefsJSON, _ := json.Marshal(prefs)

			if userID > 0 {
				_, err = app.DB.Exec(`INSERT OR REPLACE INTO user_preferences (user_id, preferences, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
					userID, string(prefsJSON))
			} else {
				// For external users, use username column with INSERT OR REPLACE
				_, err = app.DB.Exec(`INSERT OR REPLACE INTO user_preferences (user_id, username, preferences, updated_at) VALUES (-1, ?, ?, CURRENT_TIMESTAMP)`,
					user.Username, string(prefsJSON))
			}

			if err != nil {
				log.Printf("Error saving preferences: %v", err)
				http.Error(w, "Failed to save preferences", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "saved"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
