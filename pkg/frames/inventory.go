package frames

import (
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/inventory"
)

// ConvertReservationInventory converts a merged Reservation inventory list
// into a single table frame, one row per Reservation across all five
// reservation-eligible services, sorted by DaysUntilExpiry ascending so
// anything soonest to expire surfaces first.
func ConvertReservationInventory(items []inventory.ReservedItem, notices []data.Notice) data.Frames {
	sorted := make([]inventory.ReservedItem, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(left, right int) bool {
		return daysUntil(sorted[left].End) < daysUntil(sorted[right].End)
	})

	service := make([]string, 0, len(sorted))
	id := make([]string, 0, len(sorted))
	instanceType := make([]string, 0, len(sorted))
	count := make([]int32, 0, len(sorted))
	offeringType := make([]string, 0, len(sorted))
	state := make([]string, 0, len(sorted))
	start := make([]time.Time, 0, len(sorted))
	end := make([]time.Time, 0, len(sorted))
	daysUntilExpiry := make([]int64, 0, len(sorted))

	for _, item := range sorted {
		service = append(service, item.Service)
		id = append(id, item.ID)
		instanceType = append(instanceType, item.InstanceType)
		count = append(count, item.Count)
		offeringType = append(offeringType, item.OfferingType)
		state = append(state, item.State)
		start = append(start, item.Start.UTC())
		end = append(end, item.End.UTC())
		daysUntilExpiry = append(daysUntilExpiry, daysUntil(item.End))
	}

	frame := data.NewFrame("Reservations",
		data.NewField("Service", nil, service),
		data.NewField("Reservation ID", nil, id),
		data.NewField("Instance type", nil, instanceType),
		data.NewField("Count", nil, count),
		data.NewField("Offering type", nil, offeringType),
		data.NewField("State", nil, state),
		data.NewField("Start", nil, start),
		data.NewField("End", nil, end),
		data.NewField("Days until expiry", nil, daysUntilExpiry),
	)
	if len(notices) > 0 {
		frame.Meta = &data.FrameMeta{Notices: notices}
	}
	return data.Frames{frame}
}

// ConvertSavingsPlansInventory converts a Savings Plans inventory list into
// a single table frame, sorted by DaysUntilExpiry ascending.
func ConvertSavingsPlansInventory(plans []inventory.SavingsPlan) data.Frames {
	sorted := make([]inventory.SavingsPlan, len(plans))
	copy(sorted, plans)
	sort.SliceStable(sorted, func(left, right int) bool {
		return daysUntil(sorted[left].End) < daysUntil(sorted[right].End)
	})

	id := make([]string, 0, len(sorted))
	planType := make([]string, 0, len(sorted))
	paymentOption := make([]string, 0, len(sorted))
	region := make([]string, 0, len(sorted))
	commitment := make([]float64, 0, len(sorted))
	state := make([]string, 0, len(sorted))
	start := make([]time.Time, 0, len(sorted))
	end := make([]time.Time, 0, len(sorted))
	daysUntilExpiry := make([]int64, 0, len(sorted))

	for _, plan := range sorted {
		id = append(id, plan.ID)
		planType = append(planType, plan.Type)
		paymentOption = append(paymentOption, plan.PaymentOption)
		region = append(region, plan.Region)
		commitment = append(commitment, plan.Commitment)
		state = append(state, plan.State)
		start = append(start, plan.Start.UTC())
		end = append(end, plan.End.UTC())
		daysUntilExpiry = append(daysUntilExpiry, daysUntil(plan.End))
	}

	commitmentField := data.NewField("Commitment", nil, commitment)
	commitmentField.Config = &data.FieldConfig{Unit: "currencyUSD"}

	return data.Frames{data.NewFrame("Savings Plans",
		data.NewField("Savings Plan ID", nil, id),
		data.NewField("Type", nil, planType),
		data.NewField("Payment option", nil, paymentOption),
		data.NewField("Region", nil, region),
		commitmentField,
		data.NewField("State", nil, state),
		data.NewField("Start", nil, start),
		data.NewField("End", nil, end),
		data.NewField("Days until expiry", nil, daysUntilExpiry),
	)}
}

// daysUntil reports whole days between now and the given time, negative
// when it's already in the past -- a zero/unknown end (e.g. an item whose
// expiry couldn't be computed) sorts last by reporting a very large value.
func daysUntil(end time.Time) int64 {
	if end.IsZero() {
		return 1 << 32
	}
	return int64(time.Until(end).Hours() / 24)
}
