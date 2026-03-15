// Package ldap provides LDAP/Active Directory synchronization for Outpost VPN.
//
// This package defines the configuration, types, and sync logic for connecting
// to an LDAP or Active Directory server, synchronizing users and groups into
// the local database, and performing LDAP bind authentication.
//
// For the actual LDAP protocol operations, plug in "github.com/go-ldap/ldap/v3"
// by implementing the Connector interface below. The current implementation uses
// the Connector abstraction so that a real LDAP library can be swapped in without
// changing the sync logic.
package ldap

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	ldaplib "github.com/go-ldap/ldap/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// Config holds LDAP/AD connection and sync settings.
type Config struct {
	URL          string            // ldap://dc.example.com:389 or ldaps://...
	BindDN       string            // e.g., cn=admin,dc=example,dc=com
	BindPassword string            // service account password
	BaseDN       string            // search base, e.g., dc=example,dc=com
	UserFilter   string            // e.g., (objectClass=person)
	GroupFilter  string            // e.g., (objectClass=group)
	UserAttrMap  UserAttributeMap  // maps LDAP attrs to user fields
	GroupAttrMap GroupAttributeMap  // maps LDAP attrs to group fields
	TLS          bool              // use STARTTLS
	SkipVerify   bool              // skip TLS certificate verification
	SyncInterval time.Duration     // how often to run automatic sync
}

// UserAttributeMap maps LDAP attributes to user database fields.
type UserAttributeMap struct {
	Username  string // default: sAMAccountName
	Email     string // default: mail
	FirstName string // default: givenName
	LastName  string // default: sn
	Phone     string // default: telephoneNumber
	DN        string // always "dn" (distinguished name)
}

// DefaultUserAttrMap returns the default Active Directory attribute mapping.
func DefaultUserAttrMap() UserAttributeMap {
	return UserAttributeMap{
		Username:  "sAMAccountName",
		Email:     "mail",
		FirstName: "givenName",
		LastName:  "sn",
		Phone:     "telephoneNumber",
		DN:        "dn",
	}
}

// GroupAttributeMap maps LDAP attributes to group database fields.
type GroupAttributeMap struct {
	Name    string // default: cn
	Members string // default: member
}

// DefaultGroupAttrMap returns the default Active Directory group attribute mapping.
func DefaultGroupAttrMap() GroupAttributeMap {
	return GroupAttributeMap{
		Name:    "cn",
		Members: "member",
	}
}

// LDAPUser represents a user entry retrieved from LDAP.
type LDAPUser struct {
	DN        string
	Username  string
	Email     string
	FirstName string
	LastName  string
	Phone     string
}

// LDAPGroup represents a group entry retrieved from LDAP.
type LDAPGroup struct {
	DN      string
	Name    string
	Members []string // member DNs
}

// SyncResult summarizes the outcome of a sync operation.
type SyncResult struct {
	Created  int           // number of new records created
	Updated  int           // number of existing records updated
	Disabled int           // number of records disabled (not found in LDAP)
	Errors   []string      // non-fatal errors encountered during sync
	Duration time.Duration // wall-clock time for the sync
}

// Entry represents a single LDAP directory entry returned from a search.
type Entry struct {
	DN         string
	Attributes map[string][]string
}

// GetAttributeValue returns the first value of the named attribute, or empty string.
func (e *Entry) GetAttributeValue(name string) string {
	vals := e.Attributes[name]
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// GetAttributeValues returns all values of the named attribute.
func (e *Entry) GetAttributeValues(name string) []string {
	return e.Attributes[name]
}

// Connector abstracts LDAP protocol operations so that a real library
// (go-ldap/ldap/v3) can be plugged in without changing sync logic.
type Connector interface {
	// Connect establishes a connection to the LDAP server.
	Connect(url string, tlsCfg *tls.Config) error

	// Bind authenticates with the LDAP server using the given DN and password.
	Bind(dn, password string) error

	// Search performs an LDAP search and returns matching entries.
	Search(baseDN, filter string, attributes []string) ([]*Entry, error)

	// Close terminates the LDAP connection.
	Close() error
}

// defaultPageSize is the number of entries per LDAP paged search request.
const defaultPageSize = 500

// LDAPConnector implements the Connector interface using github.com/go-ldap/ldap/v3.
// It supports both ldap:// and ldaps:// connections, STARTTLS, and paged search
// for large directories.
type LDAPConnector struct {
	conn     *ldaplib.Conn
	pageSize uint32
}

// NewLDAPConnector creates a new LDAPConnector with the given configuration.
// The returned connector is ready to be passed to Syncer.SetConnector or used
// directly as the default connector in NewSyncer.
func NewLDAPConnector() *LDAPConnector {
	return &LDAPConnector{
		pageSize: defaultPageSize,
	}
}

// SetPageSize overrides the default LDAP paged search size.
// A value of 0 disables paging.
func (c *LDAPConnector) SetPageSize(size uint32) {
	c.pageSize = size
}

// Connect establishes a connection to the LDAP server at the given URL.
// It supports ldap:// (plain or with optional STARTTLS) and ldaps:// (TLS from start).
// If tlsCfg is non-nil and the scheme is ldap://, STARTTLS is performed after connecting.
func (c *LDAPConnector) Connect(url string, tlsCfg *tls.Config) error {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}

	isLDAPS := strings.HasPrefix(strings.ToLower(url), "ldaps://")

	var conn *ldaplib.Conn
	var err error

	if isLDAPS {
		// Direct TLS connection for ldaps:// scheme.
		if tlsCfg == nil {
			tlsCfg = &tls.Config{} //nolint:gosec // server name will be inferred from URL
		}
		conn, err = ldaplib.DialURL(url, ldaplib.DialWithTLSConfig(tlsCfg))
	} else {
		// Plain connection for ldap:// scheme.
		conn, err = ldaplib.DialURL(url)
	}
	if err != nil {
		return fmt.Errorf("ldap dial %s: %w", url, err)
	}

	// Upgrade to TLS via STARTTLS if requested on a plain connection.
	if !isLDAPS && tlsCfg != nil {
		if err := conn.StartTLS(tlsCfg); err != nil {
			conn.Close()
			return fmt.Errorf("ldap STARTTLS: %w", err)
		}
	}

	c.conn = conn
	return nil
}

// Bind authenticates with the LDAP server using the given DN and password.
func (c *LDAPConnector) Bind(dn, password string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}
	if err := c.conn.Bind(dn, password); err != nil {
		return fmt.Errorf("ldap bind as %s: %w", dn, err)
	}
	return nil
}

// Search performs an LDAP search with the given base DN, filter, and requested
// attributes. It uses paged search controls to handle large result sets without
// exceeding server size limits. Results are returned as []*Entry for compatibility
// with the Connector interface.
func (c *LDAPConnector) Search(baseDN, filter string, attributes []string) ([]*Entry, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("ldap: not connected")
	}

	searchReq := ldaplib.NewSearchRequest(
		baseDN,
		ldaplib.ScopeWholeSubtree,
		ldaplib.NeverDerefAliases,
		0,    // no size limit (paging handles this)
		0,    // no time limit
		false, // typesOnly
		filter,
		attributes,
		nil, // controls added below if paging
	)

	var allEntries []*ldaplib.Entry

	if c.pageSize > 0 {
		// Use paged search for large directories.
		result, err := c.conn.SearchWithPaging(searchReq, c.pageSize)
		if err != nil {
			return nil, fmt.Errorf("ldap paged search: %w", err)
		}
		allEntries = result.Entries
	} else {
		// Single search without paging.
		result, err := c.conn.Search(searchReq)
		if err != nil {
			return nil, fmt.Errorf("ldap search: %w", err)
		}
		allEntries = result.Entries
	}

	// Convert go-ldap entries to our internal Entry type.
	entries := make([]*Entry, 0, len(allEntries))
	for _, le := range allEntries {
		e := &Entry{
			DN:         le.DN,
			Attributes: make(map[string][]string, len(le.Attributes)),
		}
		for _, attr := range le.Attributes {
			e.Attributes[attr.Name] = attr.Values
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// Close terminates the LDAP connection. It is safe to call multiple times.
func (c *LDAPConnector) Close() error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	return nil
}

// Syncer synchronizes users and groups from LDAP/AD into the local database.
type Syncer struct {
	cfg       Config
	pool      *pgxpool.Pool
	logger    *slog.Logger
	connector Connector
}

// NewSyncer creates a new LDAP syncer with the given configuration.
func NewSyncer(cfg Config, pool *pgxpool.Pool, logger *slog.Logger) *Syncer {
	if cfg.UserAttrMap == (UserAttributeMap{}) {
		cfg.UserAttrMap = DefaultUserAttrMap()
	}
	if cfg.GroupAttrMap == (GroupAttributeMap{}) {
		cfg.GroupAttrMap = DefaultGroupAttrMap()
	}
	if cfg.UserFilter == "" {
		cfg.UserFilter = "(objectClass=person)"
	}
	if cfg.GroupFilter == "" {
		cfg.GroupFilter = "(objectClass=group)"
	}
	return &Syncer{
		cfg:       cfg,
		pool:      pool,
		logger:    logger,
		connector: NewLDAPConnector(),
	}
}

// SetConnector sets the LDAP connector implementation. This must be called
// before any sync or auth operations. When using go-ldap/ldap/v3, create an
// adapter that implements the Connector interface and pass it here.
func (s *Syncer) SetConnector(c Connector) {
	s.connector = c
}

// tlsConfig builds a TLS configuration based on the syncer settings.
func (s *Syncer) tlsConfig() *tls.Config {
	if !s.TLSEnabled() {
		return nil
	}
	return &tls.Config{
		InsecureSkipVerify: s.cfg.SkipVerify, //nolint:gosec // configurable for dev environments
	}
}

// TLSEnabled reports whether TLS is configured.
func (s *Syncer) TLSEnabled() bool {
	return s.cfg.TLS
}

// connect establishes and authenticates an LDAP connection using the service account.
func (s *Syncer) connect() error {
	if s.connector == nil {
		return fmt.Errorf("ldap: no connector set — call SetConnector with a go-ldap/ldap/v3 adapter")
	}
	if err := s.connector.Connect(s.cfg.URL, s.tlsConfig()); err != nil {
		return fmt.Errorf("ldap: connecting to %s: %w", s.cfg.URL, err)
	}
	if err := s.connector.Bind(s.cfg.BindDN, s.cfg.BindPassword); err != nil {
		_ = s.connector.Close()
		return fmt.Errorf("ldap: bind as %s: %w", s.cfg.BindDN, err)
	}
	return nil
}

// TestConnection verifies that the LDAP server is reachable and the service
// account credentials are valid.
func (s *Syncer) TestConnection(ctx context.Context) error {
	_ = ctx // reserved for future context-aware connector implementations
	if err := s.connect(); err != nil {
		return err
	}
	defer s.connector.Close()
	s.logger.Info("ldap connection test successful", "url", s.cfg.URL, "bind_dn", s.cfg.BindDN)
	return nil
}

// userSearchAttrs returns the list of LDAP attributes to request when searching users.
func (s *Syncer) userSearchAttrs() []string {
	m := s.cfg.UserAttrMap
	attrs := []string{m.Username, m.Email, m.FirstName, m.LastName}
	if m.Phone != "" {
		attrs = append(attrs, m.Phone)
	}
	return attrs
}

// entryToUser converts an LDAP entry to an LDAPUser using the configured attribute map.
func (s *Syncer) entryToUser(e *Entry) LDAPUser {
	m := s.cfg.UserAttrMap
	return LDAPUser{
		DN:        e.DN,
		Username:  e.GetAttributeValue(m.Username),
		Email:     e.GetAttributeValue(m.Email),
		FirstName: e.GetAttributeValue(m.FirstName),
		LastName:  e.GetAttributeValue(m.LastName),
		Phone:     e.GetAttributeValue(m.Phone),
	}
}

// SyncUsers connects to LDAP, searches for user entries, and upserts them
// into the database. Users matched by ldap_dn are updated; new users are
// created; users present in the DB but absent from LDAP are deactivated.
func (s *Syncer) SyncUsers(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	if err := s.connect(); err != nil {
		return nil, err
	}
	defer s.connector.Close()

	// Search LDAP for users.
	filter := s.cfg.UserFilter
	entries, err := s.connector.Search(s.cfg.BaseDN, filter, s.userSearchAttrs())
	if err != nil {
		return nil, fmt.Errorf("ldap: searching users with filter %q: %w", filter, err)
	}

	s.logger.Info("ldap user search completed", "entries", len(entries), "filter", filter)

	// Track which DNs we saw so we can deactivate missing users.
	seenDNs := make(map[string]bool, len(entries))

	for _, entry := range entries {
		u := s.entryToUser(entry)
		if u.Username == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("skipping entry %s: empty username", u.DN))
			continue
		}
		seenDNs[u.DN] = true

		// Generate a random password hash for LDAP-sourced users (they auth via LDAP bind).
		var rb [16]byte
		_, _ = rand.Read(rb[:])
		passwordHash, err := auth.HashPassword("ldap-" + hex.EncodeToString(rb[:]))
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("hashing password for %s: %v", u.Username, err))
			continue
		}

		// Upsert: insert or update based on ldap_dn.
		var action string
		err = s.pool.QueryRow(ctx,
			`INSERT INTO users (username, email, password_hash, first_name, last_name, ldap_dn, is_active)
			 VALUES ($1, $2, $3, $4, $5, $6, true)
			 ON CONFLICT (ldap_dn) DO UPDATE SET
				username   = EXCLUDED.username,
				email      = EXCLUDED.email,
				first_name = EXCLUDED.first_name,
				last_name  = EXCLUDED.last_name,
				is_active  = true,
				updated_at = now()
			 RETURNING CASE WHEN xmax = 0 THEN 'created' ELSE 'updated' END`,
			u.Username, u.Email, passwordHash, u.FirstName, u.LastName, u.DN,
		).Scan(&action)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("upserting user %s (%s): %v", u.Username, u.DN, err))
			continue
		}

		switch action {
		case "created":
			result.Created++
			s.logger.Debug("ldap user created", "username", u.Username, "dn", u.DN)
		case "updated":
			result.Updated++
			s.logger.Debug("ldap user updated", "username", u.Username, "dn", u.DN)
		}
	}

	// Deactivate users that have an ldap_dn but were not found in this sync.
	if len(seenDNs) > 0 {
		dnList := make([]string, 0, len(seenDNs))
		for dn := range seenDNs {
			dnList = append(dnList, dn)
		}

		tag, err := s.pool.Exec(ctx,
			`UPDATE users
			 SET is_active = false, updated_at = now()
			 WHERE ldap_dn IS NOT NULL
			   AND ldap_dn != ''
			   AND ldap_dn != ALL($1)
			   AND is_active = true`,
			dnList,
		)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("deactivating removed ldap users: %v", err))
		} else {
			result.Disabled = int(tag.RowsAffected())
			if result.Disabled > 0 {
				s.logger.Info("ldap users deactivated", "count", result.Disabled)
			}
		}
	}

	result.Duration = time.Since(start)
	s.logger.Info("ldap user sync completed",
		"created", result.Created,
		"updated", result.Updated,
		"disabled", result.Disabled,
		"errors", len(result.Errors),
		"duration", result.Duration,
	)
	return result, nil
}

// SyncGroups connects to LDAP, searches for group entries, and syncs them
// into the database along with their memberships.
func (s *Syncer) SyncGroups(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{}

	if err := s.connect(); err != nil {
		return nil, err
	}
	defer s.connector.Close()

	filter := s.cfg.GroupFilter
	attrs := []string{s.cfg.GroupAttrMap.Name, s.cfg.GroupAttrMap.Members}
	entries, err := s.connector.Search(s.cfg.BaseDN, filter, attrs)
	if err != nil {
		return nil, fmt.Errorf("ldap: searching groups with filter %q: %w", filter, err)
	}

	s.logger.Info("ldap group search completed", "entries", len(entries), "filter", filter)

	for _, entry := range entries {
		groupName := entry.GetAttributeValue(s.cfg.GroupAttrMap.Name)
		if groupName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("skipping group entry %s: empty name", entry.DN))
			continue
		}

		memberDNs := entry.GetAttributeValues(s.cfg.GroupAttrMap.Members)

		// Upsert the group.
		var groupID string
		var action string
		err := s.pool.QueryRow(ctx,
			`INSERT INTO groups (name, ldap_dn)
			 VALUES ($1, $2)
			 ON CONFLICT (ldap_dn) DO UPDATE SET
				name = EXCLUDED.name,
				updated_at = now()
			 RETURNING id, CASE WHEN xmax = 0 THEN 'created' ELSE 'updated' END`,
			groupName, entry.DN,
		).Scan(&groupID, &action)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("upserting group %s: %v", groupName, err))
			continue
		}

		switch action {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		}

		// Sync group memberships: clear existing and re-add.
		_, err = s.pool.Exec(ctx,
			`DELETE FROM group_members WHERE group_id = $1`, groupID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("clearing members for group %s: %v", groupName, err))
			continue
		}

		for _, memberDN := range memberDNs {
			_, err := s.pool.Exec(ctx,
				`INSERT INTO group_members (group_id, user_id)
				 SELECT $1, id FROM users WHERE ldap_dn = $2
				 ON CONFLICT DO NOTHING`,
				groupID, memberDN,
			)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("adding member %s to group %s: %v", memberDN, groupName, err))
			}
		}
	}

	result.Duration = time.Since(start)
	s.logger.Info("ldap group sync completed",
		"created", result.Created,
		"updated", result.Updated,
		"errors", len(result.Errors),
		"duration", result.Duration,
	)
	return result, nil
}

// Authenticate performs an LDAP bind authentication for the given username.
// It first searches for the user's DN using the service account, then
// attempts to bind with the user's credentials.
func (s *Syncer) Authenticate(ctx context.Context, username, password string) (*LDAPUser, error) {
	_ = ctx // reserved for future context-aware connector implementations

	if err := s.connect(); err != nil {
		return nil, err
	}
	defer s.connector.Close()

	// Search for the user's DN.
	userFilter := fmt.Sprintf("(&%s(%s=%s))", s.cfg.UserFilter, s.cfg.UserAttrMap.Username, ldapEscapeFilter(username))
	entries, err := s.connector.Search(s.cfg.BaseDN, userFilter, s.userSearchAttrs())
	if err != nil {
		return nil, fmt.Errorf("ldap: searching for user %q: %w", username, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("ldap: user %q not found", username)
	}
	if len(entries) > 1 {
		return nil, fmt.Errorf("ldap: multiple entries found for user %q", username)
	}

	userEntry := entries[0]
	user := s.entryToUser(userEntry)

	// Re-bind as the user to verify their password.
	if err := s.connector.Bind(userEntry.DN, password); err != nil {
		return nil, fmt.Errorf("ldap: authentication failed for user %q: %w", username, err)
	}

	s.logger.Info("ldap authentication successful", "username", username, "dn", userEntry.DN)
	return &user, nil
}

// ldapEscapeFilter escapes special characters in an LDAP search filter value
// per RFC 4515 section 3.
func ldapEscapeFilter(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '*', '(', ')', '\x00':
			result = append(result, '\\')
			result = append(result, fmt.Sprintf("%02x", c)...)
		default:
			result = append(result, c)
		}
	}
	return string(result)
}
