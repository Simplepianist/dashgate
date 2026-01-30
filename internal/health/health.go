package health

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"dashgate/internal/server"
)

// CheckHealth performs a HEAD request against the given URL and returns "online" or "offline".
// This is the ONLY place where InsecureClient (TLS skip verify) is used, because health
// checks need to reach services with self-signed certificates.
func CheckHealth(app *server.App, url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return "offline"
	}

	resp, err := app.InsecureClient.Do(req)
	if err != nil {
		return "offline"
	}
	defer resp.Body.Close()

	// 2xx and 3xx indicate the service is reachable and responding.
	// 401/403 also count as "online" since the service is running but requires auth.
	// Other 4xx/5xx are treated as offline.
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "online"
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "online"
	}
	return "offline"
}

// StartHealthChecker starts a background goroutine that runs health checks every 30 seconds.
// The goroutine stops when the provided context is cancelled.
func StartHealthChecker(app *server.App, ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Health checker recovered from panic: %v", r)
			}
		}()
		RunHealthChecks(app)
		for {
			select {
			case <-ctx.Done():
				log.Println("Health checker stopped")
				return
			case <-ticker.C:
				RunHealthChecks(app)
			}
		}
	}()
}

// RunHealthChecks concurrently checks the health of all configured app URLs
// and updates the app.HealthCache.
func RunHealthChecks(app *server.App) {
	var wg sync.WaitGroup
	results := make(chan struct {
		url    string
		status string
	}, 100)

	app.ConfigMu.RLock()
	urls := make(map[string]bool)
	for _, cat := range app.Config.Categories {
		for _, a := range cat.Apps {
			urls[a.URL] = true
		}
	}
	app.ConfigMu.RUnlock()

	sem := make(chan struct{}, 20)

	for url := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			status := CheckHealth(app, u)
			results <- struct {
				url    string
				status string
			}{u, status}
		}(url)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	newCache := make(map[string]string)
	for result := range results {
		newCache[result.url] = result.status
	}

	app.HealthMu.Lock()
	app.HealthCache = newCache
	app.HealthMu.Unlock()

	log.Printf("Health check complete: %d services checked", len(newCache))
}

// GetHealthStatus returns the cached health status for the given URL.
// Returns "unknown" if no status has been recorded yet.
func GetHealthStatus(app *server.App, url string) string {
	app.HealthMu.RLock()
	defer app.HealthMu.RUnlock()
	if status, ok := app.HealthCache[url]; ok {
		return status
	}
	return "unknown"
}
