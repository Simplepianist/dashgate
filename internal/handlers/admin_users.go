package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dashgate/internal/auth"
	"dashgate/internal/database"
	"dashgate/internal/models"
	"dashgate/internal/server"
)

// LocalUsersHandler handles GET (list) and POST (create) for local users.
func LocalUsersHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if app.DB == nil {
			http.Error(w, "Local auth not enabled", http.StatusServiceUnavailable)
			return
		}

		switch r.Method {
		case http.MethodGet:
			listLocalUsers(app, w, r)
		case http.MethodPost:
			createLocalUser(app, w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// LocalUserHandler routes operations on a single local user by ID.
func LocalUserHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())

		if app.DB == nil {
			http.Error(w, "Local auth not enabled", http.StatusServiceUnavailable)
			return
		}

		// Extract user ID from path
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/local-users/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		userID, err := strconv.Atoi(parts[0])
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		// Check for password reset endpoint
		if len(parts) > 1 && parts[1] == "password" {
			resetUserPassword(app, w, r, userID)
			return
		}

		switch r.Method {
		case http.MethodPut:
			updateLocalUser(app, w, r, userID, user.Username)
		case http.MethodDelete:
			deleteLocalUser(app, w, r, userID, user.Username)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listLocalUsers(app *server.App, w http.ResponseWriter, r *http.Request) {
	rows, err := app.DB.Query(
		"SELECT id, username, COALESCE(email, ''), COALESCE(display_name, username), COALESCE(groups, '[]'), created_at, updated_at FROM users ORDER BY username",
	)
	if err != nil {
		log.Printf("Error listing users: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []models.LocalUser
	for rows.Next() {
		var u models.LocalUser
		var groupsJSON string
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &groupsJSON, &u.CreatedAt, &u.UpdatedAt); err != nil {
			log.Printf("Error scanning user: %v", err)
			continue
		}
		json.Unmarshal([]byte(groupsJSON), &u.Groups)
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating users rows: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func createLocalUser(app *server.App, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string   `json:"username"`
		Email       string   `json:"email"`
		Password    string   `json:"password"`
		DisplayName string   `json:"displayName"`
		Groups      []string `json:"groups"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Hash password using SHA-256 pre-hash to handle passwords longer than bcrypt's 72-byte limit
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	groupsJSON, _ := json.Marshal(req.Groups)
	if req.Groups == nil {
		groupsJSON = []byte("[]")
	}

	result, err := app.DB.Exec(
		"INSERT INTO users (username, email, password_hash, display_name, groups) VALUES (?, ?, ?, ?, ?)",
		req.Username, req.Email, hashedPassword, req.DisplayName, string(groupsJSON),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Username or email already exists", http.StatusConflict)
			return
		}
		log.Printf("Error creating user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	adminUser := auth.GetUserFromContext(r)
	adminName := ""
	if adminUser != nil {
		adminName = adminUser.Username
	}
	database.LogAudit(app, adminName, "user_created", fmt.Sprintf("Created user %q (id=%d)", req.Username, id), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created", "id": id})
}

func updateLocalUser(app *server.App, w http.ResponseWriter, r *http.Request, userID int, currentUsername string) {
	var req struct {
		Email       string   `json:"email"`
		DisplayName string   `json:"displayName"`
		Groups      []string `json:"groups"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Prevent admin from removing their own admin role
	var targetUsername string
	app.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&targetUsername)
	if targetUsername == currentUsername {
		if !auth.CheckIsAdmin(app, req.Groups) {
			http.Error(w, "Cannot remove admin role from your own account", http.StatusForbidden)
			return
		}
	}

	groupsJSON, _ := json.Marshal(req.Groups)
	if req.Groups == nil {
		groupsJSON = []byte("[]")
	}

	result, err := app.DB.Exec(
		"UPDATE users SET email = ?, display_name = ?, groups = ?, updated_at = ? WHERE id = ?",
		req.Email, req.DisplayName, string(groupsJSON), time.Now(), userID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "Email already exists", http.StatusConflict)
			return
		}
		log.Printf("Error updating user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Invalidate all sessions for this user since their privileges changed
	database.InvalidateUserSessions(app, userID)

	database.LogAudit(app, currentUsername, "user_updated", fmt.Sprintf("Updated user id=%d", userID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func deleteLocalUser(app *server.App, w http.ResponseWriter, r *http.Request, userID int, currentUsername string) {
	// Get username of user to be deleted
	var username string
	err := app.DB.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Error getting user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Prevent self-deletion
	if username == currentUsername {
		http.Error(w, "Cannot delete yourself", http.StatusBadRequest)
		return
	}

	// Invalidate all sessions for this user before deletion
	database.InvalidateUserSessions(app, userID)

	// Delete user (sessions would also cascade delete via foreign key)
	result, err := app.DB.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		log.Printf("Error deleting user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	database.LogAudit(app, currentUsername, "user_deleted", fmt.Sprintf("Deleted user %q (id=%d)", username, userID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func resetUserPassword(app *server.App, w http.ResponseWriter, r *http.Request, userID int) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Password == "" {
		http.Error(w, "Password required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Hash new password using SHA-256 pre-hash to handle passwords longer than bcrypt's 72-byte limit
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Update password and timestamp
	result, err := app.DB.Exec(
		"UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?",
		hashedPassword, time.Now(), userID,
	)
	if err != nil {
		log.Printf("Error updating password: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Invalidate all user sessions after password reset
	database.InvalidateUserSessions(app, userID)

	adminUser := auth.GetUserFromContext(r)
	adminName := ""
	if adminUser != nil {
		adminName = adminUser.Username
	}
	database.LogAudit(app, adminName, "password_reset", fmt.Sprintf("Reset password for user id=%d", userID), r.RemoteAddr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "password_reset"})
}
