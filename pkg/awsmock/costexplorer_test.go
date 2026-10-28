package awsmock

import (
	"context"
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
