package awsmock

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	ectypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	ostypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	rstypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	sptypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
)

// Each mock *inventory* response is a small fixed snapshot (these APIs
// aren't scoped to a date range at all), but deliberately spans a mix of
// states -- including one "retired" item, matching the bundled dashboard's
// retired-filter transform -- and a mix of near/far/already-past expiry
// horizons, so the days-until-expiry sort is visibly meaningful rather than
// "some rows exist."

type SavingsPlansClient struct{}

func (SavingsPlansClient) DescribeSavingsPlans(
	_ context.Context,
	_ *savingsplans.DescribeSavingsPlansInput,
	_ ...func(*savingsplans.Options),
) (*savingsplans.DescribeSavingsPlansOutput, error) {
	return &savingsplans.DescribeSavingsPlansOutput{
		SavingsPlans: []sptypes.SavingsPlan{
			{
				SavingsPlanId:   aws.String("sp-mock-0a1b2c3d"),
				SavingsPlanType: sptypes.SavingsPlanTypeCompute,
				PaymentOption:   sptypes.SavingsPlanPaymentOptionNoUpfront,
				Commitment:      amount(0.75),
				State:           sptypes.SavingsPlanStateActive,
				Start:           rfc3339(daysFromNow(-120)),
				End:             rfc3339(daysFromNow(245)),
			},
			{
				SavingsPlanId:   aws.String("sp-mock-4e5f6a7b"),
				SavingsPlanType: sptypes.SavingsPlanTypeEc2Instance,
				PaymentOption:   sptypes.SavingsPlanPaymentOptionPartialUpfront,
				Region:          aws.String("us-east-1"),
				Commitment:      amount(1.20),
				State:           sptypes.SavingsPlanStateActive,
				Start:           rfc3339(daysFromNow(-30)),
				End:             rfc3339(daysFromNow(12)),
			},
			{
				SavingsPlanId:   aws.String("sp-mock-8c9d0e1f"),
				SavingsPlanType: sptypes.SavingsPlanTypeCompute,
				PaymentOption:   sptypes.SavingsPlanPaymentOptionAllUpfront,
				Commitment:      amount(0.35),
				State:           sptypes.SavingsPlanStateRetired,
				Start:           rfc3339(daysFromNow(-400)),
				End:             rfc3339(daysFromNow(-35)),
			},
		},
	}, nil
}

type EC2Client struct{}

func (EC2Client) DescribeReservedInstances(
	_ context.Context,
	_ *ec2.DescribeReservedInstancesInput,
	_ ...func(*ec2.Options),
) (*ec2.DescribeReservedInstancesOutput, error) {
	start1, end1 := daysFromNow(-200), daysFromNow(165)
	start2, end2 := daysFromNow(-500), daysFromNow(-135)
	return &ec2.DescribeReservedInstancesOutput{
		ReservedInstances: []ec2types.ReservedInstances{
			{
				ReservedInstancesId: aws.String("ri-mock-0123456789abcdef0"),
				InstanceType:        ec2types.InstanceTypeM5Large,
				InstanceCount:       aws.Int32(4),
				OfferingType:        ec2types.OfferingTypeValuesNoUpfront,
				State:               ec2types.ReservedInstanceStateActive,
				Start:               &start1,
				End:                 &end1,
			},
			{
				ReservedInstancesId: aws.String("ri-mock-fedcba9876543210f"),
				InstanceType:        ec2types.InstanceTypeC5Xlarge,
				InstanceCount:       aws.Int32(2),
				OfferingType:        ec2types.OfferingTypeValuesAllUpfront,
				State:               ec2types.ReservedInstanceStateRetired,
				Start:               &start2,
				End:                 &end2,
			},
		},
	}, nil
}

type RDSClient struct{}

func (RDSClient) DescribeReservedDBInstances(
	_ context.Context,
	_ *rds.DescribeReservedDBInstancesInput,
	_ ...func(*rds.Options),
) (*rds.DescribeReservedDBInstancesOutput, error) {
	start := daysFromNow(-150)
	return &rds.DescribeReservedDBInstancesOutput{
		ReservedDBInstances: []rdstypes.ReservedDBInstance{
			{
				ReservedDBInstanceId: aws.String("ri-mock-rds-a1b2c3"),
				DBInstanceClass:      aws.String("db.r5.large"),
				DBInstanceCount:      aws.Int32(2),
				OfferingType:         aws.String("No Upfront"),
				State:                aws.String("active"),
				StartTime:            &start,
				Duration:             aws.Int32(31536000), // 1 year
			},
		},
	}, nil
}

type ElastiCacheClient struct{}

func (ElastiCacheClient) DescribeReservedCacheNodes(
	_ context.Context,
	_ *elasticache.DescribeReservedCacheNodesInput,
	_ ...func(*elasticache.Options),
) (*elasticache.DescribeReservedCacheNodesOutput, error) {
	start := daysFromNow(-60)
	return &elasticache.DescribeReservedCacheNodesOutput{
		ReservedCacheNodes: []ectypes.ReservedCacheNode{
			{
				ReservedCacheNodeId: aws.String("ri-mock-cache-d4e5f6"),
				CacheNodeType:       aws.String("cache.r6g.large"),
				CacheNodeCount:      aws.Int32(3),
				OfferingType:        aws.String("Partial Upfront"),
				State:               aws.String("active"),
				StartTime:           &start,
				Duration:            aws.Int32(31536000),
			},
		},
	}, nil
}

type RedshiftClient struct{}

func (RedshiftClient) DescribeReservedNodes(
	_ context.Context,
	_ *redshift.DescribeReservedNodesInput,
	_ ...func(*redshift.Options),
) (*redshift.DescribeReservedNodesOutput, error) {
	start := daysFromNow(-20)
	return &redshift.DescribeReservedNodesOutput{
		ReservedNodes: []rstypes.ReservedNode{
			{
				ReservedNodeId: aws.String("ri-mock-redshift-7g8h9i"),
				NodeType:       aws.String("ra3.xlplus"),
				NodeCount:      aws.Int32(2),
				OfferingType:   aws.String("No Upfront"),
				State:          aws.String("active"),
				StartTime:      &start,
				Duration:       aws.Int32(94608000), // 3 years
			},
		},
	}, nil
}

type OpenSearchClient struct{}

func (OpenSearchClient) DescribeReservedInstances(
	_ context.Context,
	_ *opensearch.DescribeReservedInstancesInput,
	_ ...func(*opensearch.Options),
) (*opensearch.DescribeReservedInstancesOutput, error) {
	start := daysFromNow(-90)
	return &opensearch.DescribeReservedInstancesOutput{
		ReservedInstances: []ostypes.ReservedInstance{
			{
				ReservedInstanceId: aws.String("ri-mock-os-j0k1l2"),
				InstanceType:       ostypes.OpenSearchPartitionInstanceTypeM5LargeSearch,
				InstanceCount:      3,
				PaymentOption:      ostypes.ReservedInstancePaymentOptionNoUpfront,
				State:              aws.String("active"),
				StartTime:          &start,
				Duration:           31536000,
			},
		},
	}, nil
}

func rfc3339(t time.Time) *string {
	return aws.String(t.Format(time.RFC3339))
}
