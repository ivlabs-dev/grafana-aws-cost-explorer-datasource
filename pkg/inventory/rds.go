package inventory

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

const rdsMaxPages = 100

// fetchRDS lists reserved DB instances. RDS's Describe call has no direct
// expiry field -- only a start time and a duration in seconds -- so End is
// computed via endFromDuration.
func (s *Service) fetchRDS(ctx context.Context) ([]ReservedItem, error) {
	if s.rds == nil {
		return nil, nil
	}

	var items []ReservedItem
	var marker *string
	for page := 1; page <= rdsMaxPages; page++ {
		output, err := s.rds.DescribeReservedDBInstances(ctx, &rds.DescribeReservedDBInstancesInput{Marker: marker})
		if err != nil {
			return nil, err
		}
		for _, reservation := range output.ReservedDBInstances {
			items = append(items, rdsReservedItem(reservation))
		}
		if output.Marker == nil || *output.Marker == "" {
			return items, nil
		}
		marker = output.Marker
	}
	return nil, fmt.Errorf("%s reservation inventory exceeded the %d-page safety limit", ServiceRDS, rdsMaxPages)
}

func rdsReservedItem(reservation types.ReservedDBInstance) ReservedItem {
	var count int32
	if reservation.DBInstanceCount != nil {
		count = *reservation.DBInstanceCount
	}
	var duration int64
	if reservation.Duration != nil {
		duration = int64(*reservation.Duration)
	}
	start := timeValue(reservation.StartTime)
	return ReservedItem{
		Service:      ServiceRDS,
		ID:           stringValue(reservation.ReservedDBInstanceId),
		InstanceType: stringValue(reservation.DBInstanceClass),
		Count:        count,
		OfferingType: stringValue(reservation.OfferingType),
		State:        stringValue(reservation.State),
		Start:        start,
		End:          endFromDuration(start, duration),
	}
}
