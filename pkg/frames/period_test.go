package frames

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func TestBillingPeriodLabel(t *testing.T) {
	period := types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String("2026-07-19")},
	}

	daily, err := BillingPeriodLabel(models.GranularityDaily, period)
	if err != nil {
		t.Fatal(err)
	}
	if daily != "2026-07-19" {
		t.Fatalf("daily label = %q, want 2026-07-19", daily)
	}

	monthly, err := BillingPeriodLabel(models.GranularityMonthly, period)
	if err != nil {
		t.Fatal(err)
	}
	if monthly != "2026-07" {
		t.Fatalf("monthly label = %q, want 2026-07", monthly)
	}
}

func TestBillingPeriodStartUTC(t *testing.T) {
	got, err := BillingPeriodStartUTC(types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String("2026-07-19")},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("period start = %v (%v), want %v (UTC)", got, got.Location(), want)
	}
}

func TestBillingPeriodHelpersRejectMissingOrInvalidStart(t *testing.T) {
	tests := []types.ResultByTime{
		{},
		{TimePeriod: &types.DateInterval{Start: aws.String("not-a-date")}},
	}
	for _, period := range tests {
		if _, err := BillingPeriodStartUTC(period); err == nil {
			t.Fatal("expected invalid period error")
		}
	}
}

func TestBillingPeriodLabelRejectsUnsupportedGranularity(t *testing.T) {
	_, err := BillingPeriodLabel("HOURLY", types.ResultByTime{
		TimePeriod: &types.DateInterval{Start: aws.String("2026-07-19")},
	})
	if err == nil {
		t.Fatal("expected unsupported granularity error")
	}
}
