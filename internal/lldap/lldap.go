package lldap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"dashgate/internal/models"
	"dashgate/internal/server"
)

// InitLLDAP reads LLDAP environment variables and initializes the LLDAP client
// configuration on app.LLDAPConfig. If the required variables are missing or
// the initial token refresh fails, LLDAP is left unconfigured.
func InitLLDAP(app *server.App) {
	url := os.Getenv("LLDAP_URL")
	username := os.Getenv("LLDAP_ADMIN_USERNAME")
	password := os.Getenv("LLDAP_ADMIN_PASSWORD")

	if url == "" || username == "" || password == "" {
		log.Println("LLDAP not configured (missing LLDAP_URL, LLDAP_ADMIN_USERNAME, or LLDAP_ADMIN_PASSWORD)")
		return
	}

	app.LLDAPConfig = &server.LLDAPConfigRef{
		URL:      strings.TrimSuffix(url, "/"),
		Username: username,
		Password: password,
	}

	if err := RefreshToken(app); err != nil {
		log.Printf("Warning: LLDAP connection failed: %v", err)
		app.LLDAPConfig = nil
		return
	}

	log.Printf("LLDAP connected successfully to %s", url)
}

// RefreshToken authenticates against the LLDAP simple login endpoint and
// stores the JWT token in app.LLDAPConfig.
func RefreshToken(app *server.App) error {
	l := app.LLDAPConfig

	loginPayload := map[string]string{
		"username": l.Username,
		"password": l.Password,
	}

	body, _ := json.Marshal(loginPayload)
	resp, err := app.HTTPClient.Post(l.URL+"/auth/simple/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		log.Printf("LLDAP login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&result); err != nil { // 10MB limit
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	l.TokenMu.Lock()
	l.Token = result.Token
	l.Expiry = time.Now().Add(55 * time.Minute)
	l.TokenMu.Unlock()

	return nil
}

// GetToken returns the cached LLDAP JWT token, refreshing it if expired.
// Uses a write lock for the entire check-and-refresh to prevent TOCTOU races.
func GetToken(app *server.App) (string, error) {
	l := app.LLDAPConfig

	l.TokenMu.Lock()
	if l.Token != "" && time.Now().Before(l.Expiry) {
		token := l.Token
		l.TokenMu.Unlock()
		return token, nil
	}
	l.TokenMu.Unlock()

	if err := RefreshToken(app); err != nil {
		return "", err
	}

	l.TokenMu.RLock()
	defer l.TokenMu.RUnlock()
	return l.Token, nil
}

// GraphQL sends a GraphQL query to the LLDAP API and returns the raw response body.
func GraphQL(app *server.App, query string, variables map[string]interface{}) ([]byte, error) {
	token, err := GetToken(app)
	if err != nil {
		return nil, err
	}

	l := app.LLDAPConfig

	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", l.URL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("GraphQL request failed with status %d: %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("GraphQL request failed with status %d", resp.StatusCode)
	}

	return respBody, nil
}

// ListUsers queries all users from LLDAP via GraphQL and returns them as models.LLDAPUser slices.
func ListUsers(app *server.App) ([]models.LLDAPUser, error) {
	query := `query { users { id email displayName groups { displayName } } }`

	respBody, err := GraphQL(app, query, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Users []struct {
				ID          string `json:"id"`
				Email       string `json:"email"`
				DisplayName string `json:"displayName"`
				Groups      []struct {
					DisplayName string `json:"displayName"`
				} `json:"groups"`
			} `json:"users"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	users := make([]models.LLDAPUser, len(result.Data.Users))
	for i, u := range result.Data.Users {
		groups := make([]string, len(u.Groups))
		for j, g := range u.Groups {
			groups[j] = g.DisplayName
		}
		users[i] = models.LLDAPUser{
			ID:          u.ID,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Groups:      groups,
		}
	}

	return users, nil
}

// ListGroups queries all groups from LLDAP via GraphQL and returns them as models.LLDAPGroup slices.
func ListGroups(app *server.App) ([]models.LLDAPGroup, error) {
	query := `query { groups { id displayName users { id } } }`

	respBody, err := GraphQL(app, query, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Groups []struct {
				ID          int    `json:"id"`
				DisplayName string `json:"displayName"`
				Users       []struct {
					ID string `json:"id"`
				} `json:"users"`
			} `json:"groups"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	groups := make([]models.LLDAPGroup, len(result.Data.Groups))
	for i, g := range result.Data.Groups {
		users := make([]string, len(g.Users))
		for j, u := range g.Users {
			users[j] = u.ID
		}
		groups[i] = models.LLDAPGroup{
			ID:          g.ID,
			DisplayName: g.DisplayName,
			Users:       users,
		}
	}

	return groups, nil
}
