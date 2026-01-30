package config

import (
	"log"
	"os"
	"path/filepath"

	"dashgate/internal/server"

	"gopkg.in/yaml.v3"
)

// defaultConfig is a minimal config.yaml written when no config file exists.
const defaultConfig = `# DashGate Configuration
# Add your applications organized by category.
categories: []
#  - name: Services
#    apps:
#      - name: Example
#        url: https://example.com
#        icon: mdi:web
#        description: An example application
`

// LoadConfig reads the YAML configuration file and populates app.Config.
// If the file does not exist, a default config is created so the app can start
// cleanly for first-time users.
func LoadConfig(app *server.App, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Config file not found at %s — creating default config", path)
			if mkErr := os.MkdirAll(filepath.Dir(path), 0700); mkErr != nil {
				return mkErr
			}
			if wErr := os.WriteFile(path, []byte(defaultConfig), 0600); wErr != nil {
				return wErr
			}
			data = []byte(defaultConfig)
		} else {
			return err
		}
	}
	app.ConfigMu.Lock()
	defer app.ConfigMu.Unlock()
	return yaml.Unmarshal(data, &app.Config)
}

// SaveConfig marshals app.Config and writes it atomically to app.ConfigPath.
// It writes to a temporary file first, then renames it to the target path.
// This prevents corruption if the write is interrupted.
func SaveConfig(app *server.App) error {
	app.ConfigMu.RLock()
	data, err := yaml.Marshal(&app.Config)
	app.ConfigMu.RUnlock()
	if err != nil {
		return err
	}
	tmpPath := app.ConfigPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, app.ConfigPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// ReloadConfig re-reads the config file from disk into app.Config,
// restoring consistency after a failed save.
func ReloadConfig(app *server.App) {
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		return
	}
	app.ConfigMu.Lock()
	defer app.ConfigMu.Unlock()
	yaml.Unmarshal(data, &app.Config)
}
