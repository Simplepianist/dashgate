package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"dashgate/internal/auth"
	"dashgate/internal/models"
	"dashgate/internal/server"
)

// AutoLoginRedirect ist eine Middleware die bei fehlender Auth automatisch zum Login redirected
// oder bei OIDC-Only zum OIDC Endpoint
func AutoLoginRedirect(app *server.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Überspringe für öffentliche Endpoints
			publicPaths := []string{
				"/login",
				"/logout",
				"/setup",
				"/health",
				"/static/",
				"/manifest.json",
				"/sw.js",
				"/offline",
				"/auth/oidc",
				"/auth/oidc/callback",
			}

			for _, path := range publicPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			user := auth.GetAuthenticatedUser(app, r)
			if user == nil {
				// Prüfe ob es ein API Request ist (JSON expected)
				acceptHeader := r.Header.Get("Accept")
				isAPIRequest := strings.Contains(acceptHeader, "application/json") ||
					strings.HasPrefix(r.URL.Path, "/api/")

				if isAPIRequest {
					// API Requests bekommen JSON Response
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					json.NewEncoder(w).Encode(map[string]string{
						"error":    "unauthorized",
						"redirect": GetAuthRedirectURL(app),
					})
					return
				}

				// Browser Requests werden zum Login oder OIDC redirected
				redirectURL := GetAuthRedirectURL(app)
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetAuthRedirectURL bestimmt den richtigen Redirect URL basierend auf AuthMode und SystemConfig
func GetAuthRedirectURL(app *server.App) string {
	// Authelia ist der OIDC Mode
	switch app.AuthConfig.Mode {
	case models.AuthModeAuthelia:
		// Authelia/OIDC: Direkter OIDC Redirect
		return "/auth/oidc"
	case models.AuthModeLocal, models.AuthModeHybrid:
		// Local oder Hybrid: Login Page
		return "/login"
	default:
		// Fallback
		return "/login"
	}
}

