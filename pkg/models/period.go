package models

import (
	"fmt"
	"time"
)

const NoCompletedPeriodsNotice = "The selected time range contains no completed AWS billing periods."

// ResolvedDateRange describes the effective Cost Explorer interval after
// applying billing-period semantics. DateRange is empty when Empty is true so
// callers cannot accidentally send an invalid start/end pair to AWS.
type ResolvedDateRange struct {
	DateRange                DateRange
	Empty                    bool
	IncludesIncompletePeriod bool
}

// ResolveCostExplorerDateRange converts a dashboard range to AWS's
// inclusive-start/exclusive-end dates. now is injected to keep the definition
// of the current UTC billing period deterministic in tests.
func ResolveCostExplorerDateRange(query Query, from, to, now time.Time) (ResolvedDateRange, error) {
	if !to.After(from) {
		return ResolvedDateRange{}, fmt.Errorf("dashboard time range end must be after start")
	}

	rangeMode := query.RangeMode
	if rangeMode == "" {
		rangeMode = RangeModeDashboard
	}

	if rangeMode == RangeModePreviousEquivalentPeriod {
		currentQuery := query
		currentQuery.RangeMode = RangeModeDashboard
		current, err := ResolveCostExplorerDateRange(currentQuery, from, to, now)
		if err != nil || current.Empty {
			return current, err
		}
		return previousEquivalentRange(query, current, now)
	}

	includeIncomplete := query.IncludeIncompletePeriod
	effectiveFrom := from
	effectiveTo := to
	if rangeMode == RangeModeMonthToDate {
		anchor := to.UTC()
		effectiveFrom = firstUTCMonth(anchor)
		// Month-to-date is deliberately an incomplete-period query. The
		// dashboard panel using this mode documents AWS's reporting delay.
		includeIncomplete = true
	}

	start, end, err := periodBoundaries(query, effectiveFrom, effectiveTo)
	if err != nil {
		return ResolvedDateRange{}, err
	}

	if !includeIncomplete {
		switch query.Granularity {
		case GranularityDaily:
			end = earlierTime(end, utcDay(now))
		case GranularityMonthly:
			end = earlierTime(end, firstUTCMonth(now))
		}
	}

	if !end.After(start) {
		return ResolvedDateRange{Empty: true}, nil
	}

	return ResolvedDateRange{
		DateRange: DateRange{
			Start: start.Format(time.DateOnly),
			End:   end.Format(time.DateOnly),
		},
		IncludesIncompletePeriod: includeIncomplete && overlapsCurrentPeriod(query.Granularity, start, end, now),
	}, nil
}

func periodBoundaries(query Query, from, to time.Time) (time.Time, time.Time, error) {
	if query.Granularity == GranularityMonthly && query.AlignMonthlyToCalendarValue() {
		start := firstUTCMonth(from)
		end := firstUTCMonth(to)
		if !to.UTC().Equal(end) {
			end = end.AddDate(0, 1, 0)
		}
		return start, end, nil
	}

	dateRange, err := CostExplorerDateRange(from, to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start, err := time.Parse(time.DateOnly, dateRange.Start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse effective Cost Explorer start date: %w", err)
	}
	end, err := time.Parse(time.DateOnly, dateRange.End)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse effective Cost Explorer end date: %w", err)
	}
	return start.UTC(), end.UTC(), nil
}

func previousEquivalentRange(query Query, current ResolvedDateRange, now time.Time) (ResolvedDateRange, error) {
	start, err := time.Parse(time.DateOnly, current.DateRange.Start)
	if err != nil {
		return ResolvedDateRange{}, fmt.Errorf("parse current Cost Explorer start date: %w", err)
	}
	end, err := time.Parse(time.DateOnly, current.DateRange.End)
	if err != nil {
		return ResolvedDateRange{}, fmt.Errorf("parse current Cost Explorer end date: %w", err)
	}

	previousEnd := start
	var previousStart time.Time
	if query.Granularity == GranularityMonthly && query.AlignMonthlyToCalendarValue() {
		months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
		previousStart = start.AddDate(0, -months, 0)
	} else {
		previousStart = start.Add(-end.Sub(start))
	}

	return ResolvedDateRange{
		DateRange: DateRange{
			Start: previousStart.Format(time.DateOnly),
			End:   previousEnd.Format(time.DateOnly),
		},
		IncludesIncompletePeriod: overlapsCurrentPeriod(
			query.Granularity,
			previousStart,
			previousEnd,
			now,
		),
	}, nil
}

func overlapsCurrentPeriod(granularity string, start, end, now time.Time) bool {
	currentStart := utcDay(now)
	var currentEnd time.Time
	if granularity == GranularityMonthly {
		currentStart = firstUTCMonth(now)
		currentEnd = currentStart.AddDate(0, 1, 0)
	} else {
		currentEnd = currentStart.AddDate(0, 0, 1)
	}
	return start.Before(currentEnd) && end.After(currentStart)
}

func firstUTCMonth(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func earlierTime(left, right time.Time) time.Time {
	if right.Before(left) {
		return right
	}
	return left
}
