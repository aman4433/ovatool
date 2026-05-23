package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	img "github.com/ppc64le-cloud/ovatool/pkg/image"
	"github.com/ppc64le-cloud/ovatool/pkg/notify"
)

var (
	runDist         string
	runImageURL     string
	runImageName    string
	runVersion      string
	runTargets      string
	runRHNUser      string
	runRHNPass      string
	runOSPass       string
	runSkipOSPass   bool
	runImageSize    int
	runTargetDisk   int
	runOSType       string
	runPublicBucket bool
	runObject       string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full OVA pipeline (build → upload → import)",
	Long: `run orchestrates the full image pipeline in a single command.

Use --target to select which stages to execute. Stages run in dependency order:
  build            — convert qcow2 to OVA
  upload           — push OVA to COS
  import-pvs       — import from COS into PowerVS
  import-powervc   — import from local path into PowerVC
  all              — all of the above

Combine stages with commas: --target build,upload  or  --target import-pvs,import-powervc

At the end of a successful import, ovatool prints a pre-formatted wiki update
section ready to paste into the team wiki page.

Examples:

  # Full pipeline for CentOS
  ovatool run --dist centos --version 9 \
    --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
    --target all

  # Build and upload only (import later)
  ovatool run --dist rhel --version 9.5 \
    --image-url ./rhel-9.5-ppc64le-kvm.qcow2 \
    --rhn-user user@example.com --rhn-password secret \
    --target build,upload

  # Import only (OVA already in COS)
  ovatool run --dist rhel --image-name rhel-95-23052025 \
    --object rhel-95-23052025.ova.gz \
    --target import-pvs

  # RHCOS — fetch first, then import from public bucket
  ovatool fetch --ocp-version 4.19 --dest-dir .
  ovatool run --dist rhcos \
    --image-name rhcos-419-23052025 \
    --object rhcos-419-ppc64le-powervs.ova.gz \
    --public-bucket \
    --target import-pvs`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPipeline()
	},
}

func init() {
	runCmd.Flags().StringVar(&runDist, "dist", "", "image distribution: rhel, centos, rhcos (required)")
	runCmd.Flags().StringVar(&runTargets, "target", "all", "comma-separated stages: build, upload, import-pvs, import-powervc, all")
	runCmd.Flags().StringVar(&runImageURL, "image-url", "", "path or URL to qcow2 image (required for build stage)")
	runCmd.Flags().StringVar(&runImageName, "image-name", "", "image name (auto-generated from --dist and --version if omitted)")
	runCmd.Flags().StringVar(&runVersion, "version", "", "image version for auto-naming (e.g. 9.5, 10, 4.19)")
	runCmd.Flags().StringVar(&runRHNUser, "rhn-user", "", "Red Hat subscription username (required for --dist rhel)")
	runCmd.Flags().StringVar(&runRHNPass, "rhn-password", "", "Red Hat subscription password (required for --dist rhel)")
	runCmd.Flags().StringVar(&runOSPass, "os-password", "", "root user password (auto-generated if omitted)")
	runCmd.Flags().BoolVar(&runSkipOSPass, "skip-os-password", false, "do not set a root password (key-based access only)")
	runCmd.Flags().IntVar(&runImageSize, "image-size", 11, "size in GB of the resultant OVA image")
	runCmd.Flags().IntVar(&runTargetDisk, "target-disk-size", 120, "size in GB of the target disk volume")
	runCmd.Flags().StringVar(&runOSType, "os-type", "rhel", "OS type for PowerVC import: rhel or coreos")
	runCmd.Flags().BoolVar(&runPublicBucket, "public-bucket", false, "source bucket is public (e.g. RHCOS prebuilt images)")
	runCmd.Flags().StringVar(&runObject, "object", "", "COS object name (for import-only runs where image is already in COS)")

	runCmd.MarkFlagRequired("dist")
}

func runPipeline() error {
	dist, err := img.ParseDist(runDist)
	if err != nil {
		return err
	}

	targets, err := img.ParseTargets(runTargets)
	if err != nil {
		return err
	}

	imageName, err := resolveImageName(dist)
	if err != nil {
		return err
	}

	logger.Info("pipeline starting",
		zap.String("dist", string(dist)),
		zap.String("image-name", imageName),
		zap.String("targets", runTargets),
	)

	var ovaFile string

	// ── Stage 1: Build ────────────────────────────────────────────────────────
	if targets.Has(img.TargetBuild) {
		if !dist.RequiresBuild() {
			logger.Info("skipping build stage — RHCOS OVAs are prebuilt (use: ovatool fetch)")
		} else {
			if runImageURL == "" {
				return fmt.Errorf("--image-url is required for the build stage")
			}
			buildDist = string(dist)
			buildImageURL = runImageURL
			buildImageName = imageName
			buildImageSize = runImageSize
			buildTargetDisk = runTargetDisk
			buildRHNUser = runRHNUser
			buildRHNPass = runRHNPass
			buildOSPass = runOSPass
			buildSkipOSPass = runSkipOSPass

			if err := runBuild(); err != nil {
				return fmt.Errorf("build stage: %w", err)
			}
			ovaFile = imageName + ".ova.gz"
		}
	}

	// ── Stage 2: Upload ───────────────────────────────────────────────────────
	if targets.Has(img.TargetUpload) {
		if ovaFile == "" {
			ovaFile = imageName + ".ova.gz"
		}
		uploadFile = ovaFile
		uploadBucket = ""
		uploadObjectName = ""
		uploadSkipPreflight = false

		if err := runUpload(ovaFile); err != nil {
			return fmt.Errorf("upload stage: %w", err)
		}
	}

	// ── Stage 3a: Import into PowerVS ─────────────────────────────────────────
	if targets.Has(img.TargetImportPVS) {
		object := runObject
		if object == "" {
			object = imageName + ".ova.gz"
		}
		importTarget = "pvs"
		importObject = object
		importPVSImageName = imageName
		importBucket = ""
		importBucketRegion = ""
		importWorkspace = ""
		importStorageType = ""
		importAccessKey = ""
		importSecretKey = ""
		importPublicBucket = runPublicBucket

		if err := runImportPVS(); err != nil {
			return fmt.Errorf("import-pvs stage: %w", err)
		}
	}

	// ── Stage 3b: Import into PowerVC ────────────────────────────────────────
	if targets.Has(img.TargetImportPVC) {
		localPath := filepath.Join(".", imageName+".ova.gz")
		importPVCImageName = imageName
		importImagePath = localPath
		importOSType = runOSType
		importPVCTemplate = ""

		if err := runImportPowerVC(); err != nil {
			return fmt.Errorf("import-powervc stage: %w", err)
		}
	}

	logger.Info("pipeline complete", zap.String("image-name", imageName))

	// ── Wiki notify ───────────────────────────────────────────────────────────
	// Print after any import stage so the operator knows exactly what to paste.
	if targets.Has(img.TargetImportPVS) || targets.Has(img.TargetImportPVC) {
		printWikiNotify(dist, imageName)
	}

	fmt.Printf("✔ all requested stages complete for image %q\n", imageName)
	return nil
}

func printWikiNotify(dist img.Dist, imageName string) {
	object := runObject
	if object == "" {
		object = imageName + ".ova.gz"
	}

	// Resolve version from name or flag for the record.
	version := runVersion
	if version == "" {
		version = "—"
	}

	rec := notify.ImageRecord{
		Name:      imageName,
		Dist:      string(dist),
		Version:   version,
		BuildDate: time.Now(),
		COSBucket: coalesce(importBucket, cfg.COSBucket),
		COSRegion: coalesce(importBucketRegion, cfg.COSBucketRegion),
		COSObject: object,
		OSPassword: runOSPass,
	}

	if importPublicBucket {
		rec.COSBucket = "rhcos-powervs-images-us-east"
		rec.COSRegion = "us-east"
	}

	if importPVSWorkspace := coalesce(importWorkspace, cfg.PVSWorkspaceName); importPVSWorkspace != "" {
		rec.PVSWorkspace = importPVSWorkspace
		rec.PVSImageName = imageName
		rec.PVSStorageType = coalesce(importStorageType, cfg.PVSStorageType)
	}

	if cfg.PowerVCProject != "" {
		rec.PVCProject = cfg.PowerVCProject
		rec.PVCImageName = importPVCImageName
	}

	notify.PrintSummary(os.Stdout, rec)
}

func resolveImageName(dist img.Dist) (string, error) {
	if runImageName != "" {
		return runImageName, nil
	}
	if runVersion == "" {
		return "", fmt.Errorf("provide --image-name or --version so ovatool can auto-generate a name")
	}
	name := img.GenerateName(dist, runVersion)
	logger.Info("auto-generated image name", zap.String("image-name", name))
	return name, nil
}
