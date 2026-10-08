package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// Grafana's AWS authentication provider identifiers, as written in the
// `allowed_auth_providers` key of its `[aws]` configuration section and stored
// as a data source's AuthType.
const (
	// AuthProviderDefault is the AWS SDK default credential chain.
	AuthProviderDefault = "default"
	// AuthProviderKeys is an access key and secret access key.
	AuthProviderKeys = "keys"
)

const (
	// Superseded by AuthType; see adoptLegacyAuthMode.
	legacyAuthModeAssumeRole = "assumeRole"
	legacyAuthModeStatic     = "static"

	DefaultRegion          = "us-east-1"
	DefaultCacheTTLSeconds = 15 * 60
	DefaultCacheMaxEntries = 256
	MaxCacheTTLSeconds     = 24 * 60 * 60
	MaxCacheEntries        = 10_000
)

var (
	regionPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	roleARNPattern = regexp.MustCompile(`^arn:(aws|aws-us-gov|aws-cn|aws-iso|aws-iso-b):iam::[0-9]{12}:role/[A-Za-z0-9+=,.@_/-]{1,512}$`)
	sessionPattern = regexp.MustCompile(`^[\w+=,.@-]{2,64}$`)
)

// PluginSettings contains only non-secret data that Grafana may return to the
// browser. Secret values are loaded separately from DecryptedSecureJSONData.
type PluginSettings struct {
	// AuthType is one of the AuthProvider constants, named as
	// `[aws] allowed_auth_providers` names them.
	AuthType string `json:"authType"`
	Region   string `json:"region"`
	// Optional and independent of AuthType: when set, the credentials AuthType
	// resolves assume this role rather than call Cost Explorer directly.
	AssumeRoleARN   string `json:"assumeRoleArn,omitempty"`
	RoleSessionName string `json:"roleSessionName,omitempty"`
	CacheTTLSeconds int    `json:"cacheTTLSeconds"`
	CacheMaxEntries int    `json:"cacheMaxEntries"`

	// Read only by adoptLegacyAuthMode, which folds them into the fields above.
	LegacyAuthMode string `json:"authMode,omitempty"`
	LegacyRoleARN  string `json:"roleArn,omitempty"`

	Secrets *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings is never serialized into jsonData or sent back to the
// browser after Grafana stores the data-source configuration.
type SecretPluginSettings struct {
	ExternalID      string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := PluginSettings{}
	if len(source.JSONData) > 0 {
		if err := json.Unmarshal(source.JSONData, &settings); err != nil {
			return nil, fmt.Errorf("could not decode data-source settings: %w", err)
		}
	}

	settings.ApplyDefaults()
	settings.Secrets = loadSecretPluginSettings(source.DecryptedSecureJSONData)

	return &settings, nil
}

func (s *PluginSettings) ApplyDefaults() {
	s.adoptLegacyAuthMode()

	if s.AuthType == "" {
		s.AuthType = AuthProviderKeys
	}
	if s.Region == "" {
		s.Region = DefaultRegion
	}
	if s.CacheTTLSeconds == 0 {
		s.CacheTTLSeconds = DefaultCacheTTLSeconds
	}
	if s.CacheMaxEntries == 0 {
		s.CacheMaxEntries = DefaultCacheMaxEntries
	}
	if s.RoleSessionName == "" {
		s.RoleSessionName = "grafana-cost-explorer"
	}
	if s.Secrets == nil {
		s.Secrets = &SecretPluginSettings{}
	}
}

func (s PluginSettings) Validate() error {
	switch s.AuthType {
	case AuthProviderDefault, AuthProviderKeys:
	default:
		return fmt.Errorf("authentication provider %q is unsupported", s.AuthType)
	}

	if !regionPattern.MatchString(s.Region) || !strings.Contains(s.Region, "-") {
		return fmt.Errorf("AWS region %q is malformed", s.Region)
	}
	if s.CacheTTLSeconds < 1 || s.CacheTTLSeconds > MaxCacheTTLSeconds {
		return fmt.Errorf("cache TTL must be between 1 and %d seconds", MaxCacheTTLSeconds)
	}
	if s.CacheMaxEntries < 1 || s.CacheMaxEntries > MaxCacheEntries {
		return fmt.Errorf("cache maximum entries must be between 1 and %d", MaxCacheEntries)
	}

	if s.AssumesRole() {
		if !roleARNPattern.MatchString(s.AssumeRoleARN) {
			return fmt.Errorf("role ARN must be an IAM role ARN such as arn:aws:iam::123456789012:role/GrafanaCostExplorer")
		}
		if !sessionPattern.MatchString(s.RoleSessionName) {
			return fmt.Errorf("role session name must contain 2-64 letters, numbers, or +=,.@- characters")
		}
	}

	if s.AuthType == AuthProviderKeys {
		if s.Secrets == nil || strings.TrimSpace(s.Secrets.AccessKeyID) == "" {
			return fmt.Errorf("access key ID is required for access key authentication")
		}
		if strings.TrimSpace(s.Secrets.SecretAccessKey) == "" {
			return fmt.Errorf("secret access key is required for access key authentication")
		}
	}

	return nil
}

// AssumesRole reports whether a role hop is configured on top of AuthType.
func (s PluginSettings) AssumesRole() bool {
	return strings.TrimSpace(s.AssumeRoleARN) != ""
}

// adoptLegacyAuthMode translates the two superseded authMode values, which both
// authenticated with a stored access key; "assumeRole" also carried the role it
// hopped to. Any other value is left untranslated, so it must be reconfigured
// rather than silently reinterpreted.
func (s *PluginSettings) adoptLegacyAuthMode() {
	if s.AuthType != "" {
		return
	}
	switch s.LegacyAuthMode {
	case legacyAuthModeStatic, legacyAuthModeAssumeRole:
		s.AuthType = AuthProviderKeys
	default:
		return
	}
	if s.LegacyAuthMode == legacyAuthModeAssumeRole && s.AssumeRoleARN == "" {
		s.AssumeRoleARN = s.LegacyRoleARN
	}
}

// CredentialContext contains safe, non-secret values that may be used as part
// of a cache-key digest.
func (s PluginSettings) CredentialContext() string {
	return strings.Join([]string{s.AuthType, s.AssumeRoleARN, s.Region}, "\x00")
}

func loadSecretPluginSettings(source map[string]string) *SecretPluginSettings {
	return &SecretPluginSettings{
		ExternalID:      source["externalId"],
		AccessKeyID:     source["accessKeyId"],
		SecretAccessKey: source["secretAccessKey"],
		SessionToken:    source["sessionToken"],
	}
}
