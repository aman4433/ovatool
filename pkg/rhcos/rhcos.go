// Package rhcos resolves and downloads prebuilt RHCOS OVA images for PowerVS.
//
// Red Hat publishes RHCOS OVAs at two locations:
//
//  1. Public IBM COS bucket (stable releases):
//     https://s3.us-east.cloud-object-storage.appdomain.cloud/rhcos-powervs-images-us-east/
//
//  2. Red Hat release browser (nightly/candidate streams):
//     https://releases-rhcos-art.apps.ocp-virt.prod.psi.redhat.com/
//
// This package handles resolution against the public COS bucket because it is
// the authoritative stable source and requires no Red Hat authentication.
// Nightly stream access requires VPN and is noted in comments.
package rhcos

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// Public COS bucket hosting stable RHCOS PowerVS OVAs.
	publicBucket = "rhcos-powervs-images-us-east"
	publicRegion = "us-east"
	publicBase   = "https://s3.us-east.cloud-object-storage.appdomain.cloud/" + publicBucket + "/"

	// S3 ListBucketResult XML list endpoint.
	listEndpoint = publicBase + "?list-type=2&prefix=rhcos-"
)

// ReleaseInfo describes a single RHCOS OVA available in the public bucket.
type ReleaseInfo struct {
	ObjectName  string // e.g. rhcos-416-92-202305090606-0-ppc64le-powervs.ova.gz
	OCPVersion  string // e.g. 4.16
	RHELVersion string // e.g. 9.2
	BuildTS     string // e.g. 202305090606
	URL         string // full download URL
}

// ovaPattern extracts OCP version, RHEL version and build timestamp from the
// standard RHCOS PowerVS OVA filename.
// Example: rhcos-416-92-202305090606-0-ppc64le-powervs.ova.gz
var ovaPattern = regexp.MustCompile(
	`rhcos-(\d+)-(\d+)-(\d{12})-\d+-ppc64le-powervs\.ova\.gz`,
)

// ListReleases fetches the public bucket index and returns all available RHCOS
// PowerVS OVA releases, optionally filtered by OCP major.minor version.
//
// filter examples: "4.16", "4.19", "" (all versions)
func ListReleases(log *zap.Logger, filter string) ([]ReleaseInfo, error) {
	log.Info("fetching RHCOS release index", zap.String("source", listEndpoint))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(listEndpoint)
	if err != nil {
		return nil, fmt.Errorf("fetching RHCOS bucket index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHCOS bucket index returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading RHCOS bucket index: %w", err)
	}

	releases := parseIndex(string(body), filter)
	// Sort newest first by build timestamp.
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].BuildTS > releases[j].BuildTS
	})

	log.Info("found RHCOS releases", zap.Int("count", len(releases)))
	return releases, nil
}

// LatestRelease returns the most recent stable RHCOS OVA for the given OCP
// version. Returns an error if no matching release is found.
func LatestRelease(log *zap.Logger, ocpVersion string) (*ReleaseInfo, error) {
	releases, err := ListReleases(log, ocpVersion)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no RHCOS OVA found for OCP version %q in the public bucket", ocpVersion)
	}
	return &releases[0], nil
}

// DownloadOptions controls the Download call.
type DownloadOptions struct {
	// URL to download from (use ReleaseInfo.URL).
	URL string
	// DestDir is the directory to save the file into (default: current dir).
	DestDir string
	// FileName overrides the filename derived from the URL.
	FileName string
}

// Download fetches an RHCOS OVA to local disk, showing a simple progress
// indicator. Returns the path of the saved file.
func Download(log *zap.Logger, opts DownloadOptions) (string, error) {
	destDir := opts.DestDir
	if destDir == "" {
		destDir = "."
	}

	fileName := opts.FileName
	if fileName == "" {
		// Derive from URL.
		parts := strings.Split(opts.URL, "/")
		fileName = parts[len(parts)-1]
	}
	destPath := filepath.Join(destDir, fileName)

	// Don't re-download if already present.
	if info, err := os.Stat(destPath); err == nil {
		log.Info("file already present, skipping download",
			zap.String("path", destPath),
			zap.Int64("size_bytes", info.Size()),
		)
		return destPath, nil
	}

	log.Info("downloading RHCOS OVA",
		zap.String("url", opts.URL),
		zap.String("dest", destPath),
	)

	client := &http.Client{Timeout: 0} // no timeout for large files
	resp, err := client.Get(opts.URL)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", opts.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, opts.URL)
	}

	// Write to a temp file first, rename on success (atomic).
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	total := resp.ContentLength
	written, err := copyWithProgress(f, resp.Body, total)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing download: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("renaming temp file: %w", err)
	}

	log.Info("download complete",
		zap.String("path", destPath),
		zap.Int64("bytes", written),
	)
	return destPath, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func parseIndex(xml, filter string) []ReleaseInfo {
	// Lightweight XML key extraction — avoids importing encoding/xml for a
	// simple S3 ListBucketResult that only has <Key>...</Key> entries.
	keyRE := regexp.MustCompile(`<Key>(rhcos-[^<]+)</Key>`)
	matches := keyRE.FindAllStringSubmatch(xml, -1)

	var results []ReleaseInfo
	for _, m := range matches {
		key := m[1]
		sub := ovaPattern.FindStringSubmatch(key)
		if sub == nil {
			continue
		}

		ocpRaw := sub[1]   // "416"
		rhelRaw := sub[2]  // "92"
		ts := sub[3]       // "202305090606"

		ocpVer := formatVersion(ocpRaw)   // "4.16"
		rhelVer := formatVersion(rhelRaw) // "9.2"

		if filter != "" && ocpVer != filter {
			continue
		}

		results = append(results, ReleaseInfo{
			ObjectName:  key,
			OCPVersion:  ocpVer,
			RHELVersion: rhelVer,
			BuildTS:     ts,
			URL:         publicBase + key,
		})
	}
	return results
}

// formatVersion turns "416" → "4.16", "92" → "9.2", "419" → "4.19".
func formatVersion(s string) string {
	if len(s) == 3 {
		return s[:1] + "." + s[1:]
	}
	if len(s) == 2 {
		return s[:1] + "." + s[1:]
	}
	return s
}

// copyWithProgress copies from r to w, printing a simple progress line.
func copyWithProgress(w io.Writer, r io.Reader, total int64) (int64, error) {
	buf := make([]byte, 1024*1024) // 1 MB chunks
	var written int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			nw, werr := w.Write(buf[:n])
			written += int64(nw)
			if total > 0 {
				pct := float64(written) / float64(total) * 100
				fmt.Printf("\r  downloading... %.1f%% (%d / %d MB)",
					pct, written/1024/1024, total/1024/1024)
			} else {
				fmt.Printf("\r  downloading... %d MB", written/1024/1024)
			}
			if werr != nil {
				fmt.Println()
				return written, werr
			}
		}
		if err == io.EOF {
			fmt.Println()
			break
		}
		if err != nil {
			fmt.Println()
			return written, err
		}
	}
	return written, nil
}
