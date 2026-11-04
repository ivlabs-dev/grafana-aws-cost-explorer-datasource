package awsclient

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
)

// SavingsPlansAPI is the smallest AWS Savings Plans interface this plugin
// requires to list owned Savings Plans for the inventory query type.
type SavingsPlansAPI interface {
	DescribeSavingsPlans(context.Context, *savingsplans.DescribeSavingsPlansInput, ...func(*savingsplans.Options)) (*savingsplans.DescribeSavingsPlansOutput, error)
}

// EC2ReservedInstancesAPI is the smallest AWS EC2 interface this plugin
// requires to list owned EC2 Reserved Instances.
type EC2ReservedInstancesAPI interface {
	DescribeReservedInstances(context.Context, *ec2.DescribeReservedInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeReservedInstancesOutput, error)
}

// RDSReservedInstancesAPI is the smallest AWS RDS interface this plugin
// requires to list owned RDS reserved DB instances.
type RDSReservedInstancesAPI interface {
	DescribeReservedDBInstances(context.Context, *rds.DescribeReservedDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeReservedDBInstancesOutput, error)
}

// ElastiCacheReservedNodesAPI is the smallest AWS ElastiCache interface this
// plugin requires to list owned reserved cache nodes.
type ElastiCacheReservedNodesAPI interface {
	DescribeReservedCacheNodes(context.Context, *elasticache.DescribeReservedCacheNodesInput, ...func(*elasticache.Options)) (*elasticache.DescribeReservedCacheNodesOutput, error)
}

// RedshiftReservedNodesAPI is the smallest AWS Redshift interface this
// plugin requires to list owned reserved nodes.
type RedshiftReservedNodesAPI interface {
	DescribeReservedNodes(context.Context, *redshift.DescribeReservedNodesInput, ...func(*redshift.Options)) (*redshift.DescribeReservedNodesOutput, error)
}

// OpenSearchReservedInstancesAPI is the smallest AWS OpenSearch interface
// this plugin requires to list owned reserved instances.
type OpenSearchReservedInstancesAPI interface {
	DescribeReservedInstances(context.Context, *opensearch.DescribeReservedInstancesInput, ...func(*opensearch.Options)) (*opensearch.DescribeReservedInstancesOutput, error)
}

// EC2RegionsAPI is the smallest AWS EC2 interface this plugin requires to
// discover which regions an account has enabled, so Reservation inventory
// can optionally be gathered from every region rather than only the
// data source's configured one.
type EC2RegionsAPI interface {
	DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
}

// ReservationClients groups the five regional Reservation "Describe"
// clients. Those AWS APIs only return reservations that live in the region
// they are called against, so the set is built once per region.
type ReservationClients struct {
	EC2         EC2ReservedInstancesAPI
	RDS         RDSReservedInstancesAPI
	ElastiCache ElastiCacheReservedNodesAPI
	Redshift    RedshiftReservedNodesAPI
	OpenSearch  OpenSearchReservedInstancesAPI
}
