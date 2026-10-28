package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	opensearchtypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	savingsplanstypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	smithy "github.com/aws/smithy-go"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// apiError is a minimal smithy.APIError fake for tests.
type apiError struct {
	code string
}

func (e apiError) Error() string                 { return e.code }
func (e apiError) ErrorCode() string             { return e.code }
func (e apiError) ErrorMessage() string          { return e.code }
func (e apiError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func validSettings() models.PluginSettings {
	return models.PluginSettings{
		AuthMode:        models.AuthModeStatic,
		Region:          "us-east-1",
		CacheTTLSeconds: models.DefaultCacheTTLSeconds,
		CacheMaxEntries: models.DefaultCacheMaxEntries,
		Secrets: &models.SecretPluginSettings{
			AccessKeyID:     "test-access-key",
			SecretAccessKey: "test-secret",
		},
	}
}

func newResultCache() cache.Cache {
	resultCache, err := cache.NewMemory(time.Minute, 64)
	if err != nil {
		panic(err)
	}
	return resultCache
}

type fakeSavingsPlansClient struct {
	calls int
	fn    func(*savingsplans.DescribeSavingsPlansInput) (*savingsplans.DescribeSavingsPlansOutput, error)
}

func (f *fakeSavingsPlansClient) DescribeSavingsPlans(
	_ context.Context,
	input *savingsplans.DescribeSavingsPlansInput,
	_ ...func(*savingsplans.Options),
) (*savingsplans.DescribeSavingsPlansOutput, error) {
	f.calls++
	return f.fn(input)
}

type fakeEC2Client struct {
	err   error
	items []ec2types.ReservedInstances
}

func (f *fakeEC2Client) DescribeReservedInstances(
	context.Context,
	*ec2.DescribeReservedInstancesInput,
	...func(*ec2.Options),
) (*ec2.DescribeReservedInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ec2.DescribeReservedInstancesOutput{ReservedInstances: f.items}, nil
}

type fakeRDSClient struct {
	err   error
	items []rdstypes.ReservedDBInstance
}

func (f *fakeRDSClient) DescribeReservedDBInstances(
	context.Context,
	*rds.DescribeReservedDBInstancesInput,
	...func(*rds.Options),
) (*rds.DescribeReservedDBInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &rds.DescribeReservedDBInstancesOutput{ReservedDBInstances: f.items}, nil
}

type fakeElastiCacheClient struct {
	err   error
	items []elasticachetypes.ReservedCacheNode
}

func (f *fakeElastiCacheClient) DescribeReservedCacheNodes(
	context.Context,
	*elasticache.DescribeReservedCacheNodesInput,
	...func(*elasticache.Options),
) (*elasticache.DescribeReservedCacheNodesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &elasticache.DescribeReservedCacheNodesOutput{ReservedCacheNodes: f.items}, nil
}

type fakeRedshiftClient struct {
	err   error
	items []redshifttypes.ReservedNode
}

func (f *fakeRedshiftClient) DescribeReservedNodes(
	context.Context,
	*redshift.DescribeReservedNodesInput,
	...func(*redshift.Options),
) (*redshift.DescribeReservedNodesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &redshift.DescribeReservedNodesOutput{ReservedNodes: f.items}, nil
}

type fakeOpenSearchClient struct {
	err   error
	items []opensearchtypes.ReservedInstance
}

func (f *fakeOpenSearchClient) DescribeReservedInstances(
	context.Context,
	*opensearch.DescribeReservedInstancesInput,
	...func(*opensearch.Options),
) (*opensearch.DescribeReservedInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &opensearch.DescribeReservedInstancesOutput{ReservedInstances: f.items}, nil
}

func sampleSavingsPlan(id string) savingsplanstypes.SavingsPlan {
	return savingsplanstypes.SavingsPlan{
		SavingsPlanId: aws.String(id),
		PaymentOption: savingsplanstypes.SavingsPlanPaymentOptionAllUpfront,
		Region:        aws.String("us-east-1"),
		Commitment:    aws.String("12.50"),
		State:         savingsplanstypes.SavingsPlanStateActive,
		Start:         aws.String("2026-01-01T00:00:00Z"),
		End:           aws.String("2027-01-01T00:00:00Z"),
	}
}

var errBoom = fmt.Errorf("boom")
