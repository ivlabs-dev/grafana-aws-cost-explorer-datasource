package awsclient

import (
	"context"
	"fmt"
	"net/http"

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
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsmock"
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
	// Regions lists the regions enabled for the account. Together with
	// ReservationClientsForRegion it lets Reservation inventory span every
	// region. Either may be nil, in which case only the configured region
	// (the *Reservations clients above) can be queried.
	Regions                     EC2RegionsAPI
	ReservationClientsForRegion func(region string) ReservationClients
	ResolveCredentials          func(context.Context) error
}

type Factory interface {
	New(context.Context, models.PluginSettings, *http.Client) (*Bundle, error)
}

type SDKFactory struct{}

func NewFactory() *SDKFactory {
	return &SDKFactory{}
}

// New builds the AWS clients. httpClient must be the Grafana plugin SDK HTTP
// client (httpclient.New) so every AWS request, including STS and the
// per-region clients, goes through Grafana's HTTP middlewares. It is unused in
// mock mode.
func (f *SDKFactory) New(ctx context.Context, settings models.PluginSettings, httpClient *http.Client) (*Bundle, error) {
	settings.ApplyDefaults()
	if err := settings.Validate(); err != nil {
		return nil, err
	}

	if settings.AuthMode == models.AuthModeMock {
		return &Bundle{
			CostExplorer:            awsmock.CostExplorerClient{},
			SavingsPlans:            awsmock.SavingsPlansClient{},
			EC2Reservations:         awsmock.EC2Client{},
			RDSReservations:         awsmock.RDSClient{},
			ElastiCacheReservations: awsmock.ElastiCacheClient{},
			RedshiftReservations:    awsmock.RedshiftClient{},
			OpenSearchReservations:  awsmock.OpenSearchClient{},
			Regions:                 awsmock.RegionsClient{},
			ReservationClientsForRegion: func(region string) ReservationClients {
				return ReservationClients{
					EC2:         awsmock.EC2Client{Region: region},
					RDS:         awsmock.RDSClient{Region: region},
					ElastiCache: awsmock.ElastiCacheClient{Region: region},
					Redshift:    awsmock.RedshiftClient{Region: region},
					OpenSearch:  awsmock.OpenSearchClient{Region: region},
				}
			},
			ResolveCredentials: awsmock.ResolveCredentials,
		}, nil
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
	// Install the Grafana SDK HTTP client after loading rather than through
	// config.WithHTTPClient: with AWS_CA_BUNDLE (or a profile ca_bundle) set,
	// LoadDefaultConfig refuses any client that is not the SDK's own
	// BuildableClient. TLS trust for AWS calls now comes from the Grafana
	// instance's HTTP settings. STS, service and regional clients all derive
	// from this config, so they all use the client.
	awsConfig.HTTPClient = httpClient

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
		Regions:                 ec2.NewFromConfig(awsConfig),
		ReservationClientsForRegion: func(region string) ReservationClients {
			regional := awsConfig.Copy()
			regional.Region = region
			return ReservationClients{
				EC2:         ec2.NewFromConfig(regional),
				RDS:         rds.NewFromConfig(regional),
				ElastiCache: elasticache.NewFromConfig(regional),
				Redshift:    redshift.NewFromConfig(regional),
				OpenSearch:  opensearch.NewFromConfig(regional),
			}
		},
		ResolveCredentials: func(resolveContext context.Context) error {
			if _, resolveErr := awsConfig.Credentials.Retrieve(resolveContext); resolveErr != nil {
				return fmt.Errorf("resolve AWS credentials: %w", resolveErr)
			}
			return nil
		},
	}, nil
}
