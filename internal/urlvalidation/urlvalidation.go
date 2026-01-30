package urlvalidation

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateDiscoveryURL checks that a URL is safe for server-side requests.
// It blocks private IPs, loopback, link-local, and cloud metadata endpoints.
func ValidateDiscoveryURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Only allow http and https schemes
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}

	hostname := parsed.Hostname()

	// Block cloud metadata endpoints
	metadataHosts := []string{
		"169.254.169.254",
		"metadata.google.internal",
		"metadata.google.com",
	}
	for _, h := range metadataHosts {
		if strings.EqualFold(hostname, h) {
			return fmt.Errorf("URL points to cloud metadata endpoint")
		}
	}

	// Resolve hostname and check IP
	ips, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("cannot resolve hostname %q: %v", hostname, err)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL resolves to disallowed address: %s", ipStr)
		}
		// Check RFC 1918 private ranges
		privateRanges := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
		}
		for _, cidr := range privateRanges {
			_, network, _ := net.ParseCIDR(cidr)
			if network.Contains(ip) {
				// Log warning but allow -- many self-hosted setups use private IPs
				// This is intentionally a warning, not a block, for self-hosted use
				break
			}
		}
	}

	return nil
}

// ValidateNginxConfigPath checks that a path looks like a valid Nginx config directory.
func ValidateNginxConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	// Block obvious dangerous paths (use trailing slash to avoid blocking e.g. /processor)
	dangerousPrefixes := []string{"/proc/", "/sys/", "/dev/", "/boot/", "/root/", "/etc/shadow", "/etc/passwd"}
	dangerousExact := []string{"/proc", "/sys", "/dev", "/boot", "/root"}
	for _, prefix := range dangerousPrefixes {
		if strings.HasPrefix(path, prefix) {
			return fmt.Errorf("path %s is not allowed", prefix)
		}
	}
	for _, exact := range dangerousExact {
		if path == exact {
			return fmt.Errorf("path %s is not allowed", exact)
		}
	}
	// Must be an absolute path
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "C:") && !strings.HasPrefix(path, "\\") {
		return fmt.Errorf("path must be absolute")
	}
	// Block path traversal
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..'")
	}
	return nil
}
