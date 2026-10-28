package inventory

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
)

const redshiftMaxPages = 100

// fetchRedshift lists reserved nodes. Like RDS and ElastiCache, Redshift's
// Describe call has no direct expiry field -- only a start time and a
// duration in seconds -- so End is computed via endFromDuration.
func (s *Service) fetchRedshift(ctx context.Context) ([]ReservedItem, error) {
	if s.redshift == nil {
		return nil, nil
	}

	var items []ReservedItem
	var marker *string
	for page := 1; page <= redshiftMaxPages; page++ {
		output, err := s.redshift.DescribeReservedNodes(ctx, &redshift.DescribeReservedNodesInput{Marker: marker})
		if err != nil {
			return nil, err
		}
		for _, reservation := range output.ReservedNodes {
			items = append(items, redshiftReservedItem(reservation))
		}
		if output.Marker == nil || *output.Marker == "" {
			return items, nil
		}
		marker = output.Marker
	}
	return nil, fmt.Errorf("%s reservation inventory exceeded the %d-page safety limit", ServiceRedshift, redshiftMaxPages)
}

func redshiftReservedItem(reservation types.ReservedNode) ReservedItem {
	var count int32
	if reservation.NodeCount != nil {
		count = *reservation.NodeCount
	}
	var duration int64
	if reservation.Duration != nil {
		duration = int64(*reservation.Duration)
	}
	start := timeValue(reservation.StartTime)
	return ReservedItem{
		Service:      ServiceRedshift,
		ID:           stringValue(reservation.ReservedNodeId),
		InstanceType: stringValue(reservation.NodeType),
		Count:        count,
		OfferingType: stringValue(reservation.OfferingType),
		State:        stringValue(reservation.State),
		Start:        start,
		End:          endFromDuration(start, duration),
	}
}
