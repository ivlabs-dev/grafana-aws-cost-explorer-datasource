package inventory

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
)

const openSearchMaxPages = 100

// fetchOpenSearch lists reserved instances. OpenSearch has no OfferingType
// concept -- PaymentOption is the closest equivalent, so it's mapped into
// ReservedItem.OfferingType for a consistent column across all five
// services.
func (s *Service) fetchOpenSearch(ctx context.Context) ([]ReservedItem, error) {
	if s.openSearch == nil {
		return nil, nil
	}

	var items []ReservedItem
	var token *string
	for page := 1; page <= openSearchMaxPages; page++ {
		output, err := s.openSearch.DescribeReservedInstances(ctx, &opensearch.DescribeReservedInstancesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, reservation := range output.ReservedInstances {
			items = append(items, openSearchReservedItem(reservation))
		}
		if output.NextToken == nil || *output.NextToken == "" {
			return items, nil
		}
		token = output.NextToken
	}
	return nil, fmt.Errorf("OpenSearch reservation inventory exceeded the %d-page safety limit", openSearchMaxPages)
}

func openSearchReservedItem(reservation types.ReservedInstance) ReservedItem {
	start := timeValue(reservation.StartTime)
	return ReservedItem{
		Service:      ServiceOpenSearch,
		ID:           stringValue(reservation.ReservedInstanceId),
		InstanceType: string(reservation.InstanceType),
		Count:        reservation.InstanceCount,
		OfferingType: string(reservation.PaymentOption),
		State:        stringValue(reservation.State),
		Start:        start,
		End:          endFromDuration(start, int64(reservation.Duration)),
	}
}
