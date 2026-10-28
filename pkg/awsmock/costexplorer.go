package awsmock

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

var mockSubscriptionIDs = []string{"sub-1a2b3c", "sub-4d5e6f", "sub-7g8h9i"}

// costExplorer implements awsclient.CostExplorerAPI with synthetic,
// parameter-aware data. Every time-bucketed method walks the actually
// requested date range/granularity so a chart never silently ignores the
// dashboard's selected range -- the one thing that would look obviously
// broken in a demo.
type CostExplorerClient struct{}

func (CostExplorerClient) GetCostAndUsage(
	_ context.Context,
	input *costexplorer.GetCostAndUsageInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetCostAndUsageOutput, error) {
	metric := "UnblendedCost"
	if len(input.Metrics) > 0 {
		metric = input.Metrics[0]
	}
	unit := "USD"
	if metric == "UsageQuantity" {
		unit = "Hrs"
	}

	start, end := dateIntervalStrings(input.TimePeriod)
	granularity := string(input.Granularity)
	periods := walkPeriods(start, end, granularity)
	dimensions := groupDimensions(input.GroupBy)

	results := make([]types.ResultByTime, 0, len(periods))
	for dayIndex, periodStart := range periods {
		result := types.ResultByTime{TimePeriod: periodDateInterval(periodStart, granularity)}
		if len(dimensions) == 0 {
			total := 0.0
			for si, svc := range mockServices {
				total += svc.base * jitter(si, dayIndex)
			}
			result.Total = map[string]types.MetricValue{
				metric: {Amount: amount(total), Unit: aws.String(unit)},
			}
		} else {
			for gi, combo := range groupCombinations(dimensions) {
				value := 0.0
				for _, label := range combo {
					value += baseCostFor(label) * jitter(gi+1, dayIndex)
				}
				result.Groups = append(result.Groups, types.Group{
					Keys: combo,
					Metrics: map[string]types.MetricValue{
						metric: {Amount: amount(value), Unit: aws.String(unit)},
					},
				})
			}
		}
		results = append(results, result)
	}

	return &costexplorer.GetCostAndUsageOutput{ResultsByTime: results}, nil
}

func (CostExplorerClient) GetDimensionValues(
	_ context.Context,
	input *costexplorer.GetDimensionValuesInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetDimensionValuesOutput, error) {
	values := filterBySearch(labelsFor(string(input.Dimension)), aws.ToString(input.SearchString))
	out := make([]types.DimensionValuesWithAttributes, 0, len(values))
	for _, v := range values {
		out = append(out, types.DimensionValuesWithAttributes{Value: aws.String(v)})
	}
	return &costexplorer.GetDimensionValuesOutput{DimensionValues: out}, nil
}

func (CostExplorerClient) GetTags(
	_ context.Context,
	input *costexplorer.GetTagsInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetTagsOutput, error) {
	var values []string
	if input.TagKey == nil {
		values = mockTagKeys
	} else {
		values = mockTagValues[aws.ToString(input.TagKey)]
	}
	return &costexplorer.GetTagsOutput{Tags: filterBySearch(values, aws.ToString(input.SearchString))}, nil
}

func (CostExplorerClient) GetReservationUtilization(
	_ context.Context,
	input *costexplorer.GetReservationUtilizationInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetReservationUtilizationOutput, error) {
	start, end := dateIntervalStrings(input.TimePeriod)
	granularity := string(input.Granularity)
	periods := walkPeriods(start, end, granularity)
	grouped := len(input.GroupBy) > 0

	results := make([]types.UtilizationByTime, 0, len(periods))
	for dayIndex, periodStart := range periods {
		row := types.UtilizationByTime{TimePeriod: periodDateInterval(periodStart, granularity)}
		if grouped {
			for si, sub := range mockSubscriptionIDs {
				row.Groups = append(row.Groups, types.ReservationUtilizationGroup{
					Value:       aws.String(sub),
					Utilization: reservationAggregates(si, dayIndex),
				})
			}
		} else {
			row.Total = reservationAggregates(0, dayIndex)
		}
		results = append(results, row)
	}
	return &costexplorer.GetReservationUtilizationOutput{UtilizationsByTime: results}, nil
}

func (CostExplorerClient) GetSavingsPlansUtilization(
	_ context.Context,
	input *costexplorer.GetSavingsPlansUtilizationInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetSavingsPlansUtilizationOutput, error) {
	start, end := dateIntervalStrings(input.TimePeriod)
	granularity := string(input.Granularity)
	periods := walkPeriods(start, end, granularity)

	results := make([]types.SavingsPlansUtilizationByTime, 0, len(periods))
	for dayIndex, periodStart := range periods {
		util := clampPercent(70 + 25*jitter(0, dayIndex))
		totalCommitment := 100.0
		usedCommitment := totalCommitment * util / 100
		unusedCommitment := totalCommitment - usedCommitment
		onDemandEquivalent := usedCommitment * 1.25
		netSavings := onDemandEquivalent - usedCommitment

		results = append(results, types.SavingsPlansUtilizationByTime{
			TimePeriod: periodDateInterval(periodStart, granularity),
			Utilization: &types.SavingsPlansUtilization{
				TotalCommitment:       decimal(totalCommitment),
				UnusedCommitment:      decimal(unusedCommitment),
				UsedCommitment:        decimal(usedCommitment),
				UtilizationPercentage: decimal(util),
			},
			AmortizedCommitment: &types.SavingsPlansAmortizedCommitment{
				TotalAmortizedCommitment: decimal(totalCommitment),
			},
			Savings: &types.SavingsPlansSavings{
				NetSavings:             decimal(netSavings),
				OnDemandCostEquivalent: decimal(onDemandEquivalent),
			},
		})
	}
	return &costexplorer.GetSavingsPlansUtilizationOutput{SavingsPlansUtilizationsByTime: results}, nil
}

func (CostExplorerClient) GetReservationCoverage(
	_ context.Context,
	input *costexplorer.GetReservationCoverageInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetReservationCoverageOutput, error) {
	start, end := dateIntervalStrings(input.TimePeriod)
	granularity := string(input.Granularity)
	periods := walkPeriods(start, end, granularity)

	results := make([]types.CoverageByTime, 0, len(periods))
	for dayIndex, periodStart := range periods {
		results = append(results, types.CoverageByTime{
			TimePeriod: periodDateInterval(periodStart, granularity),
			Total:      coverageAggregate(dayIndex),
		})
	}
	return &costexplorer.GetReservationCoverageOutput{CoveragesByTime: results}, nil
}

func (CostExplorerClient) GetSavingsPlansCoverage(
	_ context.Context,
	input *costexplorer.GetSavingsPlansCoverageInput,
	_ ...func(*costexplorer.Options),
) (*costexplorer.GetSavingsPlansCoverageOutput, error) {
	start, end := dateIntervalStrings(input.TimePeriod)
	granularity := string(input.Granularity)
	periods := walkPeriods(start, end, granularity)

	results := make([]types.SavingsPlansCoverage, 0, len(periods))
	for dayIndex, periodStart := range periods {
		pct := clampPercent(45 + 20*jitter(2, dayIndex))
		totalCost := 200.0
		spendCovered := totalCost * pct / 100
		onDemandCost := totalCost - spendCovered

		results = append(results, types.SavingsPlansCoverage{
			TimePeriod: periodDateInterval(periodStart, granularity),
			Coverage: &types.SavingsPlansCoverageData{
				CoveragePercentage:         decimal(pct),
				OnDemandCost:               amount(onDemandCost),
				SpendCoveredBySavingsPlans: amount(spendCovered),
				TotalCost:                  amount(totalCost),
			},
		})
	}
	return &costexplorer.GetSavingsPlansCoverageOutput{SavingsPlansCoverages: results}, nil
}

func reservationAggregates(seed, day int) *types.ReservationAggregates {
	util := clampPercent(75 + 20*jitter(seed, day))
	purchasedHours := 24.0
	usedHours := purchasedHours * util / 100
	unusedHours := purchasedHours - usedHours
	onDemandCost := usedHours * 0.4
	amortizedFee := purchasedHours * 0.18
	netSavings := onDemandCost - amortizedFee
	if netSavings < 0 {
		netSavings = 0
	}
	return &types.ReservationAggregates{
		UtilizationPercentage:     decimal(util),
		NetRISavings:              amount(netSavings),
		OnDemandCostOfRIHoursUsed: amount(onDemandCost),
		TotalAmortizedFee:         amount(amortizedFee),
		UnusedHours:               decimal(unusedHours),
		RealizedSavings:           amount(netSavings * 0.8),
		UnrealizedSavings:         amount(netSavings * 0.2),
	}
}

func coverageAggregate(day int) *types.Coverage {
	// Centered around the 40-60% range observed against a real account
	// earlier in development, rather than an arbitrary number.
	pct := clampPercent(40 + 20*jitter(1, day))
	totalHours := 24.0
	reservedHours := totalHours * pct / 100
	onDemandHours := totalHours - reservedHours
	onDemandCost := onDemandHours * 0.45
	return &types.Coverage{
		CoverageHours: &types.CoverageHours{
			CoverageHoursPercentage: decimal(pct),
			OnDemandHours:           decimal(onDemandHours),
			ReservedHours:           decimal(reservedHours),
			TotalRunningHours:       decimal(totalHours),
		},
		CoverageCost: &types.CoverageCost{OnDemandCost: amount(onDemandCost)},
	}
}

func clampPercent(v float64) float64 {
	if v > 100 {
		return 100
	}
	if v < 0 {
		return 0
	}
	return v
}

func dateIntervalStrings(interval *types.DateInterval) (string, string) {
	if interval == nil {
		return "", ""
	}
	return aws.ToString(interval.Start), aws.ToString(interval.End)
}

func periodDateInterval(periodStart time.Time, granularity string) *types.DateInterval {
	return &types.DateInterval{
		Start: aws.String(dateOnly(periodStart)),
		End:   aws.String(dateOnly(periodEnd(periodStart, granularity))),
	}
}

func groupDimensions(groupBy []types.GroupDefinition) []string {
	dims := make([]string, 0, len(groupBy))
	for _, g := range groupBy {
		if g.Key != nil {
			dims = append(dims, *g.Key)
		}
	}
	return dims
}

func groupCombinations(dimensions []string) [][]string {
	combos := [][]string{{}}
	for _, dim := range dimensions {
		labels := labelsFor(dim)
		next := make([][]string, 0, len(combos)*len(labels))
		for _, combo := range combos {
			for _, label := range labels {
				extended := append(append([]string{}, combo...), label)
				next = append(next, extended)
			}
		}
		combos = next
	}
	return combos
}

func filterBySearch(values []string, search string) []string {
	if search == "" {
		return values
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), strings.ToLower(search)) {
			out = append(out, v)
		}
	}
	return out
}
