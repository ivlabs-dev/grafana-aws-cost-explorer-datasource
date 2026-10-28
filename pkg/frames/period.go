package frames

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// BillingPeriodStartUTC returns the beginning of an AWS billing period. Time
// series use this UTC timestamp directly; Grafana applies display timezone
// settings when rendering it.
func BillingPeriodStartUTC(period types.ResultByTime) (time.Time, error) {
	return dateIntervalStartUTC(period.TimePeriod)
}

// dateIntervalStartUTC is the shared parser behind BillingPeriodStartUTC; the
// Reservation/Savings Plans utilization responses carry a *types.DateInterval
// directly rather than wrapping it in types.ResultByTime.
func dateIntervalStartUTC(interval *types.DateInterval) (time.Time, error) {
	if interval == nil || interval.Start == nil {
		return time.Time{}, fmt.Errorf("response time period from AWS Cost Explorer is missing a start date")
	}
	value, err := time.Parse(time.DateOnly, *interval.Start)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Cost Explorer period start %q: %w", *interval.Start, err)
	}
	return value.UTC(), nil
}

// BillingPeriodLabel returns a timezone-independent label suitable for the
// primary Period column in table frames.
func BillingPeriodLabel(granularity string, period types.ResultByTime) (string, error) {
	start, err := BillingPeriodStartUTC(period)
	if err != nil {
		return "", err
	}
	if granularity != models.GranularityDaily && granularity != models.GranularityMonthly {
		return "", fmt.Errorf("format billing period for unsupported granularity %q", granularity)
	}
	return FormatBillingPeriodLabel(granularity, start), nil
}

func FormatBillingPeriodLabel(granularity string, start time.Time) string {
	switch granularity {
	case models.GranularityDaily:
		return start.UTC().Format(time.DateOnly)
	case models.GranularityMonthly:
		return start.UTC().Format("2006-01")
	default:
		return ""
	}
}
