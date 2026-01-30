package auth

import (
	"encoding/json"
	"log"
	"net/http"

	"dashgate/internal/models"
	"dashgate/internal/server"
)

// GetLocalUser authenticates the user via a session cookie stored in the
// local SQLite database. Returns nil if no valid session is found.
func GetLocalUser(app *server.App, r *http.Request) *models.AuthenticatedUser {
	if app.DB == nil {
		return nil
	}

	cookie, err := r.Cookie(app.AuthConfig.CookieName)
	if err != nil {
		return nil
	}

	var username, email, displayName, groupsJSON, passwordHash string
	err = app.DB.QueryRow(
		"SELECT u.id, u.username, u.email, u.display_name, u.groups, u.password_hash FROM sessions s JOIN users u ON s.user_id = u.id WHERE s.token = ? AND s.expires_at > datetime('now')",
		cookie.Value,
	).Scan(new(int), &username, &email, &displayName, &groupsJSON, &passwordHash)
	if err != nil {
		return nil
	}

	// Handle NULL values
	if email == "" {
		email = ""
	}
	if displayName == "" {
		displayName = username
	}
	if groupsJSON == "" {
		groupsJSON = "[]"
	}

	var groups []string
	if err := json.Unmarshal([]byte(groupsJSON), &groups); err != nil {
		log.Printf("Error parsing groups JSON: %v", err)
		groups = []string{}
	}

	source := "local"
	switch passwordHash {
	case "LDAP_USER":
		source = "ldap"
	case "OIDC_USER":
		source = "oidc"
	}

	user := &models.AuthenticatedUser{
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Groups:      groups,
		Source:      source,
	}
	user.IsAdmin = CheckIsAdmin(app, user.Groups)
	return user
}
