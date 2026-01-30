package discovery

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"dashgate/internal/models"
	"dashgate/internal/server"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// InitNginxDiscovery checks environment variables and system config,
// then starts the Nginx discovery loop if enabled.
func InitNginxDiscovery(app *server.App) {
	if cp := os.Getenv("NGINX_CONFIG_PATH"); cp != "" {
		app.SysConfigMu.Lock()
		app.SystemConfig.NginxConfigPath = cp
		app.SysConfigMu.Unlock()
	}

	if os.Getenv("NGINX_DISCOVERY") == "true" {
		app.NginxDiscoveryEnvOverride = true
		StartNginxDiscoveryLoop(app)
		app.SysConfigMu.RLock()
		log.Printf("Nginx discovery enabled (via environment variable, config path: %s)", app.SystemConfig.NginxConfigPath)
		app.SysConfigMu.RUnlock()
	} else if app.SystemConfig.NginxDiscoveryEnabled {
		StartNginxDiscoveryLoop(app)
		app.SysConfigMu.RLock()
		log.Printf("Nginx discovery enabled (via database config, config path: %s)", app.SystemConfig.NginxConfigPath)
		app.SysConfigMu.RUnlock()
	}
}

// StartNginxDiscoveryLoop starts the background goroutine that periodically
// parses Nginx configuration files. It is safe to call if already running.
func StartNginxDiscoveryLoop(app *server.App) {
	app.DiscoveryMu.Lock()
	defer app.DiscoveryMu.Unlock()

	if app.NginxDiscovery.Stop != nil {
		return // Already running
	}

	app.NginxDiscovery.Enabled = true
	app.NginxDiscovery.Stop = make(chan struct{})

	app.NginxDiscovery.Wg.Add(1)
	go func() {
		defer app.NginxDiscovery.Wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Nginx discovery goroutine panicked: %v", r)
			}
		}()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		DiscoverNginxApps(app) // Initial discovery
		for {
			select {
			case <-app.NginxDiscovery.Stop:
				return
			case <-ticker.C:
				DiscoverNginxApps(app)
			}
		}
	}()
}

// StopNginxDiscoveryLoop stops the Nginx discovery background loop
// and clears all discovered apps.
func StopNginxDiscoveryLoop(app *server.App) {
	app.DiscoveryMu.Lock()
	defer app.DiscoveryMu.Unlock()

	if app.NginxDiscovery.Stop != nil {
		close(app.NginxDiscovery.Stop)
		app.NginxDiscovery.Stop = nil
	}
	app.DiscoveryMu.Unlock()
	app.NginxDiscovery.Wg.Wait()
	app.DiscoveryMu.Lock()
	app.NginxDiscovery.Enabled = false
	app.NginxDiscovery.ClearApps()
}

// DiscoverNginxApps parses Nginx configuration files in the configured directory
// to discover proxied applications and updates the NginxDiscovery manager.
func DiscoverNginxApps(app *server.App) {
	app.DiscoveryMu.RLock()
	enabled := app.NginxDiscovery.Enabled
	app.DiscoveryMu.RUnlock()
	if !enabled {
		return
	}

	app.SysConfigMu.RLock()
	nginxConfigPath := app.SystemConfig.NginxConfigPath
	app.SysConfigMu.RUnlock()

	if nginxConfigPath == "" {
		nginxConfigPath = "/etc/nginx/conf.d"
	}

	// Check if config directory exists
	info, err := os.Stat(nginxConfigPath)
	if err != nil {
		log.Printf("Nginx config path error: %v", err)
		return
	}
	if !info.IsDir() {
		log.Printf("Nginx config path is not a directory: %s", nginxConfigPath)
		return
	}

	// Regex patterns for parsing nginx config
	serverNameRe := regexp.MustCompile(`server_name\s+([^;]+);`)
	proxyPassRe := regexp.MustCompile(`proxy_pass\s+([^;]+);`)
	listenRe := regexp.MustCompile(`listen\s+(\d+)(\s+ssl)?`)
	includeRe := regexp.MustCompile(`include\s+([^;]+);`)

	var apps []models.App
	seenHosts := make(map[string]bool)

	// Process config files
	processConfigFile := func(filePath string) {
		file, err := os.Open(filePath)
		if err != nil {
			log.Printf("Error opening nginx config file %s: %v", filePath, err)
			return
		}
		defer file.Close()

		var content strings.Builder
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			content.WriteString(scanner.Text())
			content.WriteString("\n")
		}

		configContent := content.String()

		// Find server blocks (simplified parsing)
		// Split by "server {" and process each block
		blocks := strings.Split(configContent, "server {")
		for i, block := range blocks {
			if i == 0 {
				continue // Skip content before first server block
			}

			// Find the end of this server block (count braces)
			braceCount := 1
			endIdx := 0
			for j, char := range block {
				if char == '{' {
					braceCount++
				} else if char == '}' {
					braceCount--
					if braceCount == 0 {
						endIdx = j
						break
					}
				}
			}
			if endIdx == 0 {
				endIdx = len(block)
			}
			serverBlock := block[:endIdx]

			// Extract server_name
			serverNameMatch := serverNameRe.FindStringSubmatch(serverBlock)
			if serverNameMatch == nil {
				continue
			}

			serverNames := strings.Fields(serverNameMatch[1])
			if len(serverNames) == 0 {
				continue
			}

			// Skip catch-all, localhost, default
			validHost := ""
			for _, name := range serverNames {
				name = strings.TrimSpace(name)
				if name == "_" || name == "localhost" || name == "default_server" || name == "" {
					continue
				}
				validHost = name
				break
			}
			if validHost == "" {
				continue
			}

			// Skip if no proxy_pass (static file server)
			proxyPassMatch := proxyPassRe.FindStringSubmatch(serverBlock)
			if proxyPassMatch == nil {
				continue
			}
			upstream := strings.TrimSpace(proxyPassMatch[1])

			// Determine protocol based on listen directive
			protocol := "http"
			listenMatches := listenRe.FindAllStringSubmatch(serverBlock, -1)
			for _, match := range listenMatches {
				port := match[1]
				hasSSL := len(match) > 2 && match[2] != ""
				if port == "443" || hasSSL {
					protocol = "https"
					break
				}
			}

			// Skip duplicates
			if seenHosts[validHost] {
				continue
			}
			seenHosts[validHost] = true

			// Create app name from hostname
			name := validHost
			parts := strings.Split(validHost, ".")
			if len(parts) > 0 {
				name = cases.Title(language.English).String(strings.ReplaceAll(parts[0], "-", " "))
			}

			a := models.App{
				Name:        name,
				URL:         fmt.Sprintf("%s://%s", protocol, validHost),
				Description: fmt.Sprintf("Discovered via Nginx (proxied to %s)", upstream),
				Status:      "online",
			}

			apps = append(apps, a)
		}
	}

	// Read all .conf files in the config directory
	entries, err := os.ReadDir(nginxConfigPath)
	if err != nil {
		log.Printf("Error reading nginx config directory: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".conf") {
			processConfigFile(filepath.Join(nginxConfigPath, entry.Name()))
		}
	}

	// Also check for includes (1 level deep)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".conf") {
			filePath := filepath.Join(nginxConfigPath, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			includeMatches := includeRe.FindAllStringSubmatch(string(data), -1)
			for _, match := range includeMatches {
				includePath := strings.TrimSpace(match[1])
				// Handle relative paths
				if !strings.HasPrefix(includePath, "/") {
					includePath = filepath.Join(nginxConfigPath, includePath)
				}
				// Validate include path stays within base config directory
				absInclude, err := filepath.Abs(includePath)
				if err != nil {
					continue
				}
				absBase, err := filepath.Abs(nginxConfigPath)
				if err != nil {
					continue
				}
				if !strings.HasPrefix(absInclude, absBase) {
					log.Printf("Nginx include path %s escapes base directory, skipping", includePath)
					continue
				}
				// Handle glob patterns
				matches, err := filepath.Glob(includePath)
				if err == nil {
					for _, m := range matches {
						if !strings.HasSuffix(m, ".conf") {
							continue
						}
						// Resolve symlinks and re-validate each match against the base directory
						resolvedMatch, err := filepath.EvalSymlinks(m)
						if err != nil {
							log.Printf("Nginx include: cannot resolve symlinks for %s, skipping", m)
							continue
						}
						absResolved, err := filepath.Abs(resolvedMatch)
						if err != nil || !strings.HasPrefix(absResolved, absBase) {
							log.Printf("Nginx include: resolved path %s escapes base directory, skipping", resolvedMatch)
							continue
						}
						processConfigFile(m)
					}
				}
			}
		}
	}

	app.NginxDiscovery.SetApps(apps)

	log.Printf("Nginx discovery found %d apps", len(apps))
}
