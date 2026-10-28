package costexplorer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

const (
	maxPages = 1_000
	// lookupMaxResults bounds how many suggestions this plugin returns for
	// dimension-value and tag lookups, applied client-side. AWS's GetTags API
	// rejects a MaxResults parameter unless paired with SortBy, so for tags
	// this plugin truncates after fetching instead of asking AWS to limit it.
	lookupMaxResults = 100
)

// FilterableDimensions are the Cost Explorer dimensions this plugin's query
// filter supports, and therefore the only ones value lookup accepts.
var FilterableDimensions = map[string]struct{}{
	"SERVICE":        {},
	"LINKED_ACCOUNT": {},
	"REGION":         {},
}

type Service struct {
	client awsclient.CostExplorerAPI
	cache  cache.Cache
}

type Execution struct {
	Output                *awscostexplorer.GetCostAndUsageOutput
	CacheResult           cache.Result
	CacheAge              time.Duration
	CacheTTL              time.Duration
	Pages                 int
	AWSDuration           time.Duration
	ContainsEstimatedData bool
}

type cachedResponse struct {
	Output      *awscostexplorer.GetCostAndUsageOutput
	Pages       int
	AWSDuration time.Duration
}

func NewService(client awsclient.CostExplorerAPI, resultCache cache.Cache) (*Service, error) {
	if client == nil {
		return nil, fmt.Errorf("client for AWS Cost Explorer must not be nil")
	}
	if resultCache == nil {
		return nil, fmt.Errorf("cache must not be nil")
	}
	return &Service{client: client, cache: resultCache}, nil
}

func (s *Service) Execute(
	ctx context.Context,
	settings models.PluginSettings,
	query models.Query,
	dateRange models.DateRange,
) (*Execution, error) {
	query.ApplyDefaults()
	if err := query.Validate(); err != nil {
		return nil, err
	}

	input, err := BuildInput(query, dateRange)
	if err != nil {
		return nil, err
	}
	key, err := CacheKey(settings.CredentialContext(), query, dateRange)
	if err != nil {
		return nil, err
	}

	value, cacheDetails, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		output, pages, loadErr := s.fetchAll(loadContext, input)
		return &cachedResponse{
			Output:      output,
			Pages:       pages,
			AWSDuration: time.Since(started),
		}, loadErr
	})

	cached, ok := value.(*cachedResponse)
	if !ok || cached == nil {
		if err != nil {
			return &Execution{
				CacheResult: cacheDetails.Result,
				CacheAge:    cacheDetails.Age,
				CacheTTL:    cacheDetails.TTL,
			}, ClassifyError(err)
		}
		return nil, fmt.Errorf("cache returned an invalid AWS Cost Explorer response")
	}

	awsDuration := time.Duration(0)
	pages := 0
	if cacheDetails.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
		pages = cached.Pages
	}
	execution := &Execution{
		Output:                cached.Output,
		CacheResult:           cacheDetails.Result,
		CacheAge:              cacheDetails.Age,
		CacheTTL:              cacheDetails.TTL,
		Pages:                 pages,
		AWSDuration:           awsDuration,
		ContainsEstimatedData: containsEstimatedData(cached.Output),
	}
	if err != nil {
		return execution, ClassifyError(err)
	}

	if cached.Output == nil {
		return nil, fmt.Errorf("cache returned an invalid AWS Cost Explorer response")
	}

	return execution, nil
}

func (e Execution) CacheStatus() cache.Result {
	return (cache.Details{Result: e.CacheResult}).MetadataStatus()
}

func containsEstimatedData(output *awscostexplorer.GetCostAndUsageOutput) bool {
	if output == nil {
		return false
	}
	for _, result := range output.ResultsByTime {
		if result.Estimated {
			return true
		}
	}
	return false
}

func BuildInput(query models.Query, dateRange models.DateRange) (*awscostexplorer.GetCostAndUsageInput, error) {
	if dateRange.Start == "" || dateRange.End == "" {
		return nil, fmt.Errorf("date range for AWS Cost Explorer must include start and end dates")
	}
	start, err := time.Parse(time.DateOnly, dateRange.Start)
	if err != nil {
		return nil, fmt.Errorf("invalid Cost Explorer start date: %w", err)
	}
	end, err := time.Parse(time.DateOnly, dateRange.End)
	if err != nil {
		return nil, fmt.Errorf("invalid Cost Explorer end date: %w", err)
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end date for AWS Cost Explorer must be after start date")
	}

	input := &awscostexplorer.GetCostAndUsageInput{
		Granularity: types.Granularity(query.Granularity),
		Metrics:     []string{query.Metric},
		TimePeriod: &types.DateInterval{
			Start: aws.String(dateRange.Start),
			End:   aws.String(dateRange.End),
		},
	}

	for _, group := range query.GroupBy {
		input.GroupBy = append(input.GroupBy, types.GroupDefinition{
			Key:  aws.String(awsDimension(group)),
			Type: types.GroupDefinitionTypeDimension,
		})
	}
	input.Filter = buildFilter(query.Filter)

	return input, nil
}

func CacheKey(credentialContext string, query models.Query, dateRange models.DateRange) (string, error) {
	upstreamQuery := struct {
		Metric      string             `json:"metric"`
		Granularity string             `json:"granularity"`
		GroupBy     []string           `json:"groupBy,omitempty"`
		Filter      models.QueryFilter `json:"filter,omitempty"`
	}{
		Metric:      query.Metric,
		Granularity: query.Granularity,
		GroupBy:     query.GroupBy,
		Filter:      query.Filter,
	}
	payload := struct {
		CredentialContext string           `json:"credentialContext"`
		DateRange         models.DateRange `json:"dateRange"`
		Query             any              `json:"query"`
	}{
		CredentialContext: credentialContext,
		DateRange:         dateRange,
		Query:             upstreamQuery,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode cache key: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// DimensionValues returns known AWS Cost Explorer values for a filterable
// dimension (SERVICE, LINKED_ACCOUNT, or REGION), optionally narrowed by a
// search string. Results are cached like Execute to bound AWS API usage.
func (s *Service) DimensionValues(
	ctx context.Context,
	settings models.PluginSettings,
	dimension string,
	search string,
	dateRange models.DateRange,
) ([]string, error) {
	if _, ok := FilterableDimensions[dimension]; !ok {
		return nil, fmt.Errorf("dimension %q does not support value lookup", dimension)
	}

	timePeriod, err := dateInterval(dateRange)
	if err != nil {
		return nil, err
	}

	input := &awscostexplorer.GetDimensionValuesInput{
		Dimension:  types.Dimension(dimension),
		TimePeriod: timePeriod,
		MaxResults: aws.Int32(lookupMaxResults),
	}
	if strings.TrimSpace(search) != "" {
		input.SearchString = aws.String(search)
	}

	return s.cachedLookup(ctx, settings, []string{"dimension-values", dimension, search}, dateRange,
		func(loadContext context.Context) ([]string, error) {
			output, loadErr := s.client.GetDimensionValues(loadContext, input)
			if loadErr != nil {
				return nil, loadErr
			}
			values := make([]string, 0, len(output.DimensionValues))
			for _, dv := range output.DimensionValues {
				if dv.Value != nil && strings.TrimSpace(*dv.Value) != "" {
					values = append(values, *dv.Value)
				}
			}
			sort.Strings(values)
			return values, nil
		})
}

// TagKeys returns known Cost Allocation Tag keys, optionally narrowed by a
// search string.
func (s *Service) TagKeys(
	ctx context.Context,
	settings models.PluginSettings,
	search string,
	dateRange models.DateRange,
) ([]string, error) {
	return s.tags(ctx, settings, "", search, dateRange)
}

// TagValues returns known values for a specific Cost Allocation Tag key,
// optionally narrowed by a search string.
func (s *Service) TagValues(
	ctx context.Context,
	settings models.PluginSettings,
	key string,
	search string,
	dateRange models.DateRange,
) ([]string, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("tag key is required to look up tag values")
	}
	return s.tags(ctx, settings, key, search, dateRange)
}

func (s *Service) tags(
	ctx context.Context,
	settings models.PluginSettings,
	key string,
	search string,
	dateRange models.DateRange,
) ([]string, error) {
	timePeriod, err := dateInterval(dateRange)
	if err != nil {
		return nil, err
	}

	// GetTags rejects MaxResults unless it's paired with SortBy (unlike
	// GetDimensionValues), so this plugin fetches AWS's default result set
	// and truncates it after sorting instead.
	input := &awscostexplorer.GetTagsInput{
		TimePeriod: timePeriod,
	}
	if key != "" {
		input.TagKey = aws.String(key)
	}
	if strings.TrimSpace(search) != "" {
		input.SearchString = aws.String(search)
	}

	return s.cachedLookup(ctx, settings, []string{"tags", key, search}, dateRange,
		func(loadContext context.Context) ([]string, error) {
			output, loadErr := s.client.GetTags(loadContext, input)
			if loadErr != nil {
				return nil, loadErr
			}
			values := make([]string, 0, len(output.Tags))
			for _, tag := range output.Tags {
				if strings.TrimSpace(tag) != "" {
					values = append(values, tag)
				}
			}
			sort.Strings(values)
			if len(values) > lookupMaxResults {
				values = values[:lookupMaxResults]
			}
			return values, nil
		})
}

func (s *Service) cachedLookup(
	ctx context.Context,
	settings models.PluginSettings,
	discriminator []string,
	dateRange models.DateRange,
	load func(context.Context) ([]string, error),
) ([]string, error) {
	key, err := lookupCacheKey(settings.CredentialContext(), discriminator, dateRange)
	if err != nil {
		return nil, err
	}

	value, _, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		return load(loadContext)
	})
	if err != nil {
		return nil, ClassifyError(err)
	}
	values, ok := value.([]string)
	if !ok {
		return nil, fmt.Errorf("cache returned an invalid AWS Cost Explorer lookup response")
	}
	return values, nil
}

func lookupCacheKey(credentialContext string, discriminator []string, dateRange models.DateRange) (string, error) {
	payload := struct {
		CredentialContext string           `json:"credentialContext"`
		DateRange         models.DateRange `json:"dateRange"`
		Discriminator     []string         `json:"discriminator"`
	}{
		CredentialContext: credentialContext,
		DateRange:         dateRange,
		Discriminator:     discriminator,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode lookup cache key: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func dateInterval(dateRange models.DateRange) (*types.DateInterval, error) {
	if dateRange.Start == "" || dateRange.End == "" {
		return nil, fmt.Errorf("date range for AWS Cost Explorer lookup must include start and end dates")
	}
	return &types.DateInterval{
		Start: aws.String(dateRange.Start),
		End:   aws.String(dateRange.End),
	}, nil
}

func (s *Service) fetchAll(
	ctx context.Context,
	baseInput *awscostexplorer.GetCostAndUsageInput,
) (*awscostexplorer.GetCostAndUsageOutput, int, error) {
	combined := &awscostexplorer.GetCostAndUsageOutput{}
	seenTokens := make(map[string]struct{})
	var token *string

	for page := 1; page <= maxPages; page++ {
		input := *baseInput
		input.NextPageToken = token

		output, err := s.client.GetCostAndUsage(ctx, &input)
		if err != nil {
			return nil, page, err
		}
		if output == nil {
			return nil, page, fmt.Errorf("AWS returned an empty GetCostAndUsage response")
		}

		if page == 1 {
			combined.GroupDefinitions = append(combined.GroupDefinitions, output.GroupDefinitions...)
			combined.DimensionValueAttributes = append(combined.DimensionValueAttributes, output.DimensionValueAttributes...)
		}
		combined.ResultsByTime = append(combined.ResultsByTime, output.ResultsByTime...)

		if output.NextPageToken == nil || *output.NextPageToken == "" {
			return combined, page, nil
		}
		if _, duplicate := seenTokens[*output.NextPageToken]; duplicate {
			return nil, page, fmt.Errorf("AWS returned a repeated pagination token")
		}
		seenTokens[*output.NextPageToken] = struct{}{}
		token = output.NextPageToken
	}

	return nil, maxPages, fmt.Errorf("response from AWS Cost Explorer exceeded the %d-page safety limit", maxPages)
}

func buildFilter(filter models.QueryFilter) *types.Expression {
	expressions := make([]types.Expression, 0, 4)
	addDimension := func(key types.Dimension, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		expressions = append(expressions, types.Expression{
			Dimensions: &types.DimensionValues{
				Key:    key,
				Values: []string{value},
			},
		})
	}

	addDimension(types.DimensionService, filter.Service)
	addDimension(types.DimensionLinkedAccount, filter.LinkedAccount)
	addDimension(types.DimensionRegion, filter.Region)

	if filter.TagKey != "" && filter.TagValue != "" {
		expressions = append(expressions, types.Expression{
			Tags: &types.TagValues{
				Key:    aws.String(filter.TagKey),
				Values: []string{filter.TagValue},
			},
		})
	}

	switch len(expressions) {
	case 0:
		return nil
	case 1:
		return &expressions[0]
	default:
		return &types.Expression{And: expressions}
	}
}

func awsDimension(group string) string {
	if group == "AVAILABILITY_ZONE" {
		return "AZ"
	}
	return group
}

func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("AWS request was canceled: %w", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("AWS request timed out; retry or increase Grafana's data proxy timeout: %w", err)
	}

	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return fmt.Errorf("AWS Cost Explorer request failed; verify network connectivity and the configured AWS endpoint")
	}

	switch strings.ToLower(apiError.ErrorCode()) {
	case "accessdenied", "accessdeniedexception", "unauthorizedexception":
		return fmt.Errorf("AWS denied the request; grant ce:GetCostAndUsage and verify the role trust policy")
	case "unrecognizedclientexception", "invalidclienttokenid", "signaturedoesnotmatch", "expiredtokenexception":
		return fmt.Errorf("AWS credentials are invalid or expired; verify the configured credential source")
	case "validationexception":
		return fmt.Errorf("AWS rejected the Cost Explorer query as invalid; verify the dates, groupings, and filter values")
	case "limitexceededexception", "throttling", "throttlingexception", "requestlimitexceeded":
		return fmt.Errorf("AWS Cost Explorer throttled the request; retry after a short delay")
	default:
		return fmt.Errorf("AWS Cost Explorer returned API error %s", apiError.ErrorCode())
	}
}

// isDataUnavailable reports whether err is AWS Cost Explorer's
// DataUnavailableException. The Reservation/Savings Plans utilization APIs
// can return this for an account or period with no eligible usage at all
// (e.g. an account with no Savings-Plans-eligible spend) -- that's a normal
// "nothing to report" condition, not a real failure, so callers treat it as
// an empty successful result instead of surfacing an error.
func isDataUnavailable(err error) bool {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return false
	}
	return strings.EqualFold(apiError.ErrorCode(), "DataUnavailableException")
}
