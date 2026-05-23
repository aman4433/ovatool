// Package notify generates post-import wiki update output.
//
// After importing an image into PowerVS or PowerVC, the team manually updates
// a wiki page (GitHub Enterprise) with the new image name, workspace, password,
// and COS location. This package generates the exact row/section to paste,
// eliminating copy-paste errors.
package notify

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// ImageRecord holds all the information needed for a wiki row.
type ImageRecord struct {
	// Image identity
	Name        string // e.g. rhel-95-23052025
	Dist        string // rhel, centos, rhcos
	Version     string // e.g. 9.5, 10, 4.19
	BuildDate   time.Time

	// Location
	COSBucket   string
	COSRegion   string
	COSObject   string // the .ova.gz filename

	// PowerVS details (empty if not imported there)
	PVSWorkspace    string
	PVSImageName    string
	PVSStorageType  string

	// PowerVC details (empty if not imported there)
	PVCProject   string
	PVCImageName string

	// Credentials
	OSPassword  string // empty if --skip-os-password was used
}

// PrintSummary writes a formatted summary to w.
// This is the "copy-paste this into the wiki" output printed at the end of
// every successful import.
func PrintSummary(w io.Writer, rec ImageRecord) {
	sep := strings.Repeat("─", 70)

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, sep)
	fmt.Fprintln(w, "  WIKI UPDATE — paste the section below into the wiki page")
	fmt.Fprintln(w, "  https://github.ibm.com/redstack-power/docs/wiki/PowerVS-latest-Images")
	fmt.Fprintln(w, sep)
	fmt.Fprintln(w, "")

	// ── Markdown table row ───────────────────────────────────────────────────
	fmt.Fprintln(w, "### Markdown table row")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "| %-30s | %-10s | %-10s | %-20s | %-14s | %-20s |\n",
		"Image Name", "Dist", "Version", "COS Object", "Build Date", "OS Password")
	fmt.Fprintf(w, "| %-30s | %-10s | %-10s | %-20s | %-14s | %-20s |\n",
		strings.Repeat("-", 30),
		strings.Repeat("-", 10),
		strings.Repeat("-", 10),
		strings.Repeat("-", 20),
		strings.Repeat("-", 14),
		strings.Repeat("-", 20),
	)

	osPass := rec.OSPassword
	if osPass == "" {
		osPass = "_(key-based only)_"
	}

	fmt.Fprintf(w, "| %-30s | %-10s | %-10s | %-20s | %-14s | %-20s |\n",
		rec.Name,
		rec.Dist,
		rec.Version,
		rec.COSObject,
		rec.BuildDate.Format("02 Jan 2006"),
		osPass,
	)
	fmt.Fprintln(w, "")

	// ── PowerVS section ──────────────────────────────────────────────────────
	if rec.PVSWorkspace != "" {
		fmt.Fprintln(w, "### PowerVS")
		fmt.Fprintf(w, "- **Workspace**: `%s`\n", rec.PVSWorkspace)
		fmt.Fprintf(w, "- **Image name**: `%s`\n", rec.PVSImageName)
		fmt.Fprintf(w, "- **Storage type**: `%s`\n", rec.PVSStorageType)
		fmt.Fprintf(w, "- **COS bucket**: `%s` (region: `%s`)\n", rec.COSBucket, rec.COSRegion)
		fmt.Fprintf(w, "- **COS object**: `%s`\n", rec.COSObject)
		fmt.Fprintln(w, "")
	}

	// ── PowerVC section ──────────────────────────────────────────────────────
	if rec.PVCProject != "" {
		fmt.Fprintln(w, "### PowerVC")
		fmt.Fprintf(w, "- **Project**: `%s`\n", rec.PVCProject)
		fmt.Fprintf(w, "- **Image name**: `%s`\n", rec.PVCImageName)
		fmt.Fprintln(w, "")
	}

	// ── Credentials reminder ─────────────────────────────────────────────────
	if rec.OSPassword != "" {
		fmt.Fprintln(w, "> ⚠️  **Remember**: store the OS password in the wiki — it cannot be recovered after this point.")
		fmt.Fprintf(w, "> OS root password: `%s`\n", rec.OSPassword)
		fmt.Fprintln(w, "")
	}

	fmt.Fprintln(w, sep)
	fmt.Fprintln(w, "")
}

// PrintWikiRow prints just the table row (no headers), suitable for appending
// to an existing table.
func PrintWikiRow(w io.Writer, rec ImageRecord) {
	osPass := rec.OSPassword
	if osPass == "" {
		osPass = "_(key-based only)_"
	}
	fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s |\n",
		rec.Name,
		rec.Dist,
		rec.Version,
		rec.COSObject,
		rec.BuildDate.Format("02 Jan 2006"),
		osPass,
	)
}
