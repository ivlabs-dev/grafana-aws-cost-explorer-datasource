package frames

import (
	"math"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/inventory"
)

func assertNumericLong(t *testing.T, frame *data.Frame) {
	t.Helper()
	if frame.Meta == nil || frame.Meta.Type != data.FrameTypeNumericLong {
		t.Fatalf("frame meta type = %+v, want %s", frame.Meta, data.FrameTypeNumericLong)
	}
}

func fieldNames(frame *data.Frame) []string {
	names := make([]string, 0, len(frame.Fields))
	for _, field := range frame.Fields {
		names = append(names, field.Name)
	}
	return names
}

func assertFieldNames(t *testing.T, frame *data.Frame, want []string) {
	t.Helper()
	got := fieldNames(frame)
	if len(got) != len(want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fields = %v, want %v", got, want)
		}
	}
}

func assertDays(t *testing.T, frame *data.Frame, row int, want float64) {
	t.Helper()
	got := fieldByName(t, frame, "days_until_expiry").At(row).(float64)
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("days_until_expiry[%d] = %v, want about %v", row, got, want)
	}
}

func TestReservationAlertFrameShape(t *testing.T) {
	now := time.Now()
	items := []inventory.ReservedItem{
		{Service: "EC2", Region: "us-east-1", ID: "ri-1", InstanceType: "m5.large", Count: 4, State: "active", End: now.Add(10 * 24 * time.Hour)},
		{Service: "RDS", Region: "eu-west-1", ID: "ri-2", InstanceType: "db.r5.large", State: "active", End: now.Add(40 * 24 * time.Hour)},
	}

	frames := ConvertReservationInventoryAlert(items)
	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	frame := frames[0]
	assertNumericLong(t, frame)
	assertFieldNames(t, frame, []string{"service", "region", "id", "instance_type", "days_until_expiry"})
	if frame.Rows() != 2 {
		t.Fatalf("rows = %d, want 2", frame.Rows())
	}
	if got := fieldByName(t, frame, "id").At(1).(string); got != "ri-2" {
		t.Fatalf("id[1] = %q, want ri-2", got)
	}
	if got := fieldByName(t, frame, "service").At(0).(string); got != "EC2" {
		t.Fatalf("service[0] = %q, want EC2", got)
	}
	assertDays(t, frame, 0, 10)
	assertDays(t, frame, 1, 40)
}

func TestReservationAlertFrameValueIsFractionalDays(t *testing.T) {
	items := []inventory.ReservedItem{{ID: "ri-1", State: "active", End: time.Now().Add(36 * time.Hour)}}
	assertDays(t, ConvertReservationInventoryAlert(items)[0], 0, 1.5)
}

func TestReservationAlertFrameExcludesNonActiveAndUnknownEnd(t *testing.T) {
	end := time.Now().Add(5 * 24 * time.Hour)
	items := []inventory.ReservedItem{
		{ID: "retired", State: "retired", End: end},
		{ID: "failed", State: "payment-failed", End: end},
		{ID: "pending", State: "payment-pending", End: end},
		{ID: "unknown-end", State: "active"},
		{ID: "keep", State: "active", End: end},
	}

	frame := ConvertReservationInventoryAlert(items)[0]
	if frame.Rows() != 1 {
		t.Fatalf("rows = %d, want 1", frame.Rows())
	}
	if got := fieldByName(t, frame, "id").At(0).(string); got != "keep" {
		t.Fatalf("remaining id = %q, want keep", got)
	}
}

func TestReservationAlertFrameStateMatchIsCaseInsensitive(t *testing.T) {
	end := time.Now().Add(24 * time.Hour)
	items := []inventory.ReservedItem{
		{ID: "a", State: "Active", End: end},
		{ID: "b", State: "ACTIVE", End: end},
	}
	if rows := ConvertReservationInventoryAlert(items)[0].Rows(); rows != 2 {
		t.Fatalf("rows = %d, want 2", rows)
	}
}

func TestReservationAlertFrameKeepsExpiredActiveItem(t *testing.T) {
	items := []inventory.ReservedItem{{ID: "late", State: "active", End: time.Now().Add(-48 * time.Hour)}}
	assertDays(t, ConvertReservationInventoryAlert(items)[0], 0, -2)
}

func TestReservationAlertFrameEmptyInputIsTypedZeroRows(t *testing.T) {
	frames := ConvertReservationInventoryAlert(nil)
	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	assertNumericLong(t, frames[0])
	assertFieldNames(t, frames[0], []string{"service", "region", "id", "instance_type", "days_until_expiry"})
	if frames[0].Rows() != 0 {
		t.Fatalf("rows = %d, want 0", frames[0].Rows())
	}
}

func TestSavingsPlansAlertFrameShapeAndFiltering(t *testing.T) {
	now := time.Now()
	plans := []inventory.SavingsPlan{
		{ID: "sp-compute", Type: "Compute", PaymentOption: "No Upfront", Region: "", State: "active", End: now.Add(12 * 24 * time.Hour)},
		{ID: "sp-ec2", Type: "EC2Instance", PaymentOption: "Partial Upfront", Region: "us-east-1", State: "active", End: now.Add(90 * 24 * time.Hour)},
		{ID: "sp-retired", State: "retired", End: now.Add(-30 * 24 * time.Hour)},
		{ID: "sp-unknown", State: "active"},
	}

	frames := ConvertSavingsPlansInventoryAlert(plans)
	if len(frames) != 1 {
		t.Fatalf("frame count = %d, want 1", len(frames))
	}
	frame := frames[0]
	assertNumericLong(t, frame)
	assertFieldNames(t, frame, []string{"id", "type", "region", "payment_option", "days_until_expiry"})
	if frame.Rows() != 2 {
		t.Fatalf("rows = %d, want 2", frame.Rows())
	}
	if got := fieldByName(t, frame, "region").At(0).(string); got != "" {
		t.Fatalf("empty-region plan label = %q, want empty string", got)
	}
	if got := fieldByName(t, frame, "payment_option").At(1).(string); got != "Partial Upfront" {
		t.Fatalf("payment_option[1] = %q", got)
	}
	assertDays(t, frame, 0, 12)
	assertDays(t, frame, 1, 90)
}

func TestSavingsPlansAlertFrameEmptyInputIsTypedZeroRows(t *testing.T) {
	frame := ConvertSavingsPlansInventoryAlert(nil)[0]
	assertNumericLong(t, frame)
	assertFieldNames(t, frame, []string{"id", "type", "region", "payment_option", "days_until_expiry"})
	if frame.Rows() != 0 {
		t.Fatalf("rows = %d, want 0", frame.Rows())
	}
}
