package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ppc64le-cloud/ovatool/pkg/rhcos"
)

var (
	fetchOCPVersion string
	fetchDestDir    string
	fetchListOnly   bool
	fetchURL        string // override: fetch a specific URL directly
)

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Resolve and download a prebuilt RHCOS OVA for PowerVS",
	Long: `fetch finds and downloads the latest prebuilt RHCOS OVA image for a
given OpenShift version from the IBM COS public bucket maintained by Red Hat.

No authentication is needed — these images are publicly available.

Examples:

  # List all available RHCOS releases
  ovatool fetch --list

  # List releases for a specific OCP version
  ovatool fetch --list --ocp-version 4.19

  # Download latest RHCOS for OCP 4.19
  ovatool fetch --ocp-version 4.19

  # Download into a specific directory
  ovatool fetch --ocp-version 4.16 --dest-dir /data/images

  # Download a specific OVA by URL (bypass resolution)
  ovatool fetch --url https://s3.us-east.cloud-object-storage.appdomain.cloud/rhcos-powervs-images-us-east/rhcos-416-92-202305090606-0-ppc64le-powervs.ova.gz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFetch()
	},
}

func init() {
	fetchCmd.Flags().StringVar(&fetchOCPVersion, "ocp-version", "", "OpenShift version to fetch RHCOS for (e.g. 4.19)")
	fetchCmd.Flags().StringVar(&fetchDestDir, "dest-dir", ".", "directory to save the downloaded OVA")
	fetchCmd.Flags().BoolVar(&fetchListOnly, "list", false, "list available releases without downloading")
	fetchCmd.Flags().StringVar(&fetchURL, "url", "", "download a specific OVA URL directly (bypasses version resolution)")
}

func runFetch() error {
	// ── Direct URL download ──────────────────────────────────────────────────
	if fetchURL != "" {
		path, err := rhcos.Download(logger, rhcos.DownloadOptions{
			URL:     fetchURL,
			DestDir: fetchDestDir,
		})
		if err != nil {
			return err
		}
		fmt.Printf("\n✔ saved to %s\n", path)
		printImportHint(path)
		return nil
	}

	// ── List mode ────────────────────────────────────────────────────────────
	if fetchListOnly {
		releases, err := rhcos.ListReleases(logger, fetchOCPVersion)
		if err != nil {
			return err
		}
		if len(releases) == 0 {
			fmt.Println("no releases found")
			return nil
		}
		fmt.Printf("\n%-8s  %-8s  %-14s  %s\n", "OCP", "RHEL", "BUILD", "OBJECT")
		fmt.Printf("%-8s  %-8s  %-14s  %s\n", "---", "----", "-----", "------")
		for _, r := range releases {
			fmt.Printf("%-8s  %-8s  %-14s  %s\n",
				r.OCPVersion, r.RHELVersion, r.BuildTS, r.ObjectName)
		}
		fmt.Printf("\nTotal: %d release(s)\n", len(releases))
		return nil
	}

	// ── Resolve latest + download ────────────────────────────────────────────
	if fetchOCPVersion == "" {
		return fmt.Errorf("provide --ocp-version (e.g. --ocp-version 4.19) or --list to see all available versions")
	}

	release, err := rhcos.LatestRelease(logger, fetchOCPVersion)
	if err != nil {
		return err
	}

	logger.Info("resolved latest RHCOS release",
		zap.String("ocp-version", release.OCPVersion),
		zap.String("rhel-version", release.RHELVersion),
		zap.String("build", release.BuildTS),
		zap.String("object", release.ObjectName),
	)
	fmt.Printf("\nLatest RHCOS for OCP %s:\n", release.OCPVersion)
	fmt.Printf("  Object : %s\n", release.ObjectName)
	fmt.Printf("  RHEL   : %s\n", release.RHELVersion)
	fmt.Printf("  Build  : %s\n", release.BuildTS)
	fmt.Printf("  URL    : %s\n\n", release.URL)

	path, err := rhcos.Download(logger, rhcos.DownloadOptions{
		URL:     release.URL,
		DestDir: fetchDestDir,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n✔ saved to %s\n", path)
	printImportHint(release.ObjectName)
	return nil
}

func printImportHint(objectName string) {
	fmt.Printf("\nTo import into PowerVS:\n")
	fmt.Printf("  ovatool import --target pvs \\\n")
	fmt.Printf("    --object %s \\\n", objectName)
	fmt.Printf("    --pvs-image-name <your-image-name> \\\n")
	fmt.Printf("    --public-bucket\n")
}
