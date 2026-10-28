package inventory

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// fetchEC2 lists EC2 Reserved Instances. Unlike every other reservation
// Describe call this package uses, EC2's doesn't paginate -- it returns
// everything in a single response.
func (s *Service) fetchEC2(ctx context.Context) ([]ReservedItem, error) {
	if s.ec2 == nil {
		return nil, nil
	}

	output, err := s.ec2.DescribeReservedInstances(ctx, &ec2.DescribeReservedInstancesInput{})
	if err != nil {
		return nil, err
	}

	items := make([]ReservedItem, 0, len(output.ReservedInstances))
	for _, reservation := range output.ReservedInstances {
		items = append(items, ec2ReservedItem(reservation))
	}
	return items, nil
}

func ec2ReservedItem(reservation types.ReservedInstances) ReservedItem {
	var count int32
	if reservation.InstanceCount != nil {
		count = *reservation.InstanceCount
	}
	return ReservedItem{
		Service:      ServiceEC2,
		ID:           stringValue(reservation.ReservedInstancesId),
		InstanceType: string(reservation.InstanceType),
		Count:        count,
		OfferingType: string(reservation.OfferingType),
		State:        string(reservation.State),
		Start:        timeValue(reservation.Start),
		End:          timeValue(reservation.End),
	}
}
