package auth

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"dashgate/internal/models"
	"dashgate/internal/server"

	"github.com/go-ldap/ldap/v3"
)

// AuthenticateLDAP performs LDAP bind authentication for the given username and
// password. It searches for the user using a service account, extracts group
// membership, then verifies the password by binding as the user.
func AuthenticateLDAP(app *server.App, username, password string) (*models.AuthenticatedUser, error) {
	if password == "" {
		return nil, fmt.Errorf("invalid credentials")
	}

	if app.LDAPAuth == nil {
		return nil, fmt.Errorf("LDAP not configured")
	}

	// Connect to LDAP server
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	l, err := ldap.DialURL(app.LDAPAuth.Server, ldap.DialWithDialer(dialer))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP: %w", err)
	}
	defer l.Close()

	l.SetTimeout(10 * time.Second)

	// StartTLS if configured
	if app.LDAPAuth.StartTLS {
		tlsConfig := &tls.Config{InsecureSkipVerify: app.LDAPAuth.SkipVerify}
		if app.LDAPAuth.SkipVerify {
			log.Printf("WARNING: LDAP TLS verification is disabled (SkipVerify=true). This allows man-in-the-middle attacks.")
		}
		if err := l.StartTLS(tlsConfig); err != nil {
			return nil, fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	// Bind with service account to search for user
	if app.LDAPAuth.BindDN != "" {
		if err := l.Bind(app.LDAPAuth.BindDN, app.LDAPAuth.BindPassword); err != nil {
			return nil, fmt.Errorf("service account bind failed: %w", err)
		}
	}

	// Search for user
	userFilter := strings.Replace(app.LDAPAuth.UserFilter, "%s", ldap.EscapeFilter(username), -1)
	searchRequest := ldap.NewSearchRequest(
		app.LDAPAuth.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		userFilter,
		[]string{"dn", app.LDAPAuth.UserAttr, app.LDAPAuth.EmailAttr, app.LDAPAuth.DisplayAttr, app.LDAPAuth.GroupAttr},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("user search failed: %w", err)
	}

	if len(sr.Entries) != 1 {
		return nil, fmt.Errorf("user not found or multiple matches")
	}

	userDN := sr.Entries[0].DN
	email := sr.Entries[0].GetAttributeValue(app.LDAPAuth.EmailAttr)
	displayName := sr.Entries[0].GetAttributeValue(app.LDAPAuth.DisplayAttr)
	if displayName == "" {
		displayName = username
	}

	// Get groups from user entry
	groups := sr.Entries[0].GetAttributeValues(app.LDAPAuth.GroupAttr)
	// Extract CN from group DNs
	var groupNames []string
	for _, g := range groups {
		if strings.HasPrefix(strings.ToLower(g), "cn=") {
			parts := strings.Split(g, ",")
			if len(parts) > 0 {
				cn := strings.TrimPrefix(strings.ToLower(parts[0]), "cn=")
				groupNames = append(groupNames, cn)
			}
		} else {
			groupNames = append(groupNames, g)
		}
	}

	// Bind as user to verify password
	if err := l.Bind(userDN, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	user := &models.AuthenticatedUser{
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Groups:      groupNames,
		Source:      "ldap",
	}
	user.IsAdmin = CheckIsAdmin(app, user.Groups)
	return user, nil
}
