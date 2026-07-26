package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

const (
	AuthModeDefault    = "default"
	AuthModeAssumeRole = "assumeRole"
	AuthModeStatic     = "static"

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
	AuthMode        string                `json:"authMode"`
	Region          string                `json:"region"`
	RoleARN         string                `json:"roleArn,omitempty"`
	RoleSessionName string                `json:"roleSessionName,omitempty"`
	CacheTTLSeconds int                   `json:"cacheTTLSeconds"`
	CacheMaxEntries int                   `json:"cacheMaxEntries"`
	Secrets         *SecretPluginSettings `json:"-"`
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
	if s.AuthMode == "" {
		s.AuthMode = AuthModeDefault
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
	switch s.AuthMode {
	case AuthModeDefault, AuthModeAssumeRole, AuthModeStatic:
	default:
		return fmt.Errorf("authentication mode %q is unsupported", s.AuthMode)
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

	if s.AuthMode == AuthModeAssumeRole {
		if !roleARNPattern.MatchString(s.RoleARN) {
			return fmt.Errorf("role ARN must be an IAM role ARN such as arn:aws:iam::123456789012:role/GrafanaCostExplorer")
		}
		if !sessionPattern.MatchString(s.RoleSessionName) {
			return fmt.Errorf("role session name must contain 2-64 letters, numbers, or +=,.@- characters")
		}
	}

	if s.AuthMode == AuthModeStatic {
		if s.Secrets == nil || strings.TrimSpace(s.Secrets.AccessKeyID) == "" {
			return fmt.Errorf("access key ID is required for static credentials")
		}
		if strings.TrimSpace(s.Secrets.SecretAccessKey) == "" {
			return fmt.Errorf("secret access key is required for static credentials")
		}
	}

	return nil
}

// CredentialContext contains safe, non-secret values that may be used as part
// of a cache-key digest.
func (s PluginSettings) CredentialContext() string {
	return strings.Join([]string{s.AuthMode, s.RoleARN, s.Region}, "\x00")
}

func loadSecretPluginSettings(source map[string]string) *SecretPluginSettings {
	return &SecretPluginSettings{
		ExternalID:      source["externalId"],
		AccessKeyID:     source["accessKeyId"],
		SecretAccessKey: source["secretAccessKey"],
		SessionToken:    source["sessionToken"],
	}
}
