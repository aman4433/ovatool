package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/ppc64le-cloud/ovatool/pkg/config"
	"github.com/ppc64le-cloud/ovatool/pkg/deps"
	"github.com/ppc64le-cloud/ovatool/pkg/pvsadm"
)

var installPVSADM bool
var installDeps bool
var pvsadmVersion string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate .env.template and optionally install pvsadm",
	Long: `init sets up the local environment for ovatool.

It writes a .env.template file to the current directory with all supported
configuration keys and their defaults. Copy it to .env and fill in your values:

  ovatool init
  cp .env.template .env
  vi .env

Pass --install-deps to install required system tools (qemu-img, growpart, curl).
Pass --install-pvsadm to download and install the pvsadm binary.
Both flags are idempotent — safe to re-run and use as a Jenkins setup step:

  ovatool init --install-deps --install-pvsadm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Minimal logger for init (no .env loaded yet).
		log, _ := zap.NewDevelopment()

		if err := config.WriteEnvTemplate(); err != nil {
			return err
		}
		fmt.Println("✔ wrote .env.template — copy to .env and fill in your values")

		// Install system deps before pvsadm — curl is needed to download it.
		if installDeps {
			toInstall := append(deps.BuildDeps, deps.CurlDep)
			if err := deps.Install(log, toInstall); err != nil {
				return fmt.Errorf("installing system dependencies: %w", err)
			}
			fmt.Println("✔ system dependencies installed (qemu-img, growpart, curl)")
		}

		if installPVSADM {
			if err := pvsadm.Install(log, pvsadmVersion); err != nil {
				return fmt.Errorf("installing pvsadm: %w", err)
			}
			fmt.Printf("✔ pvsadm %s installed at /usr/local/bin/pvsadm\n", pvsadmVersion)
		}

		fmt.Println("\nNext steps:")
		fmt.Println("  cp .env.template .env")
		fmt.Println("  # edit .env with your credentials and config")
		fmt.Println("  ovatool run --dist centos --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 --target all")
		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&installDeps, "install-deps", false, "install required system tools: qemu-img, growpart, curl")
	initCmd.Flags().BoolVar(&installPVSADM, "install-pvsadm", false, "download and install pvsadm")
	initCmd.Flags().StringVar(&pvsadmVersion, "pvsadm-version", "v0.1.15", "pvsadm version to install")
}
