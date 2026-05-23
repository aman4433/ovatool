package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ppc64le-cloud/ovatool/pkg/powervc"
	"github.com/ppc64le-cloud/ovatool/pkg/pvsadm"
)

var (
	importTarget        string // pvs, powervc, all
	importObject        string
	importPVSImageName  string
	importPVCImageName  string
	importBucket        string
	importBucketRegion  string
	importWorkspace     string
	importStorageType   string
	importAccessKey     string
	importSecretKey     string
	importPublicBucket  bool
	importImagePath     string // local path, for PowerVC
	importOSType        string // rhel or coreos
	importPVCTemplate   string // PowerVC storage template ID
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an OVA image into PowerVS and/or PowerVC",
	Long: `import brings an OVA image (already in COS or locally downloaded) into
IBM Power infrastructure environments.

Use --target to control which environments to import into:
  pvs       — import into IBM Power Virtual Server (via pvsadm image import)
  powervc   — import into PowerVC (via powervc-image create import)
  all       — import into both

PowerVS import reads from COS. PowerVC import reads from a local file path.

Examples:

  # Import into PowerVS only
  ovatool import --target pvs --object rhel-95-23052025.ova.gz --pvs-image-name rhel-95-23052025

  # Import into PowerVC only (image already downloaded locally)
  ovatool import --target powervc --image-path ./rhel-95-23052025.ova.gz --pvc-image-name rhel-95-23052025

  # Import RHCOS from public bucket into PowerVS
  ovatool import --target pvs \
    --object rhcos-419-ppc64le-powervs.ova.gz \
    --bucket rhcos-powervs-images-us-east \
    --region us-east \
    --public-bucket \
    --pvs-image-name rhcos-419-23052025`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runImport()
	},
}

func init() {
	importCmd.Flags().StringVar(&importTarget, "target", "", "import target: pvs, powervc, or all (required)")
	importCmd.Flags().StringVar(&importObject, "object", "", "COS object name (required for pvs target)")
	importCmd.Flags().StringVar(&importPVSImageName, "pvs-image-name", "", "name of the image in PowerVS (required for pvs target)")
	importCmd.Flags().StringVar(&importPVCImageName, "pvc-image-name", "", "name of the image in PowerVC (required for powervc target)")
	importCmd.Flags().StringVar(&importBucket, "bucket", "", "COS bucket name (overrides COS_BUCKET)")
	importCmd.Flags().StringVar(&importBucketRegion, "region", "", "COS bucket region (overrides COS_BUCKET_REGION)")
	importCmd.Flags().StringVar(&importWorkspace, "workspace", "", "PowerVS workspace name (overrides PVS_WORKSPACE_NAME)")
	importCmd.Flags().StringVar(&importStorageType, "storage-type", "", "PowerVS storage type: tier1 or tier3 (overrides PVS_STORAGE_TYPE)")
	importCmd.Flags().StringVar(&importAccessKey, "access-key", "", "COS access key (overrides COS_ACCESS_KEY)")
	importCmd.Flags().StringVar(&importSecretKey, "secret-key", "", "COS secret key (overrides COS_SECRET_KEY)")
	importCmd.Flags().BoolVar(&importPublicBucket, "public-bucket", false, "source bucket is public (e.g. RHCOS prebuilt images)")
	importCmd.Flags().StringVar(&importImagePath, "image-path", "", "local path to .ova.gz for PowerVC import")
	importCmd.Flags().StringVar(&importOSType, "os-type", "rhel", "OS type for PowerVC: rhel or coreos")
	importCmd.Flags().StringVar(&importPVCTemplate, "pvc-storage-template", "", "PowerVC storage template ID (overrides POWERVC_STORAGE_TEMPLATE_ID)")

	importCmd.MarkFlagRequired("target")
}

func runImport() error {
	target, err := parseImportTarget(importTarget)
	if err != nil {
		return err
	}

	if target.pvs {
		if err := runImportPVS(); err != nil {
			return fmt.Errorf("PowerVS import: %w", err)
		}
	}
	if target.pvc {
		if err := runImportPowerVC(); err != nil {
			return fmt.Errorf("PowerVC import: %w", err)
		}
	}
	return nil
}

func runImportPVS() error {
	bucket := coalesce(importBucket, cfg.COSBucket)
	region := coalesce(importBucketRegion, cfg.COSBucketRegion)
	workspace := coalesce(importWorkspace, cfg.PVSWorkspaceName)
	storageType := coalesce(importStorageType, cfg.PVSStorageType)
	accessKey := coalesce(importAccessKey, cfg.COSAccessKey)
	secretKey := coalesce(importSecretKey, cfg.COSSecretKey)

	if importObject == "" {
		return fmt.Errorf("--object is required for PowerVS import")
	}
	if importPVSImageName == "" {
		return fmt.Errorf("--pvs-image-name is required for PowerVS import")
	}
	if workspace == "" {
		return fmt.Errorf("PowerVS workspace not set — use --workspace or set PVS_WORKSPACE_NAME in .env")
	}

	client, err := pvsadm.NewClient(logger, cfg.IBMCloudAPIKey)
	if err != nil {
		return err
	}

	if err := client.Import(pvsadm.ImportOptions{
		WorkspaceName: workspace,
		Bucket:        bucket,
		BucketRegion:  region,
		Object:        importObject,
		PVSImageName:  importPVSImageName,
		StorageType:   storageType,
		AccessKey:     accessKey,
		SecretKey:     secretKey,
		PublicBucket:  importPublicBucket,
	}); err != nil {
		return err
	}

	fmt.Printf("\n✔ image %q is now active in PowerVS workspace %q\n", importPVSImageName, workspace)
	return nil
}

func runImportPowerVC() error {
	templateID := coalesce(importPVCTemplate, cfg.PowerVCStorageTemplateID)
	imageName := importPVCImageName
	imagePath := importImagePath

	if imageName == "" {
		return fmt.Errorf("--pvc-image-name is required for PowerVC import")
	}
	if imagePath == "" {
		return fmt.Errorf("--image-path is required for PowerVC import (local path to the .ova.gz)")
	}
	if templateID == "" {
		return fmt.Errorf("PowerVC storage template ID not set — use --pvc-storage-template or set POWERVC_STORAGE_TEMPLATE_ID in .env")
	}

	// Source powervcrc if present on this node.
	if err := powervc.SourcePowerVCRC(); err != nil {
		logger.Warn("could not source powervcrc", zap.Error(err))
	}

	client := powervc.NewClient(
		logger,
		cfg.PowerVCHost,
		cfg.PowerVCUsername,
		cfg.PowerVCPassword,
		cfg.PowerVCProject,
	)

	if err := client.Import(powervc.ImportOptions{
		ImageName:         imageName,
		ImagePath:         imagePath,
		StorageTemplateID: templateID,
		OSType:            importOSType,
	}); err != nil {
		return err
	}

	fmt.Printf("\n✔ image %q imported into PowerVC project %q\n", imageName, cfg.PowerVCProject)
	return nil
}

// importTargetFlags is a parsed version of the --target flag.
type importTargetFlags struct {
	pvs bool
	pvc bool
}

func parseImportTarget(s string) (importTargetFlags, error) {
	switch s {
	case "pvs":
		return importTargetFlags{pvs: true}, nil
	case "powervc":
		return importTargetFlags{pvc: true}, nil
	case "all":
		return importTargetFlags{pvs: true, pvc: true}, nil
	default:
		return importTargetFlags{}, fmt.Errorf("unknown import target %q (supported: pvs, powervc, all)", s)
	}
}
