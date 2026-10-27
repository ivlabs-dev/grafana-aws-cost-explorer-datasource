package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

type lookupClient struct {
	fakeClient
	dimensionValuesFn func(*awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error)
	tagsFn            func(*awscostexplorer.GetTagsInput) (*awscostexplorer.GetTagsOutput, error)
}

func (c lookupClient) GetDimensionValues(
	_ context.Context,
	input *awscostexplorer.GetDimensionValuesInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetDimensionValuesOutput, error) {
	if c.dimensionValuesFn == nil {
		return &awscostexplorer.GetDimensionValuesOutput{}, nil
	}
	return c.dimensionValuesFn(input)
}

func (c lookupClient) GetTags(
	_ context.Context,
	input *awscostexplorer.GetTagsInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetTagsOutput, error) {
	if c.tagsFn == nil {
		return &awscostexplorer.GetTagsOutput{}, nil
	}
	return c.tagsFn(input)
}

type capturingSender struct {
	response *backend.CallResourceResponse
}

func (s *capturingSender) Send(resp *backend.CallResourceResponse) error {
	s.response = resp
	return nil
}

func callResource(t *testing.T, datasource *Datasource, pathAndQuery string) *backend.CallResourceResponse {
	t.Helper()
	path, query, _ := strings.Cut(pathAndQuery, "?")
	sender := &capturingSender{}
	err := datasource.CallResource(context.Background(), &backend.CallResourceRequest{
		Method: "GET",
		Path:   path,
		URL:    "/" + path + "?" + query,
	}, sender)
	if err != nil {
		t.Fatalf("CallResource returned an error: %v", err)
	}
	if sender.response == nil {
		t.Fatal("CallResource did not send a response")
	}
	return sender.response
}

func decodeValues(t *testing.T, resp *backend.CallResourceResponse) []string {
	t.Helper()
	var body struct {
		Values []string `json:"values"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("decode response body %q: %v", resp.Body, err)
	}
	return body.Values
}

func TestCallResourceDimensionValues(t *testing.T) {
	client := lookupClient{
		dimensionValuesFn: func(input *awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error) {
			if input.Dimension != types.DimensionService {
				t.Fatalf("dimension = %q, want SERVICE", input.Dimension)
			}
			return &awscostexplorer.GetDimensionValuesOutput{
				DimensionValues: []types.DimensionValuesWithAttributes{
					{Value: strPtr("Amazon Elastic Compute Cloud - Compute")},
				},
			}, nil
		},
	}
	datasource := newDatasource(context.Background(), validInstanceSettings(), fakeFactory{client: client}, log.NewNullLogger())

	resp := callResource(t, datasource, "dimension-values?dimension=SERVICE&search=ec2")
	if resp.Status != 200 {
		t.Fatalf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
	values := decodeValues(t, resp)
	if len(values) != 1 || values[0] != "Amazon Elastic Compute Cloud - Compute" {
		t.Fatalf("values = %v", values)
	}
}

func TestCallResourceDimensionValuesRejectsUnsupportedDimension(t *testing.T) {
	datasource := newDatasource(context.Background(), validInstanceSettings(), fakeFactory{client: lookupClient{}}, log.NewNullLogger())

	resp := callResource(t, datasource, "dimension-values?dimension=USAGE_TYPE")
	if resp.Status != 400 {
		t.Fatalf("status = %d, want 400: %s", resp.Status, resp.Body)
	}
}

func TestCallResourceTagKeys(t *testing.T) {
	client := lookupClient{
		tagsFn: func(input *awscostexplorer.GetTagsInput) (*awscostexplorer.GetTagsOutput, error) {
			if input.TagKey != nil {
				t.Fatalf("tag key = %v, want nil", input.TagKey)
			}
			return &awscostexplorer.GetTagsOutput{Tags: []string{"Environment"}}, nil
		},
	}
	datasource := newDatasource(context.Background(), validInstanceSettings(), fakeFactory{client: client}, log.NewNullLogger())

	resp := callResource(t, datasource, "tag-keys?search=env")
	if resp.Status != 200 {
		t.Fatalf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
	values := decodeValues(t, resp)
	if len(values) != 1 || values[0] != "Environment" {
		t.Fatalf("values = %v", values)
	}
}

func TestCallResourceTagValues(t *testing.T) {
	client := lookupClient{
		tagsFn: func(input *awscostexplorer.GetTagsInput) (*awscostexplorer.GetTagsOutput, error) {
			if input.TagKey == nil || *input.TagKey != "Environment" {
				t.Fatalf("tag key = %v, want Environment", input.TagKey)
			}
			return &awscostexplorer.GetTagsOutput{Tags: []string{"production"}}, nil
		},
	}
	datasource := newDatasource(context.Background(), validInstanceSettings(), fakeFactory{client: client}, log.NewNullLogger())

	resp := callResource(t, datasource, "tag-values?key=Environment")
	if resp.Status != 200 {
		t.Fatalf("status = %d, want 200: %s", resp.Status, resp.Body)
	}
	values := decodeValues(t, resp)
	if len(values) != 1 || values[0] != "production" {
		t.Fatalf("values = %v", values)
	}
}

func TestCallResourceTagValuesRequiresKey(t *testing.T) {
	datasource := newDatasource(context.Background(), validInstanceSettings(), fakeFactory{client: lookupClient{}}, log.NewNullLogger())

	resp := callResource(t, datasource, "tag-values")
	if resp.Status != 400 {
		t.Fatalf("status = %d, want 400: %s", resp.Status, resp.Body)
	}
}

func TestCallResourceWithUninitializedDatasourceReturnsServerError(t *testing.T) {
	settings := validInstanceSettings()
	settings.JSONData = []byte(`{"authMode": "bogus"}`)
	datasource := newDatasource(context.Background(), settings, fakeFactory{client: lookupClient{}}, log.NewNullLogger())
	if datasource.initializationErr == nil {
		t.Fatal("expected the datasource to fail to initialize")
	}

	resp := callResource(t, datasource, "dimension-values?dimension=SERVICE")
	if resp.Status != 500 {
		t.Fatalf("status = %d, want 500: %s", resp.Status, resp.Body)
	}
}

func TestCallResourceClassifiesAWSErrorAsBadGateway(t *testing.T) {
	client := lookupClient{
		dimensionValuesFn: func(*awscostexplorer.GetDimensionValuesInput) (*awscostexplorer.GetDimensionValuesOutput, error) {
			return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "denied"}
		},
	}
	datasource := newDatasource(context.Background(), validInstanceSettings(), fakeFactory{client: client}, log.NewNullLogger())

	resp := callResource(t, datasource, "dimension-values?dimension=SERVICE")
	if resp.Status != 502 {
		t.Fatalf("status = %d, want 502: %s", resp.Status, resp.Body)
	}
}

func strPtr(value string) *string {
	return &value
}
