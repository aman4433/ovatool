package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ppc64le-cloud/ovatool/pkg/cos"
	"github.com/ppc64le-cloud/ovatool/pkg/pvsadm"
)

var (
	uploadFile          string
	uploadBucket        string
	uploadBucketRegion  string
	uploadCOSInstance   string
	uploadObjectName    string
	uploadSkipPreflight bool
)

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload an OVA image to IBM Cloud Object Storage",
	Long: `upload wraps 'pvsadm image upload' to push a local .ova.gz to COS.

Before uploading, a preflight check uses the IBM COS SDK to verify that no
object with the same name already exists in the bucket. The pipeline exits
early with a clear error rather than failing mid-upload.

Pass --skip-preflight to bypass this check.

Bucket and region are read from .env (COS_BUCKET, COS_BUCKET_REGION) or
can be overridden with flags.

Examples:

  ovatool upload --file rhel-95-23052025.ova.gz
  ovatool upload --file centos-9-23052025.ova.gz --bucket my-bucket --region us-east`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpload(uploadFile)
	},
}

func init() {
	uploadCmd.Flags().StringVar(&uploadFile, "file", "", "path to the .ova.gz file to upload (required)")
	uploadCmd.Flags().StringVar(&uploadBucket, "bucket", "", "COS bucket name (overrides COS_BUCKET)")
	uploadCmd.Flags().StringVar(&uploadBucketRegion, "region", "", "COS bucket region (overrides COS_BUCKET_REGION)")
	uploadCmd.Flags().StringVar(&uploadCOSInstance, "cos-instance", "", "COS instance name (overrides COS_INSTANCE_NAME)")
	uploadCmd.Flags().StringVar(&uploadObjectName, "object-name", "", "object name in COS (default: filename)")
	uploadCmd.Flags().BoolVar(&uploadSkipPreflight, "skip-preflight", false, "skip COS duplicate-object preflight check")

	uploadCmd.MarkFlagRequired("file")
}

func runUpload(file string) error {
	bucket := coalesce(uploadBucket, cfg.COSBucket)
	region := coalesce(uploadBucketRegion, cfg.COSBucketRegion)
	instance := coalesce(uploadCOSInstance, cfg.COSInstanceName)

	if bucket == "" {
		return fmt.Errorf("COS bucket not set — use --bucket or set COS_BUCKET in .env")
	}

	objectName := uploadObjectName
	if objectName == "" {
		objectName = filepath.Base(file)
	}

	// ── Preflight: assert object does not already exist ────────────────────
	if !uploadSkipPreflight {
		cosClient, err := cos.NewClient(logger, bucket, region, cfg.COSAccessKey, cfg.COSSecretKey)
		if err != nil {
			// If HMAC keys are missing, try IAM-based client.
			logger.Warn("HMAC keys not set, falling back to API key auth for preflight",
				zap.Error(err))
			cosClient, err = cos.NewClientFromAPIKey(logger, bucket, region, cfg.IBMCloudAPIKey)
			if err != nil {
				return fmt.Errorf("initialising COS client for preflight: %w", err)
			}
		}
		if err := cosClient.AssertObjectAbsent(objectName); err != nil {
			return err
		}
	}

	// ── Upload via pvsadm ──────────────────────────────────────────────────
	client, err := pvsadm.NewClient(logger, cfg.IBMCloudAPIKey)
	if err != nil {
		return err
	}

	if err := client.Upload(pvsadm.UploadOptions{
		Bucket:          bucket,
		BucketRegion:    region,
		COSInstanceName: instance,
		File:            file,
		ObjectName:      objectName,
	}); err != nil {
		return err
	}

	fmt.Printf("\n✔ uploaded %s → s3://%s/%s (region: %s)\n", file, bucket, objectName, region)
	fmt.Printf("  Next: ovatool import --target pvs --object %s\n", objectName)
	return nil
}

// coalesce returns the first non-empty string.
func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
