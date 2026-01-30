package config

import (
	"log"
	"os"

	"dashgate/internal/models"
	"dashgate/internal/server"

	"gopkg.in/yaml.v3"
)

// LoadAppMappings reads the app-to-group mappings YAML file and populates app.AppMappings.
func LoadAppMappings(app *server.App) error {
	data, err := os.ReadFile(app.MappingsPath)
	if err != nil {
		log.Printf("No app mappings file found at %s, using config defaults", app.MappingsPath)
		return nil
	}

	var mappingsConfig models.AppMappingsConfig
	if err := yaml.Unmarshal(data, &mappingsConfig); err != nil {
		log.Printf("Error parsing app mappings: %v", err)
		return err
	}

	app.MappingsMu.Lock()
	for _, m := range mappingsConfig.Mappings {
		app.AppMappings[m.AppURL] = m.Groups
	}
	app.MappingsMu.Unlock()

	log.Printf("Loaded %d app mappings", len(mappingsConfig.Mappings))
	return nil
}

// SaveAppMappings marshals app.AppMappings and writes them to app.MappingsPath.
func SaveAppMappings(app *server.App) error {
	app.MappingsMu.RLock()
	mappings := make([]models.AppMapping, 0, len(app.AppMappings))
	for url, groups := range app.AppMappings {
		mappings = append(mappings, models.AppMapping{AppURL: url, Groups: groups})
	}
	app.MappingsMu.RUnlock()

	data, err := yaml.Marshal(models.AppMappingsConfig{Mappings: mappings})
	if err != nil {
		return err
	}

	return os.WriteFile(app.MappingsPath, data, 0600)
}

// GetAppGroups returns the groups allowed to access the given app.
// It first checks app.AppMappings for an override, falling back to the app's own Groups field.
func GetAppGroups(sApp *server.App, a models.App) []string {
	sApp.MappingsMu.RLock()
	if groups, ok := sApp.AppMappings[a.URL]; ok {
		sApp.MappingsMu.RUnlock()
		return groups
	}
	sApp.MappingsMu.RUnlock()
	return a.Groups
}
