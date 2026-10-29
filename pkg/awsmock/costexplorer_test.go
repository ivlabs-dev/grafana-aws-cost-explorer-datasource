package awsmock

import (
	"context"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

func TestGetCostAndUsageRespectsDateRangeAndGranularity(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-04")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ResultsByTime) != 3 {
		t.Fatalf("ResultsByTime length = %d, want 3 (one per day in [07-01, 07-04))", len(output.ResultsByTime))
	}
	for i, result := range output.ResultsByTime {
		if result.Total == nil || result.Total["UnblendedCost"].Amount == nil {
			t.Fatalf("row %d missing UnblendedCost total: %+v", i, result)
		}
	}
	if aws.ToString(output.ResultsByTime[0].TimePeriod.Start) != "2026-07-01" {
		t.Fatalf("first period start = %q, want 2026-07-01", aws.ToString(output.ResultsByTime[0].TimePeriod.Start))
	}
}

func TestGetCostAndUsageMonthlyGranularity(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityMonthly,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-01-01"), End: aws.String("2026-04-01")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ResultsByTime) != 3 {
		t.Fatalf("ResultsByTime length = %d, want 3 (Jan/Feb/Mar)", len(output.ResultsByTime))
	}
}

func TestGetCostAndUsageGroupedBySingleDimension(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
		GroupBy:     []types.GroupDefinition{{Key: aws.String("SERVICE")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ResultsByTime) != 1 {
		t.Fatalf("ResultsByTime length = %d, want 1", len(output.ResultsByTime))
	}
	row := output.ResultsByTime[0]
	if row.Total != nil {
		t.Fatalf("grouped row should not set Total, got %+v", row.Total)
	}
	if len(row.Groups) != len(mockServices) {
		t.Fatalf("Groups length = %d, want %d (one per mock service)", len(row.Groups), len(mockServices))
	}
	for _, group := range row.Groups {
		if len(group.Keys) != 1 {
			t.Fatalf("expected one key per group for a single-dimension groupBy, got %+v", group.Keys)
		}
	}
}

func TestGetCostAndUsageGroupedByTwoDimensionsIsACrossProduct(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
		GroupBy: []types.GroupDefinition{
			{Key: aws.String("LINKED_ACCOUNT")},
			{Key: aws.String("REGION")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := len(mockLinkedAccounts) * len(mockRegions)
	if len(output.ResultsByTime[0].Groups) != want {
		t.Fatalf("Groups length = %d, want %d (cross product)", len(output.ResultsByTime[0].Groups), want)
	}
	for _, group := range output.ResultsByTime[0].Groups {
		if len(group.Keys) != 2 {
			t.Fatalf("expected two keys per group for a two-dimension groupBy, got %+v", group.Keys)
		}
	}
}

func TestGetCostAndUsageEmptyRangeReturnsNoResults(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetCostAndUsage(context.Background(), &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-01")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ResultsByTime) != 0 {
		t.Fatalf("expected no results for a zero-width range, got %d", len(output.ResultsByTime))
	}
}

func TestGetCostAndUsageIsDeterministic(t *testing.T) {
	client := CostExplorerClient{}
	input := &costexplorer.GetCostAndUsageInput{
		Granularity: types.GranularityDaily,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-03")},
	}
	first, err := client.GetCostAndUsage(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.GetCostAndUsage(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if aws.ToString(first.ResultsByTime[0].Total["UnblendedCost"].Amount) != aws.ToString(second.ResultsByTime[0].Total["UnblendedCost"].Amount) {
		t.Fatal("identical requests should produce identical mock output")
	}
}

func TestGetDimensionValuesFiltersBySearchString(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetDimensionValues(context.Background(), &costexplorer.GetDimensionValuesInput{
		Dimension:    types.DimensionService,
		SearchString: aws.String("lambda"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.DimensionValues) != 1 || aws.ToString(output.DimensionValues[0].Value) != "AWS Lambda" {
		t.Fatalf("unexpected filtered dimension values: %+v", output.DimensionValues)
	}
}

func TestGetTagsReturnsKeysThenValues(t *testing.T) {
	client := CostExplorerClient{}
	keys, err := client.GetTags(context.Background(), &costexplorer.GetTagsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys.Tags) == 0 {
		t.Fatal("expected at least one tag key")
	}

	values, err := client.GetTags(context.Background(), &costexplorer.GetTagsInput{TagKey: aws.String("Environment")})
	if err != nil {
		t.Fatal(err)
	}
	if len(values.Tags) == 0 {
		t.Fatal("expected at least one value for the Environment tag key")
	}
}

func TestGetReservationUtilizationGroupsBySubscriptionWhenRequested(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetReservationUtilization(context.Background(), &costexplorer.GetReservationUtilizationInput{
		Granularity: types.GranularityDaily,
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
		GroupBy:     []types.GroupDefinition{{Key: aws.String("SUBSCRIPTION_ID")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.UtilizationsByTime[0].Total != nil {
		t.Fatal("grouped response should not set Total")
	}
	if len(output.UtilizationsByTime[0].Groups) != len(mockSubscriptionIDs) {
		t.Fatalf("Groups length = %d, want %d", len(output.UtilizationsByTime[0].Groups), len(mockSubscriptionIDs))
	}
}

func TestGetReservationCoverageReturnsCuratedFields(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetReservationCoverage(context.Background(), &costexplorer.GetReservationCoverageInput{
		Granularity: types.GranularityDaily,
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
	})
	if err != nil {
		t.Fatal(err)
	}
	total := output.CoveragesByTime[0].Total
	if total.CoverageHours == nil || total.CoverageHours.CoverageHoursPercentage == nil {
		t.Fatal("expected CoverageHours to be populated")
	}
	if total.CoverageCost == nil || total.CoverageCost.OnDemandCost == nil {
		t.Fatal("expected CoverageCost to be populated")
	}
}

func TestGetSavingsPlansCoverageReturnsCuratedFields(t *testing.T) {
	client := CostExplorerClient{}
	output, err := client.GetSavingsPlansCoverage(context.Background(), &costexplorer.GetSavingsPlansCoverageInput{
		Granularity: types.GranularityDaily,
		TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
	})
	if err != nil {
		t.Fatal(err)
	}
	coverage := output.SavingsPlansCoverages[0].Coverage
	if coverage.CoveragePercentage == nil || coverage.TotalCost == nil {
		t.Fatal("expected a populated Coverage entry")
	}
}

func costQuery(t *testing.T, granularity types.Granularity, start, end string, groupBy ...string) []types.ResultByTime {
	t.Helper()
	input := &costexplorer.GetCostAndUsageInput{
		Granularity: granularity,
		Metrics:     []string{"UnblendedCost"},
		TimePeriod:  &types.DateInterval{Start: aws.String(start), End: aws.String(end)},
	}
	for _, key := range groupBy {
		input.GroupBy = append(input.GroupBy, types.GroupDefinition{Key: aws.String(key)})
	}
	output, err := CostExplorerClient{}.GetCostAndUsage(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return output.ResultsByTime
}

func parseAmount(t *testing.T, s *string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(aws.ToString(s), 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func totalOf(t *testing.T, r types.ResultByTime) float64 {
	t.Helper()
	return parseAmount(t, r.Total["UnblendedCost"].Amount)
}

func sumGroups(t *testing.T, r types.ResultByTime) float64 {
	t.Helper()
	sum := 0.0
	for _, g := range r.Groups {
		sum += parseAmount(t, g.Metrics["UnblendedCost"].Amount)
	}
	return sum
}

func TestGroupedCostsReconcileWithTotal(t *testing.T) {
	total := totalOf(t, costQuery(t, types.GranularityDaily, "2026-07-01", "2026-07-02")[0])
	for _, groupBy := range [][]string{
		{"SERVICE"},
		{"LINKED_ACCOUNT"},
		{"REGION"},
		{"Environment"},
		{"SERVICE", "LINKED_ACCOUNT"},
		{"LINKED_ACCOUNT", "REGION"},
	} {
		got := sumGroups(t, costQuery(t, types.GranularityDaily, "2026-07-01", "2026-07-02", groupBy...)[0])
		if math.Abs(got-total) > 0.01 {
			t.Errorf("groupBy %v sums to %.4f, want total %.4f", groupBy, got, total)
		}
	}
}

func TestMonthlyCostIsSumOfDays(t *testing.T) {
	monthly := totalOf(t, costQuery(t, types.GranularityMonthly, "2026-07-01", "2026-08-01")[0])
	daily := 0.0
	for _, r := range costQuery(t, types.GranularityDaily, "2026-07-01", "2026-08-01") {
		daily += totalOf(t, r)
	}
	if math.Abs(monthly-daily) > 0.01 {
		t.Fatalf("monthly = %.2f, sum of days = %.2f", monthly, daily)
	}
}

func TestMonthlyCostIsClippedToRequestedRange(t *testing.T) {
	partial := totalOf(t, costQuery(t, types.GranularityMonthly, "2026-07-01", "2026-07-11")[0])
	full := totalOf(t, costQuery(t, types.GranularityMonthly, "2026-07-01", "2026-08-01")[0])
	if partial >= full/2 {
		t.Fatalf("10 of 31 days = %.2f should be well under half of the full month %.2f", partial, full)
	}
}

func TestDailyCostDoesNotDependOnRequestedRange(t *testing.T) {
	short := costQuery(t, types.GranularityDaily, "2026-07-10", "2026-07-11")
	long := costQuery(t, types.GranularityDaily, "2026-07-01", "2026-07-31")
	if totalOf(t, short[0]) != totalOf(t, long[9]) {
		t.Fatal("2026-07-10 should cost the same whatever range it is queried in")
	}
}

func TestDailyCostShowsTrendAndWeeklyPattern(t *testing.T) {
	rows := costQuery(t, types.GranularityDaily, "2026-07-06", "2026-08-03") // Mon..Sun x4
	var weekday, weekend []float64
	minV, maxV := math.Inf(1), math.Inf(-1)
	for i, r := range rows {
		v := totalOf(t, r)
		minV, maxV = math.Min(minV, v), math.Max(maxV, v)
		if i%7 >= 5 {
			weekend = append(weekend, v)
		} else {
			weekday = append(weekday, v)
		}
	}
	if maxV/minV < 1.05 {
		t.Fatalf("series is flat: min %.1f max %.1f", minV, maxV)
	}
	if mean(weekend) >= mean(weekday) {
		t.Fatalf("weekend mean %.1f should be below weekday mean %.1f", mean(weekend), mean(weekday))
	}
}

func TestTaxIsProportionalToOtherServices(t *testing.T) {
	row := costQuery(t, types.GranularityDaily, "2026-07-01", "2026-07-02", "SERVICE")[0]
	var tax, other float64
	for _, g := range row.Groups {
		v := parseAmount(t, g.Metrics["UnblendedCost"].Amount)
		if g.Keys[0] == taxService {
			tax = v
		} else {
			other += v
		}
	}
	if math.Abs(tax-other*taxRate) > 0.01 {
		t.Fatalf("tax = %.2f, want %.2f (%.0f%% of %.2f)", tax, other*taxRate, taxRate*100, other)
	}
}

func TestUtilizationAndCoverageAreVariedPercentages(t *testing.T) {
	ctx := context.Background()
	period := &types.DateInterval{Start: aws.String("2026-05-01"), End: aws.String("2026-08-01")}
	ri, err := CostExplorerClient{}.GetReservationCoverage(ctx, &costexplorer.GetReservationCoverageInput{Granularity: types.GranularityDaily, TimePeriod: period})
	if err != nil {
		t.Fatal(err)
	}
	sp, err := CostExplorerClient{}.GetSavingsPlansUtilization(ctx, &costexplorer.GetSavingsPlansUtilizationInput{Granularity: types.GranularityDaily, TimePeriod: period})
	if err != nil {
		t.Fatal(err)
	}
	var riPct, spPct []float64
	for _, c := range ri.CoveragesByTime {
		riPct = append(riPct, parseAmount(t, c.Total.CoverageHours.CoverageHoursPercentage))
	}
	for _, u := range sp.SavingsPlansUtilizationsByTime {
		spPct = append(spPct, parseAmount(t, u.Utilization.UtilizationPercentage))
	}
	for name, series := range map[string][]float64{"RI coverage": riPct, "SP utilization": spPct} {
		lo, hi := slices.Min(series), slices.Max(series)
		if lo < 0 || hi > 100 {
			t.Errorf("%s out of [0,100]: %.1f..%.1f", name, lo, hi)
		}
		if hi-lo < 5 {
			t.Errorf("%s barely varies over 92 days: %.1f..%.1f", name, lo, hi)
		}
	}
}

func mean(vs []float64) float64 {
	sum := 0.0
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}
