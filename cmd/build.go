package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	img "github.com/ppc64le-cloud/ovatool/pkg/image"
	"github.com/ppc64le-cloud/ovatool/pkg/pvsadm"
)

var (
	buildDist       string
	buildImageURL   string
	buildImageName  string
	buildImageSize  int
	buildTargetDisk int
	buildRHNUser    string
	buildRHNPass    string
	buildOSPass     string
	buildSkipOSPass bool
	buildVersion    string
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

	buildCmd.MarkFlagRequired("dist")
	buildCmd.MarkFlagRequired("image-url")
}

func runBuild() error {
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

	outPath, err := client.Build(pvsadm.BuildOptions{
		ImageName:   imageName,
		ImageURL:    buildImageURL,
		Dist:        string(dist),
		ImageSizGB:  buildImageSize,
		TargetDisk:  buildTargetDisk,
		RHNUser:     buildRHNUser,
		RHNPassword: buildRHNPass,
		OSPassword:  buildOSPass,
		SkipOSPass:  buildSkipOSPass,
		TempDir:     cfg.TempDir,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n✔ OVA ready: %s\n", outPath)
	fmt.Printf("  Next: ovatool upload --file %s\n", outPath)
	return nil
}
