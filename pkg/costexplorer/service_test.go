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

	dimensionValuesCalls int
	dimensionValuesFn    func(*awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error)

	tagsCalls int
	tagsFn    func(*awscostexplorer.GetTagsInput) (*awscostexplorer.GetTagsOutput, error)

	reservationUtilizationCalls int
	reservationUtilizationFn    func(*awscostexplorer.GetReservationUtilizationInput) (*awscostexplorer.GetReservationUtilizationOutput, error)

	savingsPlansUtilizationCalls int
	savingsPlansUtilizationFn    func(*awscostexplorer.GetSavingsPlansUtilizationInput) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error)
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

func (m *mockClient) GetDimensionValues(
	_ context.Context,
	input *awscostexplorer.GetDimensionValuesInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetDimensionValuesOutput, error) {
	m.mu.Lock()
	m.dimensionValuesCalls++
	m.mu.Unlock()
	if m.dimensionValuesFn == nil {
		return &awscostexplorer.GetDimensionValuesOutput{}, nil
	}
	return m.dimensionValuesFn(input)
}

func (m *mockClient) GetTags(
	_ context.Context,
	input *awscostexplorer.GetTagsInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetTagsOutput, error) {
	m.mu.Lock()
	m.tagsCalls++
	m.mu.Unlock()
	if m.tagsFn == nil {
		return &awscostexplorer.GetTagsOutput{}, nil
	}
	return m.tagsFn(input)
}

func (m *mockClient) GetReservationUtilization(
	_ context.Context,
	input *awscostexplorer.GetReservationUtilizationInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetReservationUtilizationOutput, error) {
	m.mu.Lock()
	m.reservationUtilizationCalls++
	m.mu.Unlock()
	if m.reservationUtilizationFn == nil {
		return &awscostexplorer.GetReservationUtilizationOutput{}, nil
	}
	return m.reservationUtilizationFn(input)
}

func (m *mockClient) GetSavingsPlansUtilization(
	_ context.Context,
	input *awscostexplorer.GetSavingsPlansUtilizationInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
	m.mu.Lock()
	m.savingsPlansUtilizationCalls++
	m.mu.Unlock()
	if m.savingsPlansUtilizationFn == nil {
		return &awscostexplorer.GetSavingsPlansUtilizationOutput{}, nil
	}
	return m.savingsPlansUtilizationFn(input)
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

func TestDimensionValuesReturnsSortedValues(t *testing.T) {
	client := &mockClient{
		dimensionValuesFn: func(input *awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error) {
			if input.Dimension != types.DimensionService {
				t.Fatalf("dimension = %q, want SERVICE", input.Dimension)
			}
			if input.SearchString == nil || *input.SearchString != "ec2" {
				t.Fatalf("search string = %v, want ec2", input.SearchString)
			}
			return &awscostexplorer.GetDimensionValuesOutput{
				DimensionValues: []types.DimensionValuesWithAttributes{
					{Value: aws.String("Amazon EC2 Container Registry (ECR)")},
					{Value: aws.String("Amazon Elastic Compute Cloud - Compute")},
				},
			}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	values, err := service.DimensionValues(
		context.Background(),
		validSettings(),
		"SERVICE",
		"ec2",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Amazon EC2 Container Registry (ECR)", "Amazon Elastic Compute Cloud - Compute"}
	if len(values) != len(want) || values[0] != want[0] || values[1] != want[1] {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

func TestDimensionValuesRejectsUnsupportedDimension(t *testing.T) {
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(&mockClient{}, resultCache)

	_, err := service.DimensionValues(
		context.Background(),
		validSettings(),
		"USAGE_TYPE",
		"",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err == nil {
		t.Fatal("expected an error for an unsupported dimension")
	}
}

func TestDimensionValuesOmitsBlankValues(t *testing.T) {
	// AWS returns an empty-string REGION value for charges that aren't
	// attributed to a specific region (e.g. taxes); it's not a selectable
	// filter value, so this plugin drops it rather than suggesting a blank
	// option.
	client := &mockClient{
		dimensionValuesFn: func(*awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error) {
			return &awscostexplorer.GetDimensionValuesOutput{
				DimensionValues: []types.DimensionValuesWithAttributes{
					{Value: aws.String("")},
					{Value: aws.String("us-east-1")},
				},
			}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	values, err := service.DimensionValues(
		context.Background(),
		validSettings(),
		"REGION",
		"",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0] != "us-east-1" {
		t.Fatalf("values = %v, want only us-east-1", values)
	}
}

func TestDimensionValuesCachesResult(t *testing.T) {
	client := &mockClient{
		dimensionValuesFn: func(*awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error) {
			return &awscostexplorer.GetDimensionValuesOutput{
				DimensionValues: []types.DimensionValuesWithAttributes{{Value: aws.String("eu-west-1")}},
			}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)
	dateRange := models.DateRange{Start: "2026-07-01", End: "2026-07-02"}

	if _, err := service.DimensionValues(context.Background(), validSettings(), "REGION", "", dateRange); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DimensionValues(context.Background(), validSettings(), "REGION", "", dateRange); err != nil {
		t.Fatal(err)
	}
	if client.dimensionValuesCalls != 1 {
		t.Fatalf("AWS calls = %d, want one call reused from cache", client.dimensionValuesCalls)
	}
}

func TestDimensionValuesClassifiesAWSError(t *testing.T) {
	client := &mockClient{
		dimensionValuesFn: func(*awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	_, err := service.DimensionValues(
		context.Background(),
		validSettings(),
		"SERVICE",
		"",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err == nil {
		t.Fatal("expected a classified AWS error")
	}
	if got := err.Error(); got != "AWS denied the request; grant ce:GetCostAndUsage and verify the role trust policy" {
		t.Fatalf("error = %q", got)
	}
}

func TestTagKeysReturnsSortedValues(t *testing.T) {
	client := &mockClient{
		tagsFn: func(input *awscostexplorer.GetTagsInput) (*awscostexplorer.GetTagsOutput, error) {
			if input.TagKey != nil {
				t.Fatalf("tag key = %v, want nil for a tag-key lookup", input.TagKey)
			}
			return &awscostexplorer.GetTagsOutput{Tags: []string{"Team", "Environment"}}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	values, err := service.TagKeys(
		context.Background(),
		validSettings(),
		"",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Environment", "Team"}
	if len(values) != len(want) || values[0] != want[0] || values[1] != want[1] {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

func TestTagValuesRequiresKey(t *testing.T) {
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(&mockClient{}, resultCache)

	_, err := service.TagValues(
		context.Background(),
		validSettings(),
		"",
		"",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err == nil {
		t.Fatal("expected an error when the tag key is missing")
	}
}

func TestTagValuesReturnsSortedValues(t *testing.T) {
	client := &mockClient{
		tagsFn: func(input *awscostexplorer.GetTagsInput) (*awscostexplorer.GetTagsOutput, error) {
			if input.TagKey == nil || *input.TagKey != "Environment" {
				t.Fatalf("tag key = %v, want Environment", input.TagKey)
			}
			return &awscostexplorer.GetTagsOutput{Tags: []string{"staging", "production"}}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	values, err := service.TagValues(
		context.Background(),
		validSettings(),
		"Environment",
		"",
		models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"production", "staging"}
	if len(values) != len(want) || values[0] != want[0] || values[1] != want[1] {
		t.Fatalf("values = %v, want %v", values, want)
	}
}

func reservationUtilizationQuery() models.Query {
	q := models.Query{
		Version:     models.QueryVersion,
		QueryType:   models.QueryTypeReservationUtilization,
		Granularity: models.GranularityDaily,
		Format:      models.FormatTimeSeries,
	}
	q.ApplyDefaults()
	return q
}

func savingsPlansUtilizationQuery() models.Query {
	q := models.Query{
		Version:     models.QueryVersion,
		QueryType:   models.QueryTypeSavingsPlansUtilization,
		Granularity: models.GranularityDaily,
		Format:      models.FormatTimeSeries,
	}
	q.ApplyDefaults()
	return q
}

func TestReservationUtilizationPaginatesAndCaches(t *testing.T) {
	client := &mockClient{
		reservationUtilizationFn: func(
			input *awscostexplorer.GetReservationUtilizationInput,
		) (*awscostexplorer.GetReservationUtilizationOutput, error) {
			if input.NextPageToken == nil {
				return &awscostexplorer.GetReservationUtilizationOutput{
					NextPageToken: aws.String("page-2"),
					UtilizationsByTime: []types.UtilizationByTime{{
						TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
						Total:      &types.ReservationAggregates{UtilizationPercentage: aws.String("80.0")},
					}},
				}, nil
			}
			return &awscostexplorer.GetReservationUtilizationOutput{
				UtilizationsByTime: []types.UtilizationByTime{{
					TimePeriod: &types.DateInterval{Start: aws.String("2026-07-02")},
					Total:      &types.ReservationAggregates{UtilizationPercentage: aws.String("90.0")},
				}},
			}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)
	dateRange := models.DateRange{Start: "2026-07-01", End: "2026-07-03"}

	first, err := service.ReservationUtilization(context.Background(), validSettings(), reservationUtilizationQuery(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if first.Pages != 2 || first.CacheResult != cache.ResultMiss || len(first.Output.UtilizationsByTime) != 2 {
		t.Fatalf("unexpected first execution: %+v", first)
	}

	second, err := service.ReservationUtilization(context.Background(), validSettings(), reservationUtilizationQuery(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if second.CacheResult != cache.ResultHit {
		t.Fatalf("second cache result = %s, want hit", second.CacheResult)
	}
	if client.reservationUtilizationCalls != 2 {
		t.Fatalf("AWS calls = %d, want two pages from one logical request", client.reservationUtilizationCalls)
	}
}

func TestReservationUtilizationGroupsBySubscription(t *testing.T) {
	client := &mockClient{
		reservationUtilizationFn: func(
			input *awscostexplorer.GetReservationUtilizationInput,
		) (*awscostexplorer.GetReservationUtilizationOutput, error) {
			if len(input.GroupBy) != 1 || string(*input.GroupBy[0].Key) != "SUBSCRIPTION_ID" {
				t.Fatalf("groupBy = %+v, want SUBSCRIPTION_ID", input.GroupBy)
			}
			if input.Granularity != "" {
				t.Fatalf("granularity = %q, want empty when grouped", input.Granularity)
			}
			return &awscostexplorer.GetReservationUtilizationOutput{
				UtilizationsByTime: []types.UtilizationByTime{{
					TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
					Groups: []types.ReservationUtilizationGroup{
						{Value: aws.String("sub-1"), Utilization: &types.ReservationAggregates{UtilizationPercentage: aws.String("70.0")}},
					},
				}},
			}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	query := reservationUtilizationQuery()
	query.GroupBySubscription = true

	execution, err := service.ReservationUtilization(
		context.Background(), validSettings(), query, models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Output.UtilizationsByTime[0].Groups) != 1 {
		t.Fatalf("groups = %+v, want one subscription group", execution.Output.UtilizationsByTime[0].Groups)
	}
}

func TestReservationUtilizationClassifiesAWSError(t *testing.T) {
	client := &mockClient{
		reservationUtilizationFn: func(*awscostexplorer.GetReservationUtilizationInput) (*awscostexplorer.GetReservationUtilizationOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	_, err := service.ReservationUtilization(
		context.Background(), validSettings(), reservationUtilizationQuery(), models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err == nil {
		t.Fatal("expected a classified AWS error")
	}
}

func TestSavingsPlansUtilizationCaches(t *testing.T) {
	client := &mockClient{
		savingsPlansUtilizationFn: func(
			*awscostexplorer.GetSavingsPlansUtilizationInput,
		) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
			return &awscostexplorer.GetSavingsPlansUtilizationOutput{
				SavingsPlansUtilizationsByTime: []types.SavingsPlansUtilizationByTime{{
					TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01")},
					Utilization: &types.SavingsPlansUtilization{UtilizationPercentage: aws.String("95.0")},
				}},
			}, nil
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)
	dateRange := models.DateRange{Start: "2026-07-01", End: "2026-07-02"}

	first, err := service.SavingsPlansUtilization(context.Background(), validSettings(), savingsPlansUtilizationQuery(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if first.CacheResult != cache.ResultMiss || len(first.Output.SavingsPlansUtilizationsByTime) != 1 {
		t.Fatalf("unexpected first execution: %+v", first)
	}

	second, err := service.SavingsPlansUtilization(context.Background(), validSettings(), savingsPlansUtilizationQuery(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if second.CacheResult != cache.ResultHit {
		t.Fatalf("second cache result = %s, want hit", second.CacheResult)
	}
	if client.savingsPlansUtilizationCalls != 1 {
		t.Fatalf("AWS calls = %d, want one call reused from cache", client.savingsPlansUtilizationCalls)
	}
}

func TestSavingsPlansUtilizationClassifiesAWSError(t *testing.T) {
	client := &mockClient{
		savingsPlansUtilizationFn: func(*awscostexplorer.GetSavingsPlansUtilizationInput) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
		},
	}
	resultCache, _ := cache.NewMemory(time.Minute, 10)
	service, _ := NewService(client, resultCache)

	_, err := service.SavingsPlansUtilization(
		context.Background(), validSettings(), savingsPlansUtilizationQuery(), models.DateRange{Start: "2026-07-01", End: "2026-07-02"},
	)
	if err == nil {
		t.Fatal("expected a classified AWS error")
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
