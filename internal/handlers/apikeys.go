package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"dashgate/internal/auth"
	"dashgate/internal/database"
	"dashgate/internal/models"
	"dashgate/internal/server"

	"golang.org/x/crypto/bcrypt"
)

// APIKeysHandler routes GET (list), POST (create), and DELETE operations for API keys.
func APIKeysHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listAPIKeys(app, w, r)
		case http.MethodPost:
			createAPIKey(app, w, r)
		case http.MethodDelete:
			deleteAPIKey(app, w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listAPIKeys(app *server.App, w http.ResponseWriter, r *http.Request) {
	rows, err := app.DB.Query(
		"SELECT id, name, key_prefix, username, groups, permissions, expires_at, last_used_at, created_at FROM api_keys ORDER BY created_at DESC",
	)
	if err != nil {
		log.Printf("Error listing API keys: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		var k models.APIKey
		var groupsJSON, permsJSON string
		var expiresAt, lastUsedAt sql.NullTime

		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Username, &groupsJSON, &permsJSON, &expiresAt, &lastUsedAt, &k.CreatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(groupsJSON), &k.Groups)
		json.Unmarshal([]byte(permsJSON), &k.Permissions)
		if expiresAt.Valid {
			k.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			k.LastUsedAt = &lastUsedAt.Time
		}
		keys = append(keys, k)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating API keys rows: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func createAPIKey(app *server.App, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Username    string   `json:"username"`
		Groups      []string `json:"groups"`
		Permissions []string `json:"permissions"`
		ExpiresIn   int      `json:"expiresIn"` // days, 0 = never
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	if req.Username == "" {
		req.Username = "api-key"
	}

	if len(req.Permissions) == 0 {
		req.Permissions = []string{"read"}
	}

	// Generate API key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		log.Printf("Error generating random bytes for API key: %v", err)
		http.Error(w, "Failed to generate key", http.StatusInternalServerError)
		return
	}
	apiKey := base64.URLEncoding.EncodeToString(keyBytes)
	keyPrefix := apiKey[:8]

	// Hash the key
	keyHash, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to generate key", http.StatusInternalServerError)
		return
	}

	groupsJSON, _ := json.Marshal(req.Groups)
	permsJSON, _ := json.Marshal(req.Permissions)

	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * 24 * time.Hour)
		expiresAt = &t
	}

	result, err := app.DB.Exec(
		"INSERT INTO api_keys (name, key_hash, key_prefix, username, groups, permissions, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		req.Name, string(keyHash), keyPrefix, req.Username, string(groupsJSON), string(permsJSON), expiresAt,
	)
	if err != nil {
		log.Printf("Error creating API key: %v", err)
		http.Error(w, "Failed to create key", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	adminUser := auth.GetUserFromContext(r)
	adminName := ""
	if adminUser != nil {
		adminName = adminUser.Username
	}
	database.LogAudit(app, adminName, "api_key_created", fmt.Sprintf("Created API key %q (id=%d, prefix=%s)", req.Name, id, keyPrefix), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     id,
		"name":   req.Name,
		"key":    apiKey, // Only returned once!
		"prefix": keyPrefix,
	})
}

func deleteAPIKey(app *server.App, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "ID required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	result, err := app.DB.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		log.Printf("Error deleting API key: %v", err)
		http.Error(w, "Failed to delete key", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "Key not found", http.StatusNotFound)
		return
	}

	adminUser := auth.GetUserFromContext(r)
	adminName := ""
	if adminUser != nil {
		adminName = adminUser.Username
	}
	database.LogAudit(app, adminName, "api_key_deleted", fmt.Sprintf("Deleted API key id=%d", id), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}
