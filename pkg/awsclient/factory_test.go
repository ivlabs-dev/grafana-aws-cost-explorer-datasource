package awsclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// recordingTransport records request hosts and fails every request, so tests
// prove which HTTP client AWS calls use without touching the network.
type recordingTransport struct {
	mu    sync.Mutex
	hosts []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.hosts = append(r.hosts, req.URL.Host)
	r.mu.Unlock()
	return nil, errors.New("recorded")
}

func (r *recordingTransport) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.hosts...)
}

// isolateAWSEnv keeps ambient AWS configuration on the machine running the
// tests (profiles, endpoint overrides, CA bundles) from changing results.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", empty)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", empty)
	for _, name := range []string{"AWS_PROFILE", "AWS_ENDPOINT_URL", "AWS_CA_BUNDLE"} {
		t.Setenv(name, "")
	}
}

func writeTestCABundle(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSDKFactoryToleratesAWSCABundleEnv(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_CA_BUNDLE", writeTestCABundle(t))
	rt := &recordingTransport{}
	bundle, err := NewFactory().New(context.Background(), staticSettings(models.AuthModeStatic), &http.Client{Transport: rt})
	if err != nil {
		t.Fatalf("New() with AWS_CA_BUNDLE set error = %v, want nil", err)
	}
	if bundle.CostExplorer == nil {
		t.Fatal("bundle has no Cost Explorer client")
	}
}

func staticSettings(authMode string) models.PluginSettings {
	return models.PluginSettings{
		AuthMode:        authMode,
		Region:          "us-east-1",
		CacheTTLSeconds: models.DefaultCacheTTLSeconds,
		CacheMaxEntries: models.DefaultCacheMaxEntries,
		Secrets: &models.SecretPluginSettings{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}
}

func TestSDKFactoryStaticUsesInjectedHTTPClient(t *testing.T) {
	isolateAWSEnv(t)
	rt := &recordingTransport{}
	bundle, err := NewFactory().New(context.Background(), staticSettings(models.AuthModeStatic), &http.Client{Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bundle.CostExplorer.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		TimePeriod:  &cetypes.DateInterval{Start: aws.String("2026-01-01"), End: aws.String("2026-02-01")},
		Granularity: cetypes.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
	}); err == nil {
		t.Fatal("expected the recording transport error")
	}
	if len(rt.recorded()) == 0 {
		t.Fatal("Cost Explorer request did not use the injected HTTP client")
	}
}

func TestSDKFactoryAssumeRoleUsesInjectedHTTPClient(t *testing.T) {
	isolateAWSEnv(t)
	rt := &recordingTransport{}
	settings := staticSettings(models.AuthModeAssumeRole)
	settings.RoleARN = "arn:aws:iam::123456789012:role/GrafanaCostExplorer"
	settings.RoleSessionName = "grafana"
	bundle, err := NewFactory().New(context.Background(), settings, &http.Client{Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.ResolveCredentials(context.Background()); err == nil {
		t.Fatal("expected the recording transport error")
	}
	hosts := rt.recorded()
	if len(hosts) == 0 || !strings.HasPrefix(hosts[0], "sts.") {
		t.Fatalf("STS request did not use the injected HTTP client, hosts = %v", hosts)
	}
}

func TestSDKFactoryRegionalClientsUseInjectedHTTPClient(t *testing.T) {
	isolateAWSEnv(t)
	rt := &recordingTransport{}
	bundle, err := NewFactory().New(context.Background(), staticSettings(models.AuthModeStatic), &http.Client{Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	clients := bundle.ReservationClientsForRegion("eu-west-1")
	if _, err := clients.EC2.DescribeReservedInstances(context.Background(), &ec2.DescribeReservedInstancesInput{}); err == nil {
		t.Fatal("expected the recording transport error")
	}
	hosts := rt.recorded()
	if len(hosts) == 0 || !strings.Contains(hosts[0], "eu-west-1") {
		t.Fatalf("regional request did not use the injected HTTP client, hosts = %v", hosts)
	}
}

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
	}, nil)
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
