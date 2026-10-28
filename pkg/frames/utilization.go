package frames

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// utilizationRow is one parsed (period[, group]) row, keyed by the curated
// metric names in models.ReservationUtilizationMetrics /
// models.SavingsPlansUtilizationMetrics.
type utilizationRow struct {
	periodStart time.Time
	groupValue  string // subscription ID, or "" when ungrouped
	metrics     map[string]float64
}

// ConvertReservationUtilization converts a GetReservationUtilization response
// into table or time-series frames, depending on query.Format.
func ConvertReservationUtilization(
	query models.Query,
	output *awscostexplorer.GetReservationUtilizationOutput,
) (data.Frames, error) {
	if output == nil {
		return nil, fmt.Errorf("response from AWS Reservation Utilization is nil")
	}

	rows := make([]utilizationRow, 0, len(output.UtilizationsByTime))
	for _, period := range output.UtilizationsByTime {
		start, err := dateIntervalStartUTC(period.TimePeriod)
		if err != nil {
			return nil, err
		}
		if len(period.Groups) == 0 {
			metrics, err := reservationAggregateMetrics(period.Total)
			if err != nil {
				return nil, err
			}
			rows = append(rows, utilizationRow{periodStart: start, metrics: metrics})
			continue
		}
		for _, group := range period.Groups {
			metrics, err := reservationAggregateMetrics(group.Utilization)
			if err != nil {
				return nil, err
			}
			rows = append(rows, utilizationRow{
				periodStart: start,
				groupValue:  stringValue(group.Value),
				metrics:     metrics,
			})
		}
	}

	groupColumn := ""
	if query.GroupBySubscription {
		groupColumn = "Subscription ID"
	}
	return utilizationFrames(query, groupColumn, rows, models.ReservationUtilizationMetrics)
}

// ConvertSavingsPlansUtilization converts a GetSavingsPlansUtilization
// response into table or time-series frames, depending on query.Format. This
// API never groups, so there is no group column.
func ConvertSavingsPlansUtilization(
	query models.Query,
	output *awscostexplorer.GetSavingsPlansUtilizationOutput,
) (data.Frames, error) {
	if output == nil {
		return nil, fmt.Errorf("response from AWS Savings Plans Utilization is nil")
	}

	rows := make([]utilizationRow, 0, len(output.SavingsPlansUtilizationsByTime))
	for _, period := range output.SavingsPlansUtilizationsByTime {
		start, err := dateIntervalStartUTC(period.TimePeriod)
		if err != nil {
			return nil, err
		}
		metrics, err := savingsPlansMetrics(period.Utilization, period.AmortizedCommitment, period.Savings)
		if err != nil {
			return nil, err
		}
		rows = append(rows, utilizationRow{periodStart: start, metrics: metrics})
	}

	return utilizationFrames(query, "", rows, models.SavingsPlansUtilizationMetrics)
}

func reservationAggregateMetrics(aggregate *types.ReservationAggregates) (map[string]float64, error) {
	if aggregate == nil {
		return nil, fmt.Errorf("AWS Reservation Utilization response is missing aggregate totals")
	}
	return map[string]float64{
		"UtilizationPercentage":     parseAWSFloat(aggregate.UtilizationPercentage),
		"NetRISavings":              parseAWSFloat(aggregate.NetRISavings),
		"OnDemandCostOfRIHoursUsed": parseAWSFloat(aggregate.OnDemandCostOfRIHoursUsed),
		"TotalAmortizedFee":         parseAWSFloat(aggregate.TotalAmortizedFee),
		"UnusedHours":               parseAWSFloat(aggregate.UnusedHours),
		"RealizedSavings":           parseAWSFloat(aggregate.RealizedSavings),
		"UnrealizedSavings":         parseAWSFloat(aggregate.UnrealizedSavings),
	}, nil
}

func savingsPlansMetrics(
	utilization *types.SavingsPlansUtilization,
	amortized *types.SavingsPlansAmortizedCommitment,
	savings *types.SavingsPlansSavings,
) (map[string]float64, error) {
	if utilization == nil {
		return nil, fmt.Errorf("AWS Savings Plans Utilization response is missing utilization totals")
	}
	metrics := map[string]float64{
		"UtilizationPercentage": parseAWSFloat(utilization.UtilizationPercentage),
		"UsedCommitment":        parseAWSFloat(utilization.UsedCommitment),
		"UnusedCommitment":      parseAWSFloat(utilization.UnusedCommitment),
		"TotalCommitment":       parseAWSFloat(utilization.TotalCommitment),
	}
	if savings != nil {
		metrics["NetSavings"] = parseAWSFloat(savings.NetSavings)
		metrics["OnDemandCostEquivalent"] = parseAWSFloat(savings.OnDemandCostEquivalent)
	}
	if amortized != nil {
		metrics["TotalAmortizedCommitment"] = parseAWSFloat(amortized.TotalAmortizedCommitment)
	}
	return metrics, nil
}

// parseAWSFloat tolerates a missing or unparseable field as zero rather than
// failing the whole response: these curated fields are a small, optional
// subset of what AWS returns, not the one metric the user explicitly chose.
func parseAWSFloat(value *string) float64 {
	if value == nil {
		return 0
	}
	parsed, err := strconv.ParseFloat(*value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// parseAWSPercent is like parseAWSFloat, but first strips a trailing "%" --
// unlike Reservation/Savings Plans Utilization's UtilizationPercentage
// (a plain number string), AWS's own GetReservationCoverage API reference
// documents its percentage fields (e.g. CoverageHoursPercentage) with an
// embedded "%" suffix in example responses.
func parseAWSPercent(value *string) float64 {
	if value == nil {
		return 0
	}
	trimmed := strings.TrimSuffix(strings.TrimSpace(*value), "%")
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// utilizationFieldUnits is an explicit, exact lookup rather than a substring
// heuristic: OnDemandCostOfRIHoursUsed is a dollar amount despite its name
// containing "Hours", so matching on "contains Hours" misclassifies it as a
// duration. Every field absent from this map defaults to a dollar amount;
// neither API returns a currency code alongside these fields, so dollar
// amounts are assumed to be USD, matching Cost Explorer's typical behavior.
var utilizationFieldUnits = map[string]string{
	"UtilizationPercentage":   "percent",
	"UnusedHours":             "h",
	"CoveragePercentage":      "percent",
	"CoverageHoursPercentage": "percent",
	"OnDemandHours":           "h",
	"ReservedHours":           "h",
	"TotalRunningHours":       "h",
}

func utilizationFieldUnit(metric string) string {
	if unit, ok := utilizationFieldUnits[metric]; ok {
		return unit
	}
	return "currencyUSD"
}

func utilizationFrames(
	query models.Query,
	groupColumnName string,
	rows []utilizationRow,
	allowedMetrics []string,
) (data.Frames, error) {
	if query.Format == models.FormatTable {
		return data.Frames{utilizationTableFrame(query, groupColumnName, rows, allowedMetrics)}, nil
	}
	return utilizationTimeSeriesFrames(query, rows), nil
}

func utilizationTableFrame(
	query models.Query,
	groupColumnName string,
	rows []utilizationRow,
	allowedMetrics []string,
) *data.Frame {
	sort.SliceStable(rows, func(left, right int) bool {
		if !rows[left].periodStart.Equal(rows[right].periodStart) {
			return rows[left].periodStart.Before(rows[right].periodStart)
		}
		return rows[left].groupValue < rows[right].groupValue
	})

	periods := make([]string, 0, len(rows))
	groupValues := make([]string, 0, len(rows))
	metricValues := make(map[string][]float64, len(allowedMetrics))
	for _, metric := range allowedMetrics {
		metricValues[metric] = make([]float64, 0, len(rows))
	}
	for _, row := range rows {
		periods = append(periods, FormatBillingPeriodLabel(query.Granularity, row.periodStart))
		if groupColumnName != "" {
			groupValues = append(groupValues, row.groupValue)
		}
		for _, metric := range allowedMetrics {
			metricValues[metric] = append(metricValues[metric], row.metrics[metric])
		}
	}

	fields := []*data.Field{data.NewField("Period", nil, periods)}
	if groupColumnName != "" {
		fields = append(fields, data.NewField(groupColumnName, nil, groupValues))
	}
	for _, metric := range allowedMetrics {
		field := data.NewField(metric, nil, metricValues[metric])
		field.Config = &data.FieldConfig{Unit: utilizationFieldUnit(metric)}
		fields = append(fields, field)
	}

	return data.NewFrame("AWS Cost Explorer", fields...)
}

func utilizationTimeSeriesFrames(query models.Query, rows []utilizationRow) data.Frames {
	type series struct {
		times  []time.Time
		values []float64
	}

	byGroup := make(map[string]*series)
	order := make([]string, 0)
	for _, row := range rows {
		item, ok := byGroup[row.groupValue]
		if !ok {
			item = &series{}
			byGroup[row.groupValue] = item
			order = append(order, row.groupValue)
		}
		item.times = append(item.times, row.periodStart.UTC())
		item.values = append(item.values, row.metrics[query.UtilizationMetric])
	}
	sort.Strings(order)

	result := make(data.Frames, 0, max(1, len(order)))
	for _, group := range order {
		item := byGroup[group]
		displayName := query.UtilizationMetric
		labels := data.Labels{}
		if group != "" {
			displayName = group
			labels["subscriptionId"] = group
		}

		valueField := data.NewField(query.UtilizationMetric, labels, item.values)
		valueField.Config = &data.FieldConfig{
			DisplayNameFromDS: displayName,
			Unit:              utilizationFieldUnit(query.UtilizationMetric),
		}
		result = append(result, data.NewFrame(
			displayName,
			data.NewField("Time", nil, item.times),
			valueField,
		))
	}

	if len(result) == 0 {
		valueField := data.NewField(query.UtilizationMetric, data.Labels{}, []float64{})
		valueField.Config = &data.FieldConfig{DisplayNameFromDS: query.UtilizationMetric}
		result = append(result, data.NewFrame(
			query.UtilizationMetric,
			data.NewField("Time", nil, []time.Time{}),
			valueField,
		))
	}

	return result
}
