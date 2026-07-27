package costexplorer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

const maxPages = 1_000

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
