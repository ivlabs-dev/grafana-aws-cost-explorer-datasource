package models

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/config"
)

// The keys Grafana shares its `[aws]` section under.
const (
	AllowedAuthProvidersKey = "AWS_AUTH_AllowedAuthProviders"
	AssumeRoleEnabledKey    = "AWS_AUTH_AssumeRoleEnabled"
)

// defaultAllowedAuthProviders mirrors Grafana's own default, which applies
// whenever Grafana shares nothing — notably when this plugin is absent from
// `[aws] forward_settings_to_plugins`.
var defaultAllowedAuthProviders = []string{AuthProviderDefault, AuthProviderKeys, "credentials"}

// AuthSettings is the part of Grafana's `[aws]` section that governs which
// authentication a data source may use, so an administrator restricts this
// plugin with the keys that restrict every AWS data source rather than with a
// setting peculiar to it.
type AuthSettings struct {
	AllowedAuthProviders []string
	AssumeRoleEnabled    bool
}

func ReadAuthSettings(ctx context.Context) AuthSettings {
	settings := AuthSettings{
		AllowedAuthProviders: defaultAllowedAuthProviders,
		AssumeRoleEnabled:    true,
	}

	if providers := splitProviders(grafanaSetting(ctx, AllowedAuthProvidersKey)); len(providers) > 0 {
		settings.AllowedAuthProviders = providers
	}
	if enabled, err := strconv.ParseBool(grafanaSetting(ctx, AssumeRoleEnabledKey)); err == nil {
		settings.AssumeRoleEnabled = enabled
	}

	return settings
}

// Permits reports whether the Grafana server allows what these data-source
// settings ask for.
func (a AuthSettings) Permits(settings PluginSettings) error {
	if !slices.Contains(a.AllowedAuthProviders, settings.AuthType) {
		return fmt.Errorf(
			"allowed_auth_providers in this Grafana server's [aws] section permits %s, not %q",
			strings.Join(a.AllowedAuthProviders, ", "),
			settings.AuthType,
		)
	}
	if settings.AssumesRole() && !a.AssumeRoleEnabled {
		return errors.New("this Grafana server has assume_role_enabled disabled in its [aws] section")
	}
	return nil
}

// grafanaSetting prefers the value Grafana passes through the plugin context
// and falls back to the process environment, which carries the same values.
func grafanaSetting(ctx context.Context, key string) string {
	if cfg := config.GrafanaConfigFromContext(ctx); cfg != nil {
		if value := cfg.Get(key); value != "" {
			return value
		}
	}
	return os.Getenv(key)
}

func splitProviders(value string) []string {
	providers := []string{}
	for _, provider := range strings.Split(value, ",") {
		if provider = strings.TrimSpace(provider); provider != "" {
			providers = append(providers, provider)
		}
	}
	return providers
}
