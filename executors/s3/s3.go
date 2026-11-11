// Package s3 is a read-only S3 executor: head, get and list of objects and the lifecycle configuration of a bucket.
//
// The endpoint may name a compose service without a dot (http://s3:9000); host_suffix turns it into a host the machine
// reaches (s3.cgw-crawler-chain.orb.local). Credentials come from access_key/secret_key or, when empty, from
// AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY of the venom process; they are never put into the result.
package s3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/evidence"
	"github.com/ovh/venom/executors/hostsuffix"
)

// Name of the executor.
const Name = "s3"

// MaxContent bounds result.content of a get.
const MaxContent = 1 << 20

// New returns a new executor.
func New() venom.Executor {
	return &Executor{}
}

// Executor: action head | get | list | lifecycle.
type Executor struct {
	Action     string `json:"action" yaml:"action" mapstructure:"action"`
	Endpoint   string `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint"`
	HostSuffix string `json:"host_suffix" yaml:"host_suffix" mapstructure:"host_suffix"`
	Region     string `json:"region" yaml:"region" mapstructure:"region"`
	AccessKey  string `json:"access_key" yaml:"access_key" mapstructure:"access_key"`
	SecretKey  string `json:"secret_key" yaml:"secret_key" mapstructure:"secret_key"`
	Bucket     string `json:"bucket" yaml:"bucket" mapstructure:"bucket"`
	Key        string `json:"key" yaml:"key" mapstructure:"key"`
	Prefix     string `json:"prefix" yaml:"prefix" mapstructure:"prefix"`
	MaxKeys    int    `json:"max_keys" yaml:"max_keys" mapstructure:"max_keys"`
	// OutputFile: the result as JSON is written to <venom.outputdir>/<output_file> (executors/evidence).
	OutputFile string `json:"output_file" yaml:"output_file" mapstructure:"output_file"`
}

// Rule is one lifecycle rule.
type Rule struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Prefix         string `json:"prefix"`
	ExpirationDays int32  `json:"expiration_days"`
}

// Result of the executor.
type Result struct {
	Exists      bool     `json:"exists"`
	Size        int64    `json:"size"`
	ContentType string   `json:"content_type"`
	Content     string   `json:"content"`
	Keys        []string `json:"keys"`
	Count       int      `json:"count"`
	Rules       []Rule   `json:"rules"`
	RulesCount  int      `json:"rules_count"`
	Code        string   `json:"code"`
	Err         string   `json:"err"`
	OutputFile  string   `json:"output_file,omitempty"`
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions: no error.
func (Executor) GetDefaultAssertions() *venom.StepAssertions {
	return &venom.StepAssertions{Assertions: []venom.Assertion{"result.err ShouldBeEmpty"}}
}

// Run executes the action.
func (Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	if e.Bucket == "" {
		return nil, errors.New("s3: bucket is required")
	}
	client, err := e.client()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var r Result
	switch e.Action {
	case "head":
		out, err := client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: &e.Bucket, Key: &e.Key})
		if err == nil {
			r.Exists, r.Size, r.ContentType = true, aws.ToInt64(out.ContentLength), aws.ToString(out.ContentType)
		}
		r.fail(err, true)
	case "get":
		out, err := client.GetObject(ctx, &awss3.GetObjectInput{Bucket: &e.Bucket, Key: &e.Key})
		if err == nil {
			defer out.Body.Close()
			b, rerr := io.ReadAll(io.LimitReader(out.Body, MaxContent))
			r.Exists, r.Size, r.ContentType, r.Content = true, aws.ToInt64(out.ContentLength), aws.ToString(out.ContentType), string(b)
			err = rerr
		}
		r.fail(err, true)
	case "list":
		in := &awss3.ListObjectsV2Input{Bucket: &e.Bucket}
		if e.Prefix != "" {
			in.Prefix = &e.Prefix
		}
		if e.MaxKeys > 0 {
			in.MaxKeys = aws.Int32(int32(e.MaxKeys))
		}
		out, err := client.ListObjectsV2(ctx, in)
		if err == nil {
			for _, o := range out.Contents {
				r.Keys = append(r.Keys, aws.ToString(o.Key))
			}
			r.Count = len(r.Keys)
		}
		r.fail(err, false)
	case "lifecycle":
		out, err := client.GetBucketLifecycleConfiguration(ctx, &awss3.GetBucketLifecycleConfigurationInput{Bucket: &e.Bucket})
		if err == nil {
			for _, rule := range out.Rules {
				x := Rule{ID: aws.ToString(rule.ID), Status: string(rule.Status)}
				if rule.Filter != nil && rule.Filter.Prefix != nil {
					x.Prefix = aws.ToString(rule.Filter.Prefix)
				}
				if rule.Expiration != nil && rule.Expiration.Days != nil {
					x.ExpirationDays = *rule.Expiration.Days
				}
				r.Rules = append(r.Rules, x)
			}
			r.RulesCount = len(r.Rules)
		}
		r.fail(err, false)
	default:
		return nil, fmt.Errorf("s3: action %q is not one of head, get, list, lifecycle", e.Action)
	}
	if e.OutputFile != "" {
		b, _ := json.Marshal(r)
		file, err := evidence.Write(ctx, e.OutputFile, string(b))
		if err != nil {
			return nil, err
		}
		r.OutputFile = file
	}
	return r, nil
}

// fail records an API error; a missing object of head/get is exists=false with its code, not an error.
func (r *Result) fail(err error, missingIsAnswer bool) {
	if err == nil {
		return
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		r.Code = api.ErrorCode()
		if missingIsAnswer && (r.Code == "NotFound" || r.Code == "NoSuchKey") {
			return
		}
	}
	r.Err = err.Error()
}

func (e Executor) client() (*awss3.Client, error) {
	endpoint, err := WithHostSuffix(e.Endpoint, e.HostSuffix)
	if err != nil {
		return nil, err
	}
	ak, sk := e.AccessKey, e.SecretKey
	if ak == "" {
		ak, sk = os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	region := e.Region
	if region == "" {
		region = "us-east-1"
	}
	return awss3.New(awss3.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(ak, sk, ""),
	}), nil
}

// WithHostSuffix appends suffix to a bare endpoint host when the suffixed name resolves (executors/hostsuffix).
func WithHostSuffix(endpoint, suffix string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("s3: endpoint %q is not a URL", endpoint)
	}
	return hostsuffix.URL(endpoint, suffix), nil
}
