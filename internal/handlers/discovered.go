package handlers

import (
	"encoding/json"
	"net/http"

	"dashgate/internal/auth"
	"dashgate/internal/database"
	"dashgate/internal/discovery"
	"dashgate/internal/models"
	"dashgate/internal/server"
)

// DiscoveredAppsHandler returns the list of Docker-discovered apps.
func DiscoveredAppsHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.GetAuthenticatedUser(app, r)
		if user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		apps := app.DockerDiscovery.GetApps()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apps)
	}
}

// AdminDiscoveredAppsHandler manages discovered app overrides (GET/PUT/DELETE).
func AdminDiscoveredAppsHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Return active discovered apps + stale overrides
			rawApps := discovery.GetAllRawDiscoveredApps(app)
			allOverrides := database.GetAllDiscoveredOverrides(app)

			// Track which override URLs are still actively discovered
			activeURLs := make(map[string]bool)
			for _, dApp := range rawApps {
				activeURLs[dApp.URL] = true
			}

			// Find stale overrides (configured but no longer discovered)
			var stale []models.DiscoveredAppOverride
			for url, override := range allOverrides {
				if !activeURLs[url] {
					stale = append(stale, *override)
				}
			}

			response := map[string]interface{}{
				"active": rawApps,
				"stale":  stale,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)

		case http.MethodPut:
			var o models.DiscoveredAppOverride
			if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
				http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
			if o.URL == "" {
				http.Error(w, "URL is required", http.StatusBadRequest)
				return
			}
			if o.Groups == nil {
				o.Groups = []string{}
			}
			if err := database.SaveDiscoveredOverride(app, &o); err != nil {
				http.Error(w, "Failed to save: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		case http.MethodDelete:
			url := r.URL.Query().Get("url")
			if url == "" {
				http.Error(w, "URL parameter required", http.StatusBadRequest)
				return
			}
			if err := database.DeleteDiscoveredOverride(app, url); err != nil {
				http.Error(w, "Failed to delete: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
