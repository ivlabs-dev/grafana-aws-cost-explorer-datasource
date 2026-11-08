package frames

import (
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/inventory"
)

// alertValueField is the single numeric column of an alert frame. Grafana's
// numeric-long format turns every other (string) column into a label, so each
// row becomes its own alert instance.
const alertValueField = "days_until_expiry"

// ConvertReservationInventoryAlert converts a Reservation inventory list into
// a numeric-long frame for alert rules: one row per active reservation with a
// known end date, labelled by service, region, id and instance type, valued in
// fractional days until expiry (negative once past).
func ConvertReservationInventoryAlert(items []inventory.ReservedItem) data.Frames {
	service := []string{}
	region := []string{}
	id := []string{}
	instanceType := []string{}
	days := []float64{}

	for _, item := range items {
		if !alertEligible(item.State, item.End) {
			continue
		}
		service = append(service, item.Service)
		region = append(region, item.Region)
		id = append(id, item.ID)
		instanceType = append(instanceType, item.InstanceType)
		days = append(days, fractionalDaysUntil(item.End))
	}

	return data.Frames{numericLongFrame("Reservations expiry",
		data.NewField("service", nil, service),
		data.NewField("region", nil, region),
		data.NewField("id", nil, id),
		data.NewField("instance_type", nil, instanceType),
		data.NewField(alertValueField, nil, days),
	)}
}

// ConvertSavingsPlansInventoryAlert is the Savings Plans counterpart of
// ConvertReservationInventoryAlert, labelled by id, type, region and payment
// option.
func ConvertSavingsPlansInventoryAlert(plans []inventory.SavingsPlan) data.Frames {
	id := []string{}
	planType := []string{}
	region := []string{}
	paymentOption := []string{}
	days := []float64{}

	for _, plan := range plans {
		if !alertEligible(plan.State, plan.End) {
			continue
		}
		id = append(id, plan.ID)
		planType = append(planType, plan.Type)
		region = append(region, plan.Region)
		paymentOption = append(paymentOption, plan.PaymentOption)
		days = append(days, fractionalDaysUntil(plan.End))
	}

	return data.Frames{numericLongFrame("Savings Plans expiry",
		data.NewField("id", nil, id),
		data.NewField("type", nil, planType),
		data.NewField("region", nil, region),
		data.NewField("payment_option", nil, paymentOption),
		data.NewField(alertValueField, nil, days),
	)}
}

func numericLongFrame(name string, fields ...*data.Field) *data.Frame {
	frame := data.NewFrame(name, fields...)
	frame.Meta = &data.FrameMeta{
		Type:        data.FrameTypeNumericLong,
		TypeVersion: data.FrameTypeVersion{0, 1},
	}
	return frame
}

// alertEligible reports whether an item can be thresholded on: it must be
// active (retired and payment-failed items would otherwise alert forever) and
// have a known end date.
func alertEligible(state string, end time.Time) bool {
	return strings.EqualFold(state, "active") && !end.IsZero()
}

func fractionalDaysUntil(end time.Time) float64 {
	return time.Until(end).Hours() / 24
}
