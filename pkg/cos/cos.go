package cos

import (
	"fmt"
	"strings"

	"github.com/IBM/ibm-cos-sdk-go/aws"
	"github.com/IBM/ibm-cos-sdk-go/aws/credentials"
	"github.com/IBM/ibm-cos-sdk-go/aws/credentials/ibmiam"
	"github.com/IBM/ibm-cos-sdk-go/aws/session"
	cosaws "github.com/IBM/ibm-cos-sdk-go/service/s3"
	"go.uber.org/zap"
)

const (
	// IBM COS authentication endpoint.
	authEndpoint = "https://iam.cloud.ibm.com/identity/token"
	// IBM COS service endpoint template.
	serviceEndpointTpl = "https://s3.%s.cloud-object-storage.appdomain.cloud"
)

// Client performs COS operations using the IBM COS Go SDK.
// No external CLI dependency required.
type Client struct {
	log    *zap.Logger
	s3     *cosaws.S3
	bucket string
	region string
}

// NewClient creates a COS client authenticated via HMAC keys (access + secret).
// This is the standard method for CI/service accounts.
func NewClient(log *zap.Logger, bucket, region, accessKey, secretKey string) (*Client, error) {
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("COS_ACCESS_KEY and COS_SECRET_KEY are required for COS operations")
	}

	endpoint := fmt.Sprintf(serviceEndpointTpl, region)

	conf := aws.NewConfig().
		WithRegion(region).
		WithEndpoint(endpoint).
		WithCredentials(
			credentials.NewStaticCredentials(accessKey, secretKey, ""),
		).
		WithS3ForcePathStyle(true)

	sess, err := session.NewSession(conf)
	if err != nil {
		return nil, fmt.Errorf("creating COS session: %w", err)
	}

	return &Client{
		log:    log,
		s3:     cosaws.New(sess),
		bucket: bucket,
		region: region,
	}, nil
}

// NewClientFromAPIKey creates a COS client authenticated via an IBM Cloud API
// key (IAM-based). Use this when HMAC keys are not available.
func NewClientFromAPIKey(log *zap.Logger, bucket, region, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("IBMCLOUD_API_KEY is required for COS IAM authentication")
	}

	endpoint := fmt.Sprintf(serviceEndpointTpl, region)

	conf := aws.NewConfig().
		WithRegion(region).
		WithEndpoint(endpoint).
		WithCredentials(
			ibmiam.NewStaticCredentials(aws.NewConfig(), authEndpoint, apiKey, ""),
		).
		WithS3ForcePathStyle(true)

	sess, err := session.NewSession(conf)
	if err != nil {
		return nil, fmt.Errorf("creating COS session: %w", err)
	}

	return &Client{
		log:    log,
		s3:     cosaws.New(sess),
		bucket: bucket,
		region: region,
	}, nil
}

// ObjectExists checks whether an object with the given key exists in the
// configured bucket. Uses HeadObject — cheap, no data transfer.
func (c *Client) ObjectExists(objectKey string) (bool, error) {
	_, err := c.s3.HeadObject(&cosaws.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		// SDK wraps HTTP 404 as an awserr — check the message string since
		// we want to avoid importing aws/awserr just for this check.
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not found") ||
			strings.Contains(msg, "nosuchkey") ||
			strings.Contains(msg, "404") {
			return false, nil
		}
		return false, fmt.Errorf("HeadObject %s/%s: %w", c.bucket, objectKey, err)
	}
	return true, nil
}

// AssertObjectAbsent is the idempotency preflight guard: returns an error if
// the object already exists, so the pipeline fails fast before a long build.
func (c *Client) AssertObjectAbsent(objectKey string) error {
	c.log.Info("preflight: checking COS for existing object",
		zap.String("bucket", c.bucket),
		zap.String("object", objectKey),
		zap.String("region", c.region),
	)

	exists, err := c.ObjectExists(objectKey)
	if err != nil {
		// Auth failure, bucket not found, network issue — don't silently skip.
		return fmt.Errorf("COS preflight failed: %w", err)
	}
	if exists {
		return fmt.Errorf(
			"object %q already exists in bucket %q (region %s)\n"+
				"  → use a different --image-name, remove the existing object, or pass --skip-preflight",
			objectKey, c.bucket, c.region,
		)
	}

	c.log.Info("preflight OK: object does not exist", zap.String("object", objectKey))
	return nil
}

// ListObjects returns all object keys in the bucket matching an optional prefix.
// Useful for diagnostics and wiki generation.
func (c *Client) ListObjects(prefix string) ([]string, error) {
	input := &cosaws.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var keys []string
	err := c.s3.ListObjectsV2Pages(input, func(page *cosaws.ListObjectsV2Output, _ bool) bool {
		for _, obj := range page.Contents {
			keys = append(keys, aws.StringValue(obj.Key))
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("listing COS objects: %w", err)
	}
	return keys, nil
}
