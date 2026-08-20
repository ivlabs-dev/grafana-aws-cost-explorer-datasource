package models

import "testing"

func TestPluginSettingsAssumeRoleValidation(t *testing.T) {
	settings := PluginSettings{
		AuthMode:        AuthModeAssumeRole,
		Region:          "us-east-1",
		RoleARN:         "not-an-arn",
		RoleSessionName: "grafana-cost-explorer",
		CacheTTLSeconds: DefaultCacheTTLSeconds,
		CacheMaxEntries: DefaultCacheMaxEntries,
		Secrets: &SecretPluginSettings{
			AccessKeyID:     "source-access-key",
			SecretAccessKey: "source-secret",
		},
	}
	if err := settings.Validate(); err == nil {
		t.Fatal("expected malformed role ARN to fail validation")
	}

	settings.RoleARN = "arn:aws:iam::123456789012:role/GrafanaCostExplorer"
	if err := settings.Validate(); err != nil {
		t.Fatalf("valid AssumeRole settings failed validation: %v", err)
	}
}

func TestPluginSettingsAssumeRoleRequiresExplicitSourceCredentials(t *testing.T) {
	settings := PluginSettings{
		AuthMode:        AuthModeAssumeRole,
		Region:          "us-east-1",
		RoleARN:         "arn:aws:iam::123456789012:role/GrafanaCostExplorer",
		RoleSessionName: "grafana-cost-explorer",
		CacheTTLSeconds: DefaultCacheTTLSeconds,
		CacheMaxEntries: DefaultCacheMaxEntries,
		Secrets:         &SecretPluginSettings{},
	}
	if err := settings.Validate(); err == nil {
		t.Fatal("expected missing AssumeRole source credentials to fail validation")
	}
}

func TestPluginSettingsStaticCredentialsValidation(t *testing.T) {
	settings := PluginSettings{
		AuthMode:        AuthModeStatic,
		Region:          "eu-west-1",
		CacheTTLSeconds: DefaultCacheTTLSeconds,
		CacheMaxEntries: DefaultCacheMaxEntries,
		Secrets:         &SecretPluginSettings{},
	}
	if err := settings.Validate(); err == nil {
		t.Fatal("expected missing static credentials to fail validation")
	}

	settings.Secrets.AccessKeyID = "test-access-key"
	settings.Secrets.SecretAccessKey = "test-secret"
	if err := settings.Validate(); err != nil {
		t.Fatalf("valid static settings failed validation: %v", err)
	}
}

func TestPluginSettingsRejectsDefaultCredentialChain(t *testing.T) {
	settings := PluginSettings{
		AuthMode:        "default",
		Region:          "us-east-1",
		CacheTTLSeconds: DefaultCacheTTLSeconds,
		CacheMaxEntries: DefaultCacheMaxEntries,
		Secrets: &SecretPluginSettings{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}
	if err := settings.Validate(); err == nil {
		t.Fatal("expected the default credential chain to be unsupported")
	}
}
