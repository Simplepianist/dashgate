package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"dashgate/internal/config"
	"dashgate/internal/models"
	"dashgate/internal/server"
)

// IconCache hält gecachte Icon-URLs von selfh.st
var (
	iconCache     = make(map[string]string)
	iconCacheMu   sync.RWMutex
	iconCacheTime time.Time
	iconCacheTTL  = 24 * time.Hour
)

// validateSVGContent checks SVG content for dangerous XSS patterns.
func validateSVGContent(content []byte) error {
	contentStr := strings.ToLower(string(content))
	dangerousPatterns := []string{
		"<script", "javascript:", "vbscript:", "data:text/html", "data:image/svg+xml",
		"onerror", "onload", "onclick", "onmouseover", "onmouseout",
		"onfocus", "onblur", "oninput", "onchange", "onsubmit",
		"onkeydown", "onkeyup", "onkeypress", "onmousedown", "onmouseup",
		"ondblclick", "oncontextmenu", "ondrag", "ondragend", "ondragenter",
		"ondragleave", "ondragover", "ondragstart", "ondrop", "onscroll",
		"onwheel", "oncopy", "oncut", "onpaste", "onanimationend",
		"onanimationstart", "ontransitionend", "onresize", "ontoggle",
		"onbegin", "onend", "onrepeat",
		"<foreignobject", "<iframe", "<embed", "<object", "<handler",
		"expression(",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(contentStr, pattern) {
			return fmt.Errorf("SVG contains potentially unsafe content: %s", pattern)
		}
	}
	return nil
}

// SelfhstIconHandler sucht und lädt Icons von selfh.st automatisch
func SelfhstIconHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			AppURL string `json:"appUrl"`
			AppName string `json:"appName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.AppURL == "" {
			http.Error(w, "App URL required", http.StatusBadRequest)
			return
		}

		// Extrahiere Domain aus URL
		parsedURL, err := url.Parse(req.AppURL)
		if err != nil {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}

		domain := parsedURL.Hostname()
		// Entferne www. prefix
		domain = strings.TrimPrefix(domain, "www.")

		// Suche Icon auf selfh.st
		iconURL, iconName, err := findSelfhstIcon(app, domain, req.AppName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Icon not found: %v", err), http.StatusNotFound)
			return
		}

		// Lade Icon herunter
		filename, err := downloadAndSaveIcon(app, iconURL, iconName)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to download icon: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"filename": filename,
			"status":   "downloaded",
		})
	}
}

// findSelfhstIcon sucht nach einem passenden Icon auf selfh.st
func findSelfhstIcon(app *server.App, domain, appName string) (iconURL, iconName string, err error) {
	// Lade Icon-Liste von selfh.st
	iconCacheMu.RLock()
	if time.Since(iconCacheTime) < iconCacheTTL && len(iconCache) > 0 {
		iconCacheMu.RUnlock()
		// Suche in Cache
		return searchIconInCache(domain, appName)
	}
	iconCacheMu.RUnlock()

	// Lade neue Icon-Liste
	resp, err := app.HTTPClient.Get("https://api.selfh.st/apps")
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch icon list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("selfh.st API returned status %d", resp.StatusCode)
	}

	var apps []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&apps); err != nil {
		return "", "", fmt.Errorf("failed to parse icon list: %w", err)
	}

	// Aktualisiere Cache
	iconCacheMu.Lock()
	iconCache = make(map[string]string)
	for _, app := range apps {
		iconCache[strings.ToLower(app.ID)] = app.URL
		iconCache[strings.ToLower(app.Name)] = app.URL
	}
	iconCacheTime = time.Now()
	iconCacheMu.Unlock()

	return searchIconInCache(domain, appName)
}

// searchIconInCache sucht nach einem Icon im Cache
func searchIconInCache(domain, appName string) (iconURL, iconName string, err error) {
	iconCacheMu.RLock()
	defer iconCacheMu.RUnlock()

	// Versuche verschiedene Suchstrategien
	searchTerms := []string{
		strings.ToLower(domain),
		strings.ToLower(appName),
		strings.ToLower(strings.ReplaceAll(appName, " ", "-")),
		strings.ToLower(strings.ReplaceAll(domain, ".", "-")),
	}

	// Entferne TLD für bessere Matches
	domainParts := strings.Split(domain, ".")
	if len(domainParts) > 1 {
		searchTerms = append(searchTerms, strings.ToLower(domainParts[0]))
	}

	for _, term := range searchTerms {
		if url, ok := iconCache[term]; ok {
			return url, term, nil
		}
	}

	return "", "", fmt.Errorf("no icon found for %s / %s", domain, appName)
}

// downloadAndSaveIcon lädt ein Icon herunter und speichert es lokal
func downloadAndSaveIcon(app *server.App, iconURL, iconName string) (string, error) {
	resp, err := app.HTTPClient.Get(iconURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("icon download returned status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return "", err
	}

	// Bestimme Dateiendung
	ext := ".png"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "svg") {
		ext = ".svg"
		// SVG Sicherheitsvalidierung
		if err := validateSVGContent(content); err != nil {
			return "", fmt.Errorf("SVG validation failed: %w", err)
		}
	} else if strings.Contains(contentType, "webp") {
		ext = ".webp"
	}

	// Erstelle eindeutigen Dateinamen
	sanitized := regexp.MustCompile(`[^a-zA-Z0-9-_]`).ReplaceAllString(iconName, "-")
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])[:8]
	filename := fmt.Sprintf("%s-%s%s", sanitized, hashStr, ext)

	// Speichere Datei
	dstPath := filepath.Join(app.IconsPath, filename)
	cleanDst := filepath.Clean(dstPath)
	cleanBase := filepath.Clean(app.IconsPath)
	if !strings.HasPrefix(cleanDst, cleanBase) {
		return "", fmt.Errorf("invalid filename")
	}

	if err := os.WriteFile(dstPath, content, 0644); err != nil {
		return "", err
	}

	return filename, nil
}

// AutoIconUpdateHandler aktualisiert Icons automatisch für alle Apps
func AutoIconUpdateHandler(app *server.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		results := map[string]interface{}{
			"updated": 0,
			"failed":  0,
			"skipped": 0,
			"details": []map[string]string{},
		}

		app.ConfigMu.RLock()
		categories := make([]models.Category, len(app.Config.Categories))
		copy(categories, app.Config.Categories)
		app.ConfigMu.RUnlock()

		for _, cat := range categories {
			for _, a := range cat.Apps {
				// Überspringe Apps die bereits ein lokales Icon haben
				if a.Icon != "" && !strings.HasPrefix(a.Icon, "mdi:") {
					results["skipped"] = results["skipped"].(int) + 1
					continue
				}

				parsedURL, err := url.Parse(a.URL)
				if err != nil {
					results["failed"] = results["failed"].(int) + 1
					results["details"] = append(results["details"].([]map[string]string), map[string]string{
						"app":   a.Name,
						"error": "Invalid URL",
					})
					continue
				}

				domain := parsedURL.Hostname()
				domain = strings.TrimPrefix(domain, "www.")

				iconURL, iconName, err := findSelfhstIcon(app, domain, a.Name)
				if err != nil {
					results["failed"] = results["failed"].(int) + 1
					results["details"] = append(results["details"].([]map[string]string), map[string]string{
						"app":   a.Name,
						"error": fmt.Sprintf("Icon not found: %v", err),
					})
					continue
				}

				filename, err := downloadAndSaveIcon(app, iconURL, iconName)
				if err != nil {
					results["failed"] = results["failed"].(int) + 1
					results["details"] = append(results["details"].([]map[string]string), map[string]string{
						"app":   a.Name,
						"error": fmt.Sprintf("Download failed: %v", err),
					})
					continue
				}

				// Aktualisiere Icon in Config
				app.ConfigMu.Lock()
				for i := range app.Config.Categories {
					if app.Config.Categories[i].Name == cat.Name {
						for j := range app.Config.Categories[i].Apps {
							if app.Config.Categories[i].Apps[j].URL == a.URL {
								app.Config.Categories[i].Apps[j].Icon = filename
								break
							}
						}
					}
				}
				app.ConfigMu.Unlock()

				results["updated"] = results["updated"].(int) + 1
				results["details"] = append(results["details"].([]map[string]string), map[string]string{
					"app":  a.Name,
					"icon": filename,
				})
			}
		}

		// Speichere Config nur wenn Updates gemacht wurden
		if results["updated"].(int) > 0 {
			if err := config.SaveConfig(app); err != nil {
				log.Printf("Error saving config after icon updates: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

