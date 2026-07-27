package frames

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func TestConvertUngroupedTimeSeries(t *testing.T) {
	query := baseQuery(models.FormatTimeSeries)
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			periodWithTotal("2026-07-01", "12.34"),
			periodWithTotal("2026-07-02", "23.45"),
		},
	}

	result, err := Convert(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 2 {
		t.Fatalf("frames = %d, rows = %d", len(result), result[0].Rows())
	}
	if got := result[0].Fields[1].At(0).(float64); got != 12.34 {
		t.Fatalf("first amount = %v", got)
	}
	if got := result[0].Fields[1].Config.Unit; got != "currencyUSD" {
		t.Fatalf("unit = %q, want currencyUSD", got)
	}
}

func TestConvertGroupedTimeSeries(t *testing.T) {
	query := baseQuery(models.FormatTimeSeries)
	query.GroupBy = []string{"SERVICE"}
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Groups: []types.Group{
				group("Amazon EC2", "5.00"),
				group("Amazon S3", "2.00"),
			},
		}},
	}

	result, err := Convert(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("frames = %d, want 2", len(result))
	}
	if result[0].Fields[1].Labels["SERVICE"] == "" {
		t.Fatal("grouping label is missing")
	}
	for _, frame := range result {
		if frame.Name == "SERVICE=Amazon EC2" || frame.Name == "SERVICE=Amazon S3" {
			t.Fatalf("frame name contains a dimension prefix: %q", frame.Name)
		}
	}
}

func TestConvertTimeSeriesDoesNotAdvertiseMixedUnits(t *testing.T) {
	query := baseQuery(models.FormatTimeSeries)
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			periodWithMetric("2026-07-01", "1", "USD"),
			periodWithMetric("2026-07-02", "2", "Hrs"),
			periodWithMetric("2026-07-03", "3", "USD"),
		},
	}

	result, err := Convert(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if got := result[0].Fields[1].Config.Unit; got != "" {
		t.Fatalf("mixed series unit = %q, want empty", got)
	}
}

func TestConvertTableAndEmptyResponse(t *testing.T) {
	query := baseQuery(models.FormatTable)
	query.GroupBy = []string{"LINKED_ACCOUNT"}
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Groups:     []types.Group{group("123456789012", "9.99")},
		}},
	}
	result, err := Convert(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Rows() != 1 || result[0].Fields[0].Name != "Period" ||
		result[0].Fields[1].Name != "LINKED_ACCOUNT" {
		t.Fatalf("unexpected table schema: fields=%d rows=%d", len(result[0].Fields), result[0].Rows())
	}
	if got := result[0].Fields[0].At(0); got != "2026-07-01" {
		t.Fatalf("daily table period = %#v, want 2026-07-01", got)
	}

	empty, err := Convert(query, &awscostexplorer.GetCostAndUsageOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 1 || empty[0].Rows() != 0 {
		t.Fatalf("empty response produced %d frames and %d rows", len(empty), empty[0].Rows())
	}
}

func TestConvertMonthlyTableUsesSortablePeriodString(t *testing.T) {
	query := baseQuery(models.FormatTable)
	query.Granularity = models.GranularityMonthly
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{periodWithTotal("2026-02-01", "12.34")},
	}

	result, err := Convert(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if got := result[0].Fields[0].At(0); got != "2026-02" {
		t.Fatalf("monthly table period = %#v, want 2026-02", got)
	}
}

func baseQuery(format string) models.Query {
	return models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		Format:      format,
	}
}

func periodWithTotal(start, amount string) types.ResultByTime {
	return periodWithMetric(start, amount, "USD")
}

func periodWithMetric(start, amount, unit string) types.ResultByTime {
	return types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String(start)},
		Total: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String(amount), Unit: aws.String(unit)},
		},
	}
}

func group(key, amount string) types.Group {
	return types.Group{
		Keys: []string{key},
		Metrics: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String(amount), Unit: aws.String("USD")},
		},
	}
}
