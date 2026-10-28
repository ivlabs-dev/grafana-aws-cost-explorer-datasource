package awsclient

import (
	"context"
	"testing"

	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// TestSDKFactoryMockModeNeedsNoCredentials asserts that mock mode never
// reaches the AWS configuration/credential-loading code at all -- it
// returns an awsmock-backed Bundle before any of that runs, regardless of
// ambient AWS environment variables or the absence of Secrets.
func TestSDKFactoryMockModeNeedsNoCredentials(t *testing.T) {
	factory := NewFactory()
	bundle, err := factory.New(context.Background(), models.PluginSettings{
		AuthMode:        models.AuthModeMock,
		CacheTTLSeconds: models.DefaultCacheTTLSeconds,
		CacheMaxEntries: models.DefaultCacheMaxEntries,
		Secrets:         &models.SecretPluginSettings{},
	})
	if err != nil {
		t.Fatalf("SDKFactory.New() with mock auth mode error = %v", err)
	}
	if bundle.CostExplorer == nil || bundle.SavingsPlans == nil || bundle.EC2Reservations == nil ||
		bundle.RDSReservations == nil || bundle.ElastiCacheReservations == nil ||
		bundle.RedshiftReservations == nil || bundle.OpenSearchReservations == nil {
		t.Fatalf("mock Bundle is missing a client: %+v", bundle)
	}
	if bundle.ResolveCredentials == nil {
		t.Fatal("mock Bundle has no ResolveCredentials")
	}
	if err := bundle.ResolveCredentials(context.Background()); err != nil {
		t.Fatalf("mock ResolveCredentials() error = %v, want nil", err)
	}
}
