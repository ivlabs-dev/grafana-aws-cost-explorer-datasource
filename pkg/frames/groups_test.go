package frames

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func TestPrepareResultsUngroupedUsesMetricNameAndChronologicalPoints(t *testing.T) {
	query := presentationQuery()
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			presentationPeriodWithTotal("2026-07-02", "2", "USD"),
			presentationPeriodWithTotal("2026-07-01", "1", "USD"),
		},
	}

	result := mustPrepareResults(t, query, output)
	if len(result) != 1 {
		t.Fatalf("series count = %d, want 1", len(result))
	}
	if result[0].DisplayName != "UnblendedCost" {
		t.Fatalf("display name = %q, want UnblendedCost", result[0].DisplayName)
	}
	if got := result[0].Points[0].PeriodStart.Format(time.DateOnly); got != "2026-07-01" {
		t.Fatalf("first period = %q, want 2026-07-01", got)
	}
}

func TestPrepareResultsUsesCleanNamesAndPreservesDimensionLabels(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE", "LINKED_ACCOUNT"}
	output := presentationOutput("2026-07-01",
		presentationGroup([]string{"Amazon EC2", "Production"}, "1", "USD"),
		presentationGroup([]string{"Amazon S3"}, "2", "USD"),
	)

	result := mustPrepareResults(t, query, output)
	byName := seriesByName(result)

	ec2 := byName["Amazon EC2 · Production"]
	if ec2 == nil {
		t.Fatalf("clean two-dimension display name is missing: %#v", names(result))
	}
	if ec2.Labels["SERVICE"] != "Amazon EC2" || ec2.Labels["LINKED_ACCOUNT"] != "Production" {
		t.Fatalf("dimension labels = %#v", ec2.Labels)
	}
	s3 := byName["Amazon S3"]
	if s3 == nil {
		t.Fatalf("missing-value display name is not clean: %#v", names(result))
	}
	if s3.Labels["LINKED_ACCOUNT"] != "" {
		t.Fatalf("missing dimension label = %q, want empty", s3.Labels["LINKED_ACCOUNT"])
	}
	if len(s3.GroupValues) != 2 || s3.GroupValues[1] != "" {
		t.Fatalf("table-consumable group values = %#v", s3.GroupValues)
	}
}

func TestPrepareResultsKeepsDuplicateVisibleNamesUniqueThroughLabels(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE", "REGION"}
	output := presentationOutput("2026-07-01",
		presentationGroup([]string{"Shared", ""}, "1", "USD"),
		presentationGroup([]string{"", "Shared"}, "2", "USD"),
	)

	result := mustPrepareResults(t, query, output)
	if len(result) != 2 || result[0].DisplayName != "Shared" || result[1].DisplayName != "Shared" {
		t.Fatalf("display names = %#v, want two Shared names", names(result))
	}
	if result[0].Identity == result[1].Identity {
		t.Fatal("series identities are not unique")
	}
	if reflect.DeepEqual(result[0].Labels, result[1].Labels) {
		t.Fatalf("dimension labels should preserve uniqueness: %#v", result[0].Labels)
	}
}

func TestPrepareResultsUsesReadableFallbackWhenAllGroupValuesAreMissing(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE", "REGION"}

	result := mustPrepareResults(t, query, presentationOutput(
		"2026-07-01",
		presentationGroup(nil, "1", "USD"),
	))
	if got := result[0].DisplayName; got != "(no group value)" {
		t.Fatalf("display name = %q, want readable fallback", got)
	}
}

func TestPrepareResultsTopFiveRanksAcrossCompleteRange(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5
	query.IncludeOther = aws.Bool(false)
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			presentationPeriod("2026-07-01",
				presentationGroup([]string{"steady"}, "100", "USD"),
				presentationGroup([]string{"a"}, "9", "USD"),
				presentationGroup([]string{"b"}, "8", "USD"),
				presentationGroup([]string{"c"}, "7", "USD"),
				presentationGroup([]string{"d"}, "6", "USD"),
				presentationGroup([]string{"latest-spike"}, "0", "USD"),
			),
			presentationPeriod("2026-07-02",
				presentationGroup([]string{"steady"}, "0", "USD"),
				presentationGroup([]string{"a"}, "0", "USD"),
				presentationGroup([]string{"b"}, "0", "USD"),
				presentationGroup([]string{"c"}, "0", "USD"),
				presentationGroup([]string{"d"}, "0", "USD"),
				presentationGroup([]string{"latest-spike"}, "10", "USD"),
			),
		},
	}

	result := mustPrepareResults(t, query, output)
	if got, want := names(result), []string{"steady", "latest-spike", "a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ranked names = %#v, want %#v", got, want)
	}
}

func TestPrepareResultsOtherIsAggregatedExactlyPerPeriod(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			presentationPeriod("2026-07-01",
				presentationGroup([]string{"a"}, "10", "USD"),
				presentationGroup([]string{"b"}, "9", "USD"),
				presentationGroup([]string{"c"}, "8", "USD"),
				presentationGroup([]string{"d"}, "7", "USD"),
				presentationGroup([]string{"e"}, "6", "USD"),
				presentationGroup([]string{"f"}, "0.1", "USD"),
				presentationGroup([]string{"g"}, "0.2", "USD"),
			),
			presentationPeriod("2026-07-02",
				presentationGroup([]string{"f"}, "0.2", "USD"),
				presentationGroup([]string{"g"}, "0.1", "USD"),
			),
		},
	}

	result := mustPrepareResults(t, query, output)
	if len(result) != 6 {
		t.Fatalf("series count = %d, want 6", len(result))
	}
	other := result[5]
	if !other.IsOther || other.DisplayName != "Other" {
		t.Fatalf("Other series = %#v", other)
	}
	if got := other.Labels[otherGroupLabel]; got != "Other" {
		t.Fatalf("Other label = %q", got)
	}
	if _, hasOriginalLabel := other.Labels["SERVICE"]; hasOriginalLabel {
		t.Fatalf("Other exposes an original grouping label: %#v", other.Labels)
	}
	if len(other.Points) != 2 || other.Points[0].Amount != 0.3 || other.Points[1].Amount != 0.3 {
		t.Fatalf("Other points = %#v, want exact per-period 0.3 values", other.Points)
	}
	if got := other.GroupValues; len(got) != 1 || got[0] != "Other" {
		t.Fatalf("table Other values = %#v", got)
	}
}

func TestPrepareResultsOtherIncludesZeroForEveryReturnedPeriod(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 1
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			presentationPeriod("2026-07-01",
				presentationGroup([]string{"top"}, "10", "USD"),
				presentationGroup([]string{"remainder"}, "1", "USD"),
			),
			presentationPeriod("2026-07-02",
				presentationGroup([]string{"top"}, "10", "USD"),
			),
		},
	}

	result := mustPrepareResults(t, query, output)
	if len(result) != 2 || !result[1].IsOther {
		t.Fatalf("expected Top 1 and Other, got %#v", result)
	}
	if got := result[1].Points; len(got) != 2 || got[0].Amount != 1 || got[1].Amount != 0 {
		t.Fatalf("Other points = %#v, want 1 then 0", got)
	}
}

func TestPrepareResultsTopNWithoutOtherDropsRemainingGroups(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5
	query.IncludeOther = aws.Bool(false)
	groups := make([]types.Group, 0, 6)
	for index, name := range []string{"a", "b", "c", "d", "e", "f"} {
		groups = append(groups, presentationGroup([]string{name}, amount(index+1), "USD"))
	}

	result := mustPrepareResults(t, query, presentationOutput("2026-07-01", groups...))
	if len(result) != 5 {
		t.Fatalf("series count = %d, want 5", len(result))
	}
	for _, item := range result {
		if item.IsOther {
			t.Fatal("Other was returned while disabled")
		}
	}
}

func TestPrepareResultsTopNDoesNotCreateOtherForFewerGroups(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5

	result := mustPrepareResults(t, query, presentationOutput("2026-07-01",
		presentationGroup([]string{"a"}, "2", "USD"),
		presentationGroup([]string{"b"}, "1", "USD"),
	))
	if len(result) != 2 || result[0].IsOther || result[1].IsOther {
		t.Fatalf("unexpected result for fewer groups: %#v", result)
	}
}

func TestPrepareResultsEqualTotalsUseStableIdentityTieBreak(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5
	query.IncludeOther = aws.Bool(false)
	output := presentationOutput("2026-07-01",
		presentationGroup([]string{"f"}, "1", "USD"),
		presentationGroup([]string{"e"}, "1", "USD"),
		presentationGroup([]string{"d"}, "1", "USD"),
		presentationGroup([]string{"c"}, "1", "USD"),
		presentationGroup([]string{"b"}, "1", "USD"),
		presentationGroup([]string{"a"}, "1", "USD"),
	)

	first := names(mustPrepareResults(t, query, output))
	second := names(mustPrepareResults(t, query, output))
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("tie ordering is unstable: %#v then %#v", first, second)
	}
	if got, want := first, []string{"a", "b", "c", "d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tie ordering = %#v, want %#v", got, want)
	}
}

func TestPrepareResultsEmptyResponseIsValid(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5

	result := mustPrepareResults(t, query, &awscostexplorer.GetCostAndUsageOutput{})
	if len(result) != 0 {
		t.Fatalf("empty response produced %d series", len(result))
	}
}

func TestPrepareResultsRanksAndAggregatesDifferentUnitsSeparately(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	query.TopN = 5
	groups := make([]types.Group, 0, 12)
	for _, unit := range []string{"Hrs", "USD"} {
		for index, name := range []string{"a", "b", "c", "d", "e", "f"} {
			groups = append(groups, presentationGroup([]string{name}, amount(index+1), unit))
		}
	}

	result := mustPrepareResults(t, query, presentationOutput("2026-07-01", groups...))
	if len(result) != 12 {
		t.Fatalf("series count = %d, want 5 + Other for each unit", len(result))
	}
	otherUnits := make(map[string]float64)
	for _, item := range result {
		if item.IsOther {
			otherUnits[item.Unit] = item.Points[0].Amount
			if item.Labels[unitGroupLabel] != item.Unit {
				t.Fatalf("unit label does not preserve Other identity: %#v", item.Labels)
			}
		}
	}
	if !reflect.DeepEqual(otherUnits, map[string]float64{"Hrs": 1, "USD": 1}) {
		t.Fatalf("Other units were combined: %#v", otherUnits)
	}
}

func TestPrepareResultsMergesPaginatedDuplicatesBeforeRanking(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	output := &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			presentationPeriod("2026-07-01", presentationGroup([]string{"a"}, "0.1", "USD")),
			presentationPeriod("2026-07-01", presentationGroup([]string{"a"}, "0.2", "USD")),
		},
	}

	result := mustPrepareResults(t, query, output)
	if len(result) != 1 || len(result[0].Points) != 1 || result[0].Points[0].Amount != 0.3 {
		t.Fatalf("paginated duplicate was not merged exactly: %#v", result)
	}
}

func TestPrepareResultsRejectsInvalidDecimal(t *testing.T) {
	query := presentationQuery()
	query.GroupBy = []string{"SERVICE"}
	_, err := PrepareResults(query, presentationOutput(
		"2026-07-01",
		presentationGroup([]string{"a"}, "not-a-number", "USD"),
	))
	if err == nil {
		t.Fatal("expected invalid decimal error")
	}
}

func presentationQuery() models.Query {
	query := models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		Format:      models.FormatTimeSeries,
	}
	query.ApplyDefaults()
	return query
}

func presentationOutput(start string, groups ...types.Group) *awscostexplorer.GetCostAndUsageOutput {
	return &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{presentationPeriod(start, groups...)},
	}
}

func presentationPeriod(start string, groups ...types.Group) types.ResultByTime {
	return types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String(start)},
		Groups:     groups,
	}
}

func presentationPeriodWithTotal(start, value, unit string) types.ResultByTime {
	return types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String(start)},
		Total: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String(value), Unit: aws.String(unit)},
		},
	}
}

func presentationGroup(keys []string, value, unit string) types.Group {
	return types.Group{
		Keys: keys,
		Metrics: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String(value), Unit: aws.String(unit)},
		},
	}
}

func mustPrepareResults(
	t *testing.T,
	query models.Query,
	output *awscostexplorer.GetCostAndUsageOutput,
) []PreparedSeries {
	t.Helper()
	result, err := PrepareResults(query, output)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func seriesByName(items []PreparedSeries) map[string]*PreparedSeries {
	result := make(map[string]*PreparedSeries, len(items))
	for index := range items {
		result[items[index].DisplayName] = &items[index]
	}
	return result
}

func names(items []PreparedSeries) []string {
	result := make([]string, len(items))
	for index := range items {
		result[index] = items[index].DisplayName
	}
	return result
}

func amount(value int) string {
	return strconv.Itoa(value)
}
