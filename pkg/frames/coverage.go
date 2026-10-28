package frames

import (
	"fmt"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// ConvertReservationCoverage converts a GetReservationCoverage response into
// table or time-series frames, depending on query.Format. This plugin
// doesn't request grouping for this query type, so every row uses the
// period's Total (AWS's per-period aggregate), the same way
// ConvertReservationUtilization treats an ungrouped period -- there is no
// group column.
func ConvertReservationCoverage(
	query models.Query,
	output *awscostexplorer.GetReservationCoverageOutput,
) (data.Frames, error) {
	if output == nil {
		return nil, fmt.Errorf("response from AWS Reservation Coverage is nil")
	}

	rows := make([]utilizationRow, 0, len(output.CoveragesByTime))
	for _, period := range output.CoveragesByTime {
		start, err := dateIntervalStartUTC(period.TimePeriod)
		if err != nil {
			return nil, err
		}
		metrics, err := reservationCoverageMetrics(period.Total)
		if err != nil {
			return nil, err
		}
		rows = append(rows, utilizationRow{periodStart: start, metrics: metrics})
	}

	return utilizationFrames(query, "", rows, models.ReservationCoverageMetrics)
}

// ConvertSavingsPlansCoverage converts a GetSavingsPlansCoverage response
// into table or time-series frames, depending on query.Format. This plugin
// doesn't request grouping for this query type, so there is no group
// column.
func ConvertSavingsPlansCoverage(
	query models.Query,
	output *awscostexplorer.GetSavingsPlansCoverageOutput,
) (data.Frames, error) {
	if output == nil {
		return nil, fmt.Errorf("response from AWS Savings Plans Coverage is nil")
	}

	rows := make([]utilizationRow, 0, len(output.SavingsPlansCoverages))
	for _, row := range output.SavingsPlansCoverages {
		start, err := dateIntervalStartUTC(row.TimePeriod)
		if err != nil {
			return nil, err
		}
		metrics, err := savingsPlansCoverageMetrics(row.Coverage)
		if err != nil {
			return nil, err
		}
		rows = append(rows, utilizationRow{periodStart: start, metrics: metrics})
	}

	return utilizationFrames(query, "", rows, models.SavingsPlansCoverageMetrics)
}

func reservationCoverageMetrics(coverage *types.Coverage) (map[string]float64, error) {
	if coverage == nil {
		return nil, fmt.Errorf("AWS Reservation Coverage response is missing aggregate totals")
	}
	metrics := map[string]float64{}
	if coverage.CoverageHours != nil {
		metrics["CoverageHoursPercentage"] = parseAWSPercent(coverage.CoverageHours.CoverageHoursPercentage)
		metrics["OnDemandHours"] = parseAWSFloat(coverage.CoverageHours.OnDemandHours)
		metrics["ReservedHours"] = parseAWSFloat(coverage.CoverageHours.ReservedHours)
		metrics["TotalRunningHours"] = parseAWSFloat(coverage.CoverageHours.TotalRunningHours)
	}
	if coverage.CoverageCost != nil {
		metrics["OnDemandCost"] = parseAWSFloat(coverage.CoverageCost.OnDemandCost)
	}
	return metrics, nil
}

func savingsPlansCoverageMetrics(coverage *types.SavingsPlansCoverageData) (map[string]float64, error) {
	if coverage == nil {
		return nil, fmt.Errorf("AWS Savings Plans Coverage response is missing coverage data")
	}
	return map[string]float64{
		"CoveragePercentage":         parseAWSPercent(coverage.CoveragePercentage),
		"SpendCoveredBySavingsPlans": parseAWSFloat(coverage.SpendCoveredBySavingsPlans),
		"OnDemandCost":               parseAWSFloat(coverage.OnDemandCost),
		"TotalCost":                  parseAWSFloat(coverage.TotalCost),
	}, nil
}
