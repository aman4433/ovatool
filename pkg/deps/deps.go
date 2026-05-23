package deps

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// Dep describes a required system binary and how to install it.
type Dep struct {
	Binary  string
	Package string
	UsedBy  string
}

// BuildDeps are the system tools required for the build (qcow2→OVA) stage.
var BuildDeps = []Dep{
	{"qemu-img", "qemu-img", "build stage (qcow2 → raw conversion)"},
	{"growpart", "cloud-utils-growpart", "build stage (disk partition resize)"},
}

// CurlDep is required for downloading pvsadm.
var CurlDep = Dep{"curl", "curl", "pvsadm download"}

// Missing returns the subset of deps not found on PATH.
func Missing(deps []Dep) []Dep {
	var absent []Dep
	for _, d := range deps {
		if _, err := exec.LookPath(d.Binary); err != nil {
			absent = append(absent, d)
		}
	}
	return absent
}

// PreflightError returns a formatted, actionable error for missing build deps.
func PreflightError(missing []Dep) error {
	var sb strings.Builder
	for _, d := range missing {
		fmt.Fprintf(&sb, "  ✗ %s not found\n", d.Binary)
		fmt.Fprintf(&sb, "    required by: %s\n", d.UsedBy)
		fmt.Fprintf(&sb, "    fix: %s install -y %s\n", packageManager(), d.Package)
	}
	fmt.Fprintf(&sb, "\n  or install all at once: ovatool init --install-deps")
	return fmt.Errorf("preflight failed: missing required system tools\n%s", sb.String())
}

// Install installs deps using dnf (falls back to yum).
// Binaries already on PATH are skipped — safe to re-run.
func Install(log *zap.Logger, deps []Dep) error {
	pm := packageManager()
	for _, d := range deps {
		if _, err := exec.LookPath(d.Binary); err == nil {
			log.Info("already installed, skipping", zap.String("tool", d.Binary))
			continue
		}
		log.Info("installing system tool", zap.String("tool", d.Binary), zap.String("package", d.Package), zap.String("via", pm))
		cmd := exec.Command(pm, "install", "-y", d.Package)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("installing %s via %s: %w", d.Package, pm, err)
		}
		log.Info("installed", zap.String("tool", d.Binary))
	}
	return nil
}

func packageManager() string {
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	}
	return "yum"
}
