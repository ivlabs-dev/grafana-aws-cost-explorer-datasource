package models

import (
	"testing"
	"time"
)

func TestResolveCostExplorerDateRangeDailyIncompletePeriod(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	from := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	to := now

	excluded, err := ResolveCostExplorerDateRange(Query{
		Granularity: GranularityDaily,
	}, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, excluded, "2026-07-25", "2026-07-27", false)

	included, err := ResolveCostExplorerDateRange(Query{
		Granularity:             GranularityDaily,
		IncludeIncompletePeriod: true,
	}, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, included, "2026-07-25", "2026-07-28", true)
}

func TestResolveCostExplorerDateRangeDoesNotExcludeYesterday(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	got, err := ResolveCostExplorerDateRange(Query{
		Granularity: GranularityDaily,
	}, time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, got, "2026-07-26", "2026-07-27", false)
}

func TestResolveCostExplorerDateRangeMonthlyIncompletePeriod(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	from := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)
	to := now

	excluded, err := ResolveCostExplorerDateRange(Query{
		Granularity:            GranularityMonthly,
		AlignMonthlyToCalendar: boolValue(true),
	}, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, excluded, "2026-05-01", "2026-07-01", false)

	included, err := ResolveCostExplorerDateRange(Query{
		Granularity:             GranularityMonthly,
		AlignMonthlyToCalendar:  boolValue(true),
		IncludeIncompletePeriod: true,
	}, from, to, now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, included, "2026-05-01", "2026-08-01", true)
}

func TestResolveCostExplorerDateRangeOnlyCurrentPeriodIsEmpty(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	tests := []struct {
		name  string
		query Query
		from  time.Time
	}{
		{
			name:  "today",
			query: Query{Granularity: GranularityDaily},
			from:  time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "current month",
			query: Query{
				Granularity:            GranularityMonthly,
				AlignMonthlyToCalendar: boolValue(true),
			},
			from: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveCostExplorerDateRange(test.query, test.from, now, now)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Empty {
				t.Fatalf("got %+v, want an empty effective range", got)
			}
			if got.DateRange.Start != "" || got.DateRange.End != "" {
				t.Fatalf("empty range must not expose AWS request dates: %+v", got.DateRange)
			}
		})
	}
}

func TestResolveCostExplorerDateRangeMonthlyAlignment(t *testing.T) {
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		from      time.Time
		to        time.Time
		wantStart string
		wantEnd   string
	}{
		{
			name:      "year boundary",
			from:      time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC),
			to:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			wantStart: "2025-12-01",
			wantEnd:   "2026-02-01",
		},
		{
			name:      "leap year February",
			from:      time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC),
			to:        time.Date(2028, 2, 29, 18, 0, 0, 0, time.UTC),
			wantStart: "2028-02-01",
			wantEnd:   "2028-03-01",
		},
		{
			name:      "exclusive month boundary remains exclusive",
			from:      time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			to:        time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
			wantStart: "2026-05-01",
			wantEnd:   "2026-07-01",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveCostExplorerDateRange(Query{
				Granularity:             GranularityMonthly,
				AlignMonthlyToCalendar:  boolValue(true),
				IncludeIncompletePeriod: true,
			}, test.from, test.to, now)
			if err != nil {
				t.Fatal(err)
			}
			assertResolvedRange(t, got, test.wantStart, test.wantEnd, false)
		})
	}
}

func TestResolveCostExplorerDateRangeMonthlyAlignmentDisabledPreservesPartialDates(t *testing.T) {
	got, err := ResolveCostExplorerDateRange(Query{
		Granularity:             GranularityMonthly,
		AlignMonthlyToCalendar:  boolValue(false),
		IncludeIncompletePeriod: true,
	}, time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC), time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC), time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, got, "2026-04-17", "2026-06-13", false)
}

func TestResolveCostExplorerDateRangeExclusiveDailyEnd(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got, err := ResolveCostExplorerDateRange(
		Query{Granularity: GranularityDaily},
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, got, "2026-07-01", "2026-07-03", false)
}

func TestResolveCostExplorerDateRangeModes(t *testing.T) {
	now := time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC)
	from := time.Date(2026, 7, 20, 15, 30, 0, 0, time.UTC)

	monthToDate, err := ResolveCostExplorerDateRange(Query{
		Granularity: GranularityDaily,
		RangeMode:   RangeModeMonthToDate,
	}, from, now, now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, monthToDate, "2026-07-01", "2026-07-28", true)

	previous, err := ResolveCostExplorerDateRange(Query{
		Granularity: GranularityDaily,
		RangeMode:   RangeModePreviousEquivalentPeriod,
	}, from, now, now)
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, previous, "2026-07-13", "2026-07-20", false)
}

func TestResolveCostExplorerDateRangePreviousEquivalentCalendarMonths(t *testing.T) {
	got, err := ResolveCostExplorerDateRange(Query{
		Granularity:            GranularityMonthly,
		AlignMonthlyToCalendar: boolValue(true),
		RangeMode:              RangeModePreviousEquivalentPeriod,
	}, time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 27, 15, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedRange(t, got, "2026-01-01", "2026-04-01", false)
}

func TestResolveCostExplorerDateRangeRejectsInvalidDashboardRange(t *testing.T) {
	now := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	_, err := ResolveCostExplorerDateRange(Query{Granularity: GranularityDaily}, now, now, now)
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}

func assertResolvedRange(t *testing.T, got ResolvedDateRange, start, end string, incomplete bool) {
	t.Helper()
	if got.Empty {
		t.Fatalf("got empty range, want %s to %s", start, end)
	}
	if got.DateRange.Start != start || got.DateRange.End != end {
		t.Fatalf("got %+v, want start %s and exclusive end %s", got.DateRange, start, end)
	}
	if got.IncludesIncompletePeriod != incomplete {
		t.Fatalf("includes incomplete = %v, want %v", got.IncludesIncompletePeriod, incomplete)
	}
}
