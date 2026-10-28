package frames

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func reservationUtilizationQuery(format string) models.Query {
	q := models.Query{
		QueryType:   models.QueryTypeReservationUtilization,
		Granularity: models.GranularityDaily,
		Format:      format,
	}
	q.ApplyDefaults()
	return q
}

func TestConvertReservationUtilizationTable(t *testing.T) {
	query := reservationUtilizationQuery(models.FormatTable)
	output := &awscostexplorer.GetReservationUtilizationOutput{
		UtilizationsByTime: []types.UtilizationByTime{
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
				Total: &types.ReservationAggregates{
					UtilizationPercentage: aws.String("80.5"),
					NetRISavings:          aws.String("12.34"),
				},
			},
		},
	}

	result, err := ConvertReservationUtilization(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 1 {
		t.Fatalf("frames = %d, rows = %d", len(result), result[0].Rows())
	}
	if got := result[0].Fields[0].At(0).(string); got != "2026-07-01" {
		t.Fatalf("period = %q", got)
	}

	utilizationField := fieldByName(t, result[0], "UtilizationPercentage")
	if got := utilizationField.At(0).(float64); got != 80.5 {
		t.Fatalf("UtilizationPercentage = %v, want 80.5", got)
	}
	if got := utilizationField.Config.Unit; got != "percent" {
		t.Fatalf("unit = %q, want percent", got)
	}

	savingsField := fieldByName(t, result[0], "NetRISavings")
	if got := savingsField.Config.Unit; got != "currencyUSD" {
		t.Fatalf("unit = %q, want currencyUSD", got)
	}
}

func TestUtilizationFieldUnitsAreExactNotSubstringMatched(t *testing.T) {
	// OnDemandCostOfRIHoursUsed is a dollar amount despite its name
	// containing "Hours" -- a substring-based unit heuristic previously
	// misclassified it as an hours duration. UnusedHours is the only
	// curated field that actually is a duration.
	if got := utilizationFieldUnit("OnDemandCostOfRIHoursUsed"); got != "currencyUSD" {
		t.Fatalf("unit = %q, want currencyUSD", got)
	}
	if got := utilizationFieldUnit("UnusedHours"); got != "h" {
		t.Fatalf("unit = %q, want h", got)
	}
	if got := utilizationFieldUnit("UtilizationPercentage"); got != "percent" {
		t.Fatalf("unit = %q, want percent", got)
	}
	if got := utilizationFieldUnit("NetRISavings"); got != "currencyUSD" {
		t.Fatalf("unit = %q, want currencyUSD", got)
	}
}

func TestConvertReservationUtilizationGroupedBySubscriptionTable(t *testing.T) {
	query := reservationUtilizationQuery(models.FormatTable)
	query.GroupBySubscription = true
	output := &awscostexplorer.GetReservationUtilizationOutput{
		UtilizationsByTime: []types.UtilizationByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Groups: []types.ReservationUtilizationGroup{
				{Value: aws.String("sub-1"), Utilization: &types.ReservationAggregates{UtilizationPercentage: aws.String("70")}},
				{Value: aws.String("sub-2"), Utilization: &types.ReservationAggregates{UtilizationPercentage: aws.String("90")}},
			},
		}},
	}

	result, err := ConvertReservationUtilization(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Rows() != 2 {
		t.Fatalf("rows = %d, want 2", result[0].Rows())
	}
	subscriptionField := fieldByName(t, result[0], "Subscription ID")
	if got := subscriptionField.At(0).(string); got != "sub-1" {
		t.Fatalf("first subscription = %q, want sub-1", got)
	}
}

func TestConvertReservationUtilizationTimeSeries(t *testing.T) {
	query := reservationUtilizationQuery(models.FormatTimeSeries)
	output := &awscostexplorer.GetReservationUtilizationOutput{
		UtilizationsByTime: []types.UtilizationByTime{
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
				Total:      &types.ReservationAggregates{UtilizationPercentage: aws.String("80")},
			},
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-02")},
				Total:      &types.ReservationAggregates{UtilizationPercentage: aws.String("85")},
			},
		},
	}

	result, err := ConvertReservationUtilization(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 2 {
		t.Fatalf("frames = %d, rows = %d", len(result), result[0].Rows())
	}
	if got := result[0].Fields[1].At(1).(float64); got != 85 {
		t.Fatalf("second point = %v, want 85", got)
	}
}

func TestConvertSavingsPlansUtilizationTable(t *testing.T) {
	query := models.Query{QueryType: models.QueryTypeSavingsPlansUtilization, Granularity: models.GranularityDaily, Format: models.FormatTable}
	query.ApplyDefaults()
	output := &awscostexplorer.GetSavingsPlansUtilizationOutput{
		SavingsPlansUtilizationsByTime: []types.SavingsPlansUtilizationByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Utilization: &types.SavingsPlansUtilization{
				UtilizationPercentage: aws.String("95"),
				UsedCommitment:        aws.String("10.00"),
			},
			Savings: &types.SavingsPlansSavings{NetSavings: aws.String("3.00")},
			AmortizedCommitment: &types.SavingsPlansAmortizedCommitment{
				TotalAmortizedCommitment: aws.String("13.00"),
			},
		}},
	}

	result, err := ConvertSavingsPlansUtilization(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if result[0].Rows() != 1 {
		t.Fatalf("rows = %d, want 1", result[0].Rows())
	}
	if got := fieldByName(t, result[0], "NetSavings").At(0).(float64); got != 3.0 {
		t.Fatalf("NetSavings = %v, want 3.0", got)
	}
	if got := fieldByName(t, result[0], "TotalAmortizedCommitment").At(0).(float64); got != 13.0 {
		t.Fatalf("TotalAmortizedCommitment = %v, want 13.0", got)
	}
	// Savings Plans utilization never groups: no group column is present.
	for _, field := range result[0].Fields {
		if field.Name == "Subscription ID" {
			t.Fatal("unexpected group column for Savings Plans utilization")
		}
	}
}

func fieldByName(t *testing.T, frame *data.Frame, name string) *data.Field {
	t.Helper()
	for _, field := range frame.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("field %q not found in frame %q", name, frame.Name)
	return nil
}
