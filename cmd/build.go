package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ppc64le-cloud/ovatool/pkg/cos"
	"github.com/ppc64le-cloud/ovatool/pkg/deps"
	img "github.com/ppc64le-cloud/ovatool/pkg/image"
	"github.com/ppc64le-cloud/ovatool/pkg/pvsadm"
)

var (
	buildDist             string
	buildImageURL         string
	buildImageName        string
	buildImageSize        int
	buildTargetDisk       int
	buildRHNUser          string
	buildRHNPass          string
	buildOSPass           string
	buildSkipOSPass       bool
	buildVersion          string
	buildPrepTemplate     string
	buildNameserver       string
	buildImageCOSObject   string
	buildImageCOSBucket   string
	buildImageCOSRegion   string
	buildImageCOSAccessKey string
	buildImageCOSSecretKey string
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Convert a qcow2 image to OVA format using pvsadm",
	Long: `build wraps 'pvsadm image qcow2ova' to convert a downloaded qcow2 image
into an OVA bundle suitable for PowerVS and PowerVC.

Must run on a ppc64le machine (LPAR or VM) with qemu-img and growpart installed.

Examples:

  # CentOS (no RHN credentials needed)
  ovatool build --dist centos --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2

  # RHEL (RHN credentials required)
  ovatool build --dist rhel --image-url ./rhel-9.5-ppc64le-kvm.qcow2 \
    --rhn-user user@example.com --rhn-password secret

  # With explicit image name
  ovatool build --dist centos --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
    --image-name centos-9-stream-23052025`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBuild()
	},
}

func init() {
	buildCmd.Flags().StringVar(&buildDist, "dist", "", "image distribution: rhel, centos (required)")
	buildCmd.Flags().StringVar(&buildImageURL, "image-url", "", "path or URL to the qcow2(.gz) image (required)")
	buildCmd.Flags().StringVar(&buildImageName, "image-name", "", "output image name (auto-generated if omitted)")
	buildCmd.Flags().StringVar(&buildVersion, "version", "", "image version, used for auto-generated name (e.g. 9.5)")
	buildCmd.Flags().IntVar(&buildImageSize, "image-size", 11, "size in GB of the resultant OVA image")
	buildCmd.Flags().IntVar(&buildTargetDisk, "target-disk-size", 120, "size in GB of the target disk volume")
	buildCmd.Flags().StringVar(&buildRHNUser, "rhn-user", "", "Red Hat subscription username (required for --dist rhel)")
	buildCmd.Flags().StringVar(&buildRHNPass, "rhn-password", "", "Red Hat subscription password (required for --dist rhel)")
	buildCmd.Flags().StringVar(&buildOSPass, "os-password", "", "root user password (auto-generated if omitted)")
	buildCmd.Flags().BoolVar(&buildSkipOSPass, "skip-os-password", false, "do not set a root password (cloud/key-based access only)")
	buildCmd.Flags().StringVar(&buildPrepTemplate, "prep-template", "", "path to a custom pvsadm prep script template")
	buildCmd.Flags().StringVar(&buildNameserver, "nameserver", "", "DNS nameserver to inject into the prep template (replaces hardcoded 9.9.9.9)")
	buildCmd.Flags().StringVar(&buildImageCOSObject, "image-cos-object", "", "COS object key of the source qcow2 image (alternative to --image-url)")
	buildCmd.Flags().StringVar(&buildImageCOSBucket, "image-cos-bucket", "", "COS bucket containing the source qcow2 (overrides COS_BUCKET)")
	buildCmd.Flags().StringVar(&buildImageCOSRegion, "image-cos-region", "", "COS region of the source bucket (overrides COS_BUCKET_REGION)")
	buildCmd.Flags().StringVar(&buildImageCOSAccessKey, "image-cos-access-key", "", "COS access key for source bucket (overrides COS_ACCESS_KEY)")
	buildCmd.Flags().StringVar(&buildImageCOSSecretKey, "image-cos-secret-key", "", "COS secret key for source bucket (overrides COS_SECRET_KEY)")

	buildCmd.MarkFlagRequired("dist")
}

func runBuild() error {
	if missing := deps.Missing(deps.BuildDeps); len(missing) > 0 {
		return deps.PreflightError(missing)
	}

	// Validate image source — exactly one of --image-url or --image-cos-object required.
	if buildImageURL == "" && buildImageCOSObject == "" {
		return fmt.Errorf("provide --image-url (local path or HTTPS URL) or --image-cos-object (COS object key)")
	}
	if buildImageURL != "" && buildImageCOSObject != "" {
		return fmt.Errorf("--image-url and --image-cos-object are mutually exclusive")
	}

	// Download source qcow2 from COS if --image-cos-object was given.
	if buildImageCOSObject != "" {
		bucket := coalesce(buildImageCOSBucket, cfg.COSBucket)
		region := coalesce(buildImageCOSRegion, cfg.COSBucketRegion)
		accessKey := coalesce(buildImageCOSAccessKey, cfg.COSAccessKey)
		secretKey := coalesce(buildImageCOSSecretKey, cfg.COSSecretKey)

		cosClient, err := cos.NewClient(logger, bucket, region, accessKey, secretKey)
		if err != nil {
			cosClient, err = cos.NewClientFromAPIKey(logger, bucket, region, cfg.IBMCloudAPIKey)
			if err != nil {
				return fmt.Errorf("initialising COS client for image download: %w", err)
			}
		}

		destPath := filepath.Join(cfg.TempDir, filepath.Base(buildImageCOSObject))
		if err := cosClient.Download(buildImageCOSObject, destPath); err != nil {
			return err
		}
		defer func() {
			logger.Info("removing downloaded source image", zap.String("path", destPath))
			os.Remove(destPath)
		}()
		buildImageURL = destPath
		logger.Info("source image downloaded from COS", zap.String("path", destPath))
	}

	dist, err := img.ParseDist(buildDist)
	if err != nil {
		return err
	}
	if !dist.RequiresBuild() {
		return fmt.Errorf("dist %q does not require a build step — RHCOS OVAs are prebuilt by Red Hat", dist)
	}
	if dist.RequiresRHNCredentials() {
		if buildRHNUser == "" || buildRHNPass == "" {
			return fmt.Errorf("--rhn-user and --rhn-password are required for --dist rhel")
		}
	}

	// Resolve image name.
	imageName := buildImageName
	if imageName == "" {
		if buildVersion == "" {
			return fmt.Errorf("provide --image-name or --version so ovatool can auto-generate a name")
		}
		imageName = img.GenerateName(dist, buildVersion)
		logger.Info("auto-generated image name", zap.String("image-name", imageName))
	}

	client, err := pvsadm.NewClient(logger, cfg.IBMCloudAPIKey)
	if err != nil {
		return err
	}

	prepTemplate := buildPrepTemplate
	if prepTemplate == "" && buildNameserver != "" {
		path, cleanup, err := client.PatchedPrepTemplate(buildNameserver)
		if err != nil {
			return err
		}
		defer cleanup()
		prepTemplate = path
		logger.Info("using auto-patched prep template", zap.String("nameserver", buildNameserver))
	}

	outPath, err := client.Build(pvsadm.BuildOptions{
		ImageName:    imageName,
		ImageURL:     buildImageURL,
		Dist:         string(dist),
		ImageSizGB:   buildImageSize,
		TargetDisk:   buildTargetDisk,
		RHNUser:      buildRHNUser,
		RHNPassword:  buildRHNPass,
		OSPassword:   buildOSPass,
		SkipOSPass:   buildSkipOSPass,
		TempDir:      cfg.TempDir,
		PrepTemplate: prepTemplate,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n✔ OVA ready: %s\n", outPath)
	fmt.Printf("  Next: ovatool upload --file %s\n", outPath)
	return nil
}
