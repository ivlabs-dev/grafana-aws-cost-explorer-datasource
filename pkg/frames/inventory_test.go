package frames

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/inventory"
)

func TestConvertReservationInventorySortsByExpiry(t *testing.T) {
	now := time.Now().UTC()
	items := []inventory.ReservedItem{
		{Service: "RDS", ID: "far", State: "active", Start: now, End: now.Add(300 * 24 * time.Hour)},
		{Service: "EC2", ID: "soon", State: "active", Start: now, End: now.Add(5 * 24 * time.Hour)},
		{Service: "Redshift", ID: "expired", State: "retired", Start: now.Add(-800 * 24 * time.Hour), End: now.Add(-10 * 24 * time.Hour)},
	}

	frames := ConvertReservationInventory(items, nil)
	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	idField := fieldByName(t, frames[0], "Reservation ID")
	got := []string{idField.At(0).(string), idField.At(1).(string), idField.At(2).(string)}
	want := []string{"expired", "soon", "far"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", got, want)
		}
	}

	daysField := fieldByName(t, frames[0], "Days until expiry")
	if daysField.At(0).(int64) >= 0 {
		t.Fatalf("expired item should have a negative days-until-expiry, got %v", daysField.At(0))
	}
}

func TestConvertReservationInventoryAttachesNotices(t *testing.T) {
	notices := []data.Notice{{Severity: data.NoticeSeverityWarning, Text: "ElastiCache failed"}}
	frames := ConvertReservationInventory(nil, notices)
	if frames[0].Meta == nil || len(frames[0].Meta.Notices) != 1 {
		t.Fatalf("expected the notice to be attached, got %+v", frames[0].Meta)
	}
}

func TestConvertSavingsPlansInventoryTableShape(t *testing.T) {
	now := time.Now().UTC()
	plans := []inventory.SavingsPlan{
		{ID: "sp-1", Type: "Compute", PaymentOption: "All Upfront", Region: "us-east-1", Commitment: 10.5, State: "active", Start: now, End: now.Add(365 * 24 * time.Hour)},
	}

	frames := ConvertSavingsPlansInventory(plans)
	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	frame := frames[0]
	if fieldByName(t, frame, "Savings Plan ID").At(0).(string) != "sp-1" {
		t.Fatalf("unexpected Savings Plan ID field")
	}
	commitmentField := fieldByName(t, frame, "Commitment")
	if commitmentField.Config == nil || commitmentField.Config.Unit != "currencyUSD" {
		t.Fatalf("Commitment field should have a currencyUSD unit, got %+v", commitmentField.Config)
	}
}
