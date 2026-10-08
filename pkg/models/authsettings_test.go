package models

import (
	"context"
	"testing"
)

func defaultChain() PluginSettings {
	return PluginSettings{AuthType: AuthProviderDefault, Region: "us-east-1"}
}

func accessKey() PluginSettings {
	return PluginSettings{AuthType: AuthProviderKeys, Region: "us-east-1"}
}

// Grafana shares its [aws] section only with the plugins named in
// forward_settings_to_plugins; everywhere else its defaults apply.
func TestAuthSettingsDefaultToGrafanasOwnDefaults(t *testing.T) {
	settings := ReadAuthSettings(context.Background())

	if !settings.AssumeRoleEnabled {
		t.Error("assume role should default to enabled, as it does in Grafana")
	}
	for _, permitted := range []PluginSettings{defaultChain(), accessKey()} {
		if err := settings.Permits(permitted); err != nil {
			t.Errorf("%s should be permitted by default: %v", permitted.AuthType, err)
		}
	}
}

func TestAuthSettingsHonourAllowedAuthProviders(t *testing.T) {
	t.Setenv(AllowedAuthProvidersKey, "keys, credentials")
	settings := ReadAuthSettings(context.Background())

	if err := settings.Permits(defaultChain()); err == nil {
		t.Error("expected the default chain to be refused when it is not an allowed provider")
	}
	if err := settings.Permits(accessKey()); err != nil {
		t.Errorf("the keys provider should stay permitted: %v", err)
	}
}

func TestAuthSettingsHonourAssumeRoleEnabled(t *testing.T) {
	t.Setenv(AssumeRoleEnabledKey, "false")
	settings := ReadAuthSettings(context.Background())

	assuming := accessKey()
	assuming.AssumeRoleARN = "arn:aws:iam::123456789012:role/GrafanaCostExplorer"
	if err := settings.Permits(assuming); err == nil {
		t.Error("expected a role hop to be refused when the server disabled it")
	}
	if err := settings.Permits(accessKey()); err != nil {
		t.Errorf("disabling assume role must not affect a data source that assumes nothing: %v", err)
	}
}

func TestAuthSettingsIgnoreAnEmptyProviderList(t *testing.T) {
	t.Setenv(AllowedAuthProvidersKey, " , ")
	if err := ReadAuthSettings(context.Background()).Permits(defaultChain()); err != nil {
		t.Errorf("an empty list must fall back to Grafana's defaults: %v", err)
	}
}
