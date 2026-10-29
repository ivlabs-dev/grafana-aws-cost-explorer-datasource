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
	rangeEnd, _ := time.Parse(time.DateOnly, end)
	combos := groupCombinations(dimensions)

	results := make([]types.ResultByTime, 0, len(periods))
	for _, periodStart := range periods {
		result := types.ResultByTime{TimePeriod: periodDateInterval(periodStart, granularity)}

		// A period's cost is the sum of its days, clipped to the requested
		// range so a partial final month reads as month-to-date.
		to := periodEnd(periodStart, granularity)
		if to.After(rangeEnd) {
			to = rangeEnd
		}
		total := 0.0
		values := make([]float64, len(combos))
		for day := periodStart; day.Before(to); day = day.AddDate(0, 0, 1) {
			costs := serviceCosts(day)
			for _, c := range costs {
				total += c
			}
			for ci, combo := range combos {
				values[ci] += combo.dailyValue(dimensions, costs, day)
			}
		}

		if len(dimensions) == 0 {
			result.Total = map[string]types.MetricValue{
				metric: {Amount: amount(total), Unit: aws.String(unit)},
			}
		} else {
			for ci, combo := range combos {
				result.Groups = append(result.Groups, types.Group{
					Keys: combo.labels,
					Metrics: map[string]types.MetricValue{
						metric: {Amount: amount(values[ci]), Unit: aws.String(unit)},
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
	for _, periodStart := range periods {
		days := periodDays(periodStart, granularity)
		row := types.UtilizationByTime{TimePeriod: periodDateInterval(periodStart, granularity)}
		if grouped {
			for si, sub := range mockSubscriptionIDs {
				row.Groups = append(row.Groups, types.ReservationUtilizationGroup{
					Value:       aws.String(sub),
					Utilization: reservationAggregates(si, periodStart, days*subscriptionSizes[si]),
				})
			}
		} else {
			row.Total = reservationAggregates(len(mockSubscriptionIDs), periodStart, days*totalSubscriptionSize)
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
	for _, periodStart := range periods {
		util := clampPercent(signal(10, periodStart, 93, 4, 55, 0, 1))
		totalCommitment := 60.0 * periodDays(periodStart, granularity)
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
	for _, periodStart := range periods {
		results = append(results, types.CoverageByTime{
			TimePeriod: periodDateInterval(periodStart, granularity),
			Total:      coverageAggregate(periodStart, periodDays(periodStart, granularity)),
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
	for _, periodStart := range periods {
		pct := clampPercent(signal(11, periodStart, 58, 8, 80, 4, 1.5))
		totalCost := 200.0 * periodDays(periodStart, granularity)
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

// subscriptionSizes is each mock reservation subscription's relative size
// (instances), so per-subscription hours and dollars differ.
var (
	subscriptionSizes     = []float64{10, 6, 4}
	totalSubscriptionSize = 20.0
)

// subscriptionUtilization centers each subscription on a different level so
// grouped charts separate: one healthy, one decent, one underused.
var subscriptionUtilization = []float64{96, 88, 74, 89}

// reservationAggregates builds one utilization row. size is the number of
// reserved instances times the days in the period, so hours and dollars
// scale with the requested granularity.
func reservationAggregates(seed int, t time.Time, size float64) *types.ReservationAggregates {
	util := clampPercent(signal(seed, t, subscriptionUtilization[seed], 3, float64(60+13*seed), 0, 1.5))
	purchasedHours := 24.0 * size
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

func coverageAggregate(t time.Time, days float64) *types.Coverage {
	pct := clampPercent(signal(12, t, 52, 9, 70, 3, 1.2))
	totalHours := 24.0 * totalSubscriptionSize * days
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

// periodDays is the number of days in one granularity bucket.
func periodDays(periodStart time.Time, granularity string) float64 {
	return periodEnd(periodStart, granularity).Sub(periodStart).Hours() / 24
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

// groupCombo is one cell of a group-by cross product: its display labels and
// each label's index within its dimension's labelsFor list.
type groupCombo struct {
	labels  []string
	indexes []int
}

// dailyValue is this combo's cost on one day. A SERVICE dimension selects
// that service's cost; otherwise the day's total is used. Every other
// dimension then takes its label's share, so groupings reconcile.
func (c groupCombo) dailyValue(dimensions []string, serviceDay []float64, day time.Time) float64 {
	value := 0.0
	for _, cost := range serviceDay {
		value += cost
	}
	serviceUsed := false
	for k, dim := range dimensions {
		if dim == "SERVICE" && !serviceUsed {
			value = serviceDay[c.indexes[k]]
			serviceUsed = true
		}
	}
	for k, dim := range dimensions {
		if dim != "SERVICE" {
			value *= sharesFor(dim, day)[c.indexes[k]]
		}
	}
	return value
}

func groupCombinations(dimensions []string) []groupCombo {
	combos := []groupCombo{{}}
	for _, dim := range dimensions {
		labels := labelsFor(dim)
		next := make([]groupCombo, 0, len(combos)*len(labels))
		for _, combo := range combos {
			for i, label := range labels {
				next = append(next, groupCombo{
					labels:  append(append([]string{}, combo.labels...), label),
					indexes: append(append([]int{}, combo.indexes...), i),
				})
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
