package awsclient

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// CostExplorerAPI is the smallest AWS interface required by the MVP and keeps
// query and health tests independent from real AWS credentials.
type CostExplorerAPI interface {
	GetCostAndUsage(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

type Bundle struct {
	CostExplorer       CostExplorerAPI
	ResolveCredentials func(context.Context) error
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
		CostExplorer: costexplorer.NewFromConfig(awsConfig),
		ResolveCredentials: func(resolveContext context.Context) error {
			if _, resolveErr := awsConfig.Credentials.Retrieve(resolveContext); resolveErr != nil {
				return fmt.Errorf("resolve AWS credentials: %w", resolveErr)
			}
			return nil
		},
	}, nil
}
