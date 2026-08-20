package costexplorer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

type mockClient struct {
	mu      sync.Mutex
	calls   int
	handler func(*awscostexplorer.GetCostAndUsageInput) (*awscostexplorer.GetCostAndUsageOutput, error)
}

func (m *mockClient) GetCostAndUsage(
	_ context.Context,
	input *awscostexplorer.GetCostAndUsageInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostAndUsageOutput, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.handler(input)
}

func TestServicePaginationAndCache(t *testing.T) {
	client := &mockClient{}
	client.handler = func(input *awscostexplorer.GetCostAndUsageInput) (*awscostexplorer.GetCostAndUsageOutput, error) {
		if input.NextPageToken == nil {
			return &awscostexplorer.GetCostAndUsageOutput{
				NextPageToken: aws.String("page-2"),
				ResultsByTime: []types.ResultByTime{result("2026-07-01", "1.00")},
			}, nil
		}
		if *input.NextPageToken != "page-2" {
			t.Fatalf("unexpected pagination token %q", *input.NextPageToken)
		}
		return &awscostexplorer.GetCostAndUsageOutput{
			ResultsByTime: []types.ResultByTime{result("2026-07-02", "2.00")},
		}, nil
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, err := NewService(client, resultCache)
	if err != nil {
		t.Fatal(err)
	}

	settings := validSettings()
	query := validQuery()
	dateRange := models.DateRange{Start: "2026-07-01", End: "2026-07-03"}

	first, err := service.Execute(context.Background(), settings, query, dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if first.Pages != 2 || first.CacheResult != cache.ResultMiss || len(first.Output.ResultsByTime) != 2 {
		t.Fatalf("unexpected first execution: %+v", first)
	}
	if first.CacheStatus() != cache.ResultMiss || first.CacheTTL != time.Minute {
		t.Fatalf("unexpected first cache metadata: %+v", first)
	}
	second, err := service.Execute(context.Background(), settings, query, dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if second.CacheResult != cache.ResultHit || second.CacheStatus() != cache.ResultHit {
		t.Fatalf("second cache result = %s (%s), want hit", second.CacheResult, second.CacheStatus())
	}
	if second.CacheAge < 0 || second.CacheTTL != time.Minute {
		t.Fatalf("unexpected second cache timing: age=%s ttl=%s", second.CacheAge, second.CacheTTL)
	}
	if second.AWSDuration != 0 {
		t.Fatalf("cache hit AWS duration = %s, want zero", second.AWSDuration)
	}
	if second.Pages != 0 {
		t.Fatalf("cache hit pages = %d, want zero pages fetched for this request", second.Pages)
	}
	if client.calls != 2 {
		t.Fatalf("AWS calls = %d, want two pages from one logical request", client.calls)
	}
}

func TestCacheKeyIgnoresPresentationOnlyOptions(t *testing.T) {
	query := validQuery()
	query.GroupBy = []string{"SERVICE"}
	query.ApplyDefaults()
	dateRange := models.DateRange{Start: "2026-07-01", End: "2026-07-03"}

	first, err := CacheKey("default:us-east-1", query, dateRange)
	if err != nil {
		t.Fatal(err)
	}
	query.Format = models.FormatTable
	query.TopN = 5
	query.IncludeOther = aws.Bool(false)
	second, err := CacheKey("default:us-east-1", query, dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("presentation-only options caused an unnecessary AWS cache miss")
	}

	query.Filter.Service = "Amazon S3"
	third, err := CacheKey("default:us-east-1", query, dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("an upstream AWS filter change did not change the cache key")
	}
}

func TestServiceReturnsClassifiedAWSError(t *testing.T) {
	client := &mockClient{handler: func(*awscostexplorer.GetCostAndUsageInput) (*awscostexplorer.GetCostAndUsageOutput, error) {
		return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
	}}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	execution, err := service.Execute(
		context.Background(),
		validSettings(),
		validQuery(),
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("expected actionable AWS error, got %v", err)
	}
	if got := err.Error(); got != "AWS denied the request; grant ce:GetCostAndUsage and verify the role trust policy" {
		t.Fatalf("error = %q", got)
	}
	if execution == nil {
		t.Fatal("error execution metadata is nil")
	}
	if execution.CacheStatus() != cache.ResultMiss || execution.Pages != 1 || execution.CacheTTL != time.Minute {
		t.Fatalf("unexpected error execution metadata: %+v", execution)
	}
}

func TestServiceDetectsEstimatedResultsAcrossPages(t *testing.T) {
	client := &mockClient{}
	client.handler = func(input *awscostexplorer.GetCostAndUsageInput) (*awscostexplorer.GetCostAndUsageOutput, error) {
		if input.NextPageToken == nil {
			return &awscostexplorer.GetCostAndUsageOutput{
				NextPageToken: aws.String("page-2"),
				ResultsByTime: []types.ResultByTime{result("2026-07-01", "1.00")},
			}, nil
		}
		estimated := result("2026-07-02", "2.00")
		estimated.Estimated = true
		return &awscostexplorer.GetCostAndUsageOutput{
			ResultsByTime: []types.ResultByTime{estimated},
		}, nil
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	execution, err := service.Execute(
		context.Background(),
		validSettings(),
		validQuery(),
		models.DateRange{Start: "2026-07-01", End: "2026-07-03"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !execution.ContainsEstimatedData || execution.Pages != 2 {
		t.Fatalf("unexpected execution metadata: %+v", execution)
	}

	cached, err := service.Execute(
		context.Background(),
		validSettings(),
		validQuery(),
		models.DateRange{Start: "2026-07-01", End: "2026-07-03"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !cached.ContainsEstimatedData || cached.CacheStatus() != cache.ResultHit {
		t.Fatalf("unexpected cached execution metadata: %+v", cached)
	}
}

func TestBuildInputMapsFiltersAndAvailabilityZone(t *testing.T) {
	query := validQuery()
	query.GroupBy = []string{"AVAILABILITY_ZONE"}
	query.Filter = models.QueryFilter{
		Service:       "Amazon Elastic Compute Cloud - Compute",
		LinkedAccount: "123456789012",
		Region:        "eu-west-1",
		TagKey:        "Environment",
		TagValue:      "production",
	}
	input, err := BuildInput(query, models.DateRange{Start: "2026-07-01", End: "2026-07-02"})
	if err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(input.GroupBy[0].Key); got != "AZ" {
		t.Fatalf("group key = %q, want AZ", got)
	}
	if input.Filter == nil || len(input.Filter.And) != 4 {
		t.Fatalf("filter = %+v, want four AND expressions", input.Filter)
	}
}

func validSettings() models.PluginSettings {
	return models.PluginSettings{
		AuthMode:        models.AuthModeStatic,
		Region:          "us-east-1",
		CacheTTLSeconds: models.DefaultCacheTTLSeconds,
		CacheMaxEntries: models.DefaultCacheMaxEntries,
		Secrets: &models.SecretPluginSettings{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}
}

func validQuery() models.Query {
	return models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		Format:      models.FormatTimeSeries,
	}
}

func result(start, amount string) types.ResultByTime {
	return types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String(start), End: aws.String(start)},
		Total: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String(amount), Unit: aws.String("USD")},
		},
	}
}
