package awsclient

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// CostExplorerAPI is the smallest AWS interface this plugin requires, and
// keeps query, lookup, and health tests independent from real AWS
// credentials.
type CostExplorerAPI interface {
	GetCostAndUsage(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	GetDimensionValues(context.Context, *costexplorer.GetDimensionValuesInput, ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error)
	GetTags(context.Context, *costexplorer.GetTagsInput, ...func(*costexplorer.Options)) (*costexplorer.GetTagsOutput, error)
	GetReservationUtilization(context.Context, *costexplorer.GetReservationUtilizationInput, ...func(*costexplorer.Options)) (*costexplorer.GetReservationUtilizationOutput, error)
	GetSavingsPlansUtilization(context.Context, *costexplorer.GetSavingsPlansUtilizationInput, ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansUtilizationOutput, error)
	GetReservationCoverage(context.Context, *costexplorer.GetReservationCoverageInput, ...func(*costexplorer.Options)) (*costexplorer.GetReservationCoverageOutput, error)
	GetSavingsPlansCoverage(context.Context, *costexplorer.GetSavingsPlansCoverageInput, ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansCoverageOutput, error)
}

type Bundle struct {
	CostExplorer            CostExplorerAPI
	SavingsPlans            SavingsPlansAPI
	EC2Reservations         EC2ReservedInstancesAPI
	RDSReservations         RDSReservedInstancesAPI
	ElastiCacheReservations ElastiCacheReservedNodesAPI
	RedshiftReservations    RedshiftReservedNodesAPI
	OpenSearchReservations  OpenSearchReservedInstancesAPI
	ResolveCredentials      func(context.Context) error
}

type Factory interface {
	New(context.Context, models.PluginSettings) (*Bundle, error)
}

type SDKFactory struct{}

func NewFactory() *SDKFactory {
	return &SDKFactory{}
}

func (f *SDKFactory) New(ctx context.Context, settings models.PluginSettings) (*Bundle, error) {
	settings.ApplyDefaults()
	if err := settings.Validate(); err != nil {
		return nil, err
	}

	// Always install the credentials explicitly supplied through Grafana.
	// This prevents the SDK from resolving ambient environment, file, or
	// workload credentials for either direct or AssumeRole authentication.
	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion(settings.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				settings.Secrets.AccessKeyID,
				settings.Secrets.SecretAccessKey,
				settings.Secrets.SessionToken,
			),
		),
	}

	awsConfig, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	if settings.AuthMode == models.AuthModeAssumeRole {
		provider := stscreds.NewAssumeRoleProvider(
			sts.NewFromConfig(awsConfig),
			settings.RoleARN,
			func(options *stscreds.AssumeRoleOptions) {
				options.RoleSessionName = settings.RoleSessionName
				if settings.Secrets.ExternalID != "" {
					options.ExternalID = aws.String(settings.Secrets.ExternalID)
				}
			},
		)
		awsConfig.Credentials = aws.NewCredentialsCache(provider)
	}

	return &Bundle{
		CostExplorer:            costexplorer.NewFromConfig(awsConfig),
		SavingsPlans:            savingsplans.NewFromConfig(awsConfig),
		EC2Reservations:         ec2.NewFromConfig(awsConfig),
		RDSReservations:         rds.NewFromConfig(awsConfig),
		ElastiCacheReservations: elasticache.NewFromConfig(awsConfig),
		RedshiftReservations:    redshift.NewFromConfig(awsConfig),
		OpenSearchReservations:  opensearch.NewFromConfig(awsConfig),
		ResolveCredentials: func(resolveContext context.Context) error {
			if _, resolveErr := awsConfig.Credentials.Retrieve(resolveContext); resolveErr != nil {
				return fmt.Errorf("resolve AWS credentials: %w", resolveErr)
			}
			return nil
		},
	}, nil
}
