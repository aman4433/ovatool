package image

import (
	"fmt"
	"strings"
	"time"
)

// Dist represents a supported image distribution.
type Dist string

const (
	DistRHEL   Dist = "rhel"
	DistCentOS Dist = "centos"
	DistRHCOS  Dist = "rhcos"
)

// ParseDist parses and validates a distribution string.
func ParseDist(s string) (Dist, error) {
	switch Dist(strings.ToLower(s)) {
	case DistRHEL:
		return DistRHEL, nil
	case DistCentOS:
		return DistCentOS, nil
	case DistRHCOS:
		return DistRHCOS, nil
	default:
		return "", fmt.Errorf("unsupported distribution %q (supported: rhel, centos, rhcos)", s)
	}
}

// RequiresRHNCredentials returns true for distributions that need a Red Hat
// subscription username and password during the qcow2→OVA conversion.
func (d Dist) RequiresRHNCredentials() bool {
	return d == DistRHEL
}

// RequiresBuild returns true when the OVA must be built locally from a qcow2.
// RHCOS ships a prebuilt OVA so it skips the build stage entirely.
func (d Dist) RequiresBuild() bool {
	return d != DistRHCOS
}

// GenerateName auto-generates an image name following the project naming
// convention: <dist>-<version>-<ddmmyyyy>
//
// Examples:
//
//	rhel-95-23052025
//	centos-10-stream-23052025
//	rhcos-419-23052025
func GenerateName(dist Dist, version string) string {
	date := time.Now().Format("02012006") // ddmmyyyy
	v := strings.ReplaceAll(version, ".", "")
	return fmt.Sprintf("%s-%s-%s", dist, v, date)
}

// Target represents which pipeline stages to execute.
type Target int

const (
	TargetBuild      Target = 1 << iota // 1
	TargetUpload                         // 2
	TargetImportPVS                      // 4
	TargetImportPVC                      // 8
)

// ParseTargets parses a comma-separated target string into a bitmask.
// Accepts: build, upload, import-pvs, import-powervc, all
func ParseTargets(s string) (Target, error) {
	if strings.ToLower(s) == "all" {
		return TargetBuild | TargetUpload | TargetImportPVS | TargetImportPVC, nil
	}
	var result Target
	for _, part := range strings.Split(s, ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "build":
			result |= TargetBuild
		case "upload":
			result |= TargetUpload
		case "import-pvs":
			result |= TargetImportPVS
		case "import-powervc":
			result |= TargetImportPVC
		default:
			return 0, fmt.Errorf("unknown target %q (supported: build, upload, import-pvs, import-powervc, all)", part)
		}
	}
	if result == 0 {
		return 0, fmt.Errorf("no valid targets specified")
	}
	return result, nil
}

func (t Target) Has(flag Target) bool { return t&flag != 0 }
