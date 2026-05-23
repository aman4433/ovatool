package pvsadm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"go.uber.org/zap"
)

const binaryName = "pvsadm"
const installPath = "/usr/local/bin/pvsadm"

// Client wraps the pvsadm binary.
type Client struct {
	log        *zap.Logger
	binaryPath string
	apiKey     string
}

// NewClient returns a Client. It verifies pvsadm is available on PATH or at
// installPath.
func NewClient(log *zap.Logger, apiKey string) (*Client, error) {
	path, err := resolveBinary()
	if err != nil {
		return nil, err
	}
	return &Client{log: log, binaryPath: path, apiKey: apiKey}, nil
}

func resolveBinary() (string, error) {
	// Check PATH first.
	if p, err := exec.LookPath(binaryName); err == nil {
		return p, nil
	}
	// Fall back to known install location.
	if _, err := os.Stat(installPath); err == nil {
		return installPath, nil
	}
	return "", fmt.Errorf("pvsadm not found on PATH or at %s — run `ovatool init --install-pvsadm` to install", installPath)
}

// Install downloads and installs pvsadm at /usr/local/bin/pvsadm.
func Install(log *zap.Logger, version string) error {
	arch := runtime.GOARCH
	// Map Go arch names to pvsadm release artifact names.
	archMap := map[string]string{
		"ppc64le": "ppc64le",
		"amd64":   "amd64",
		"arm64":   "arm64",
	}
	releaseArch, ok := archMap[arch]
	if !ok {
		return fmt.Errorf("unsupported architecture %s for pvsadm install", arch)
	}

	url := fmt.Sprintf(
		"https://github.com/ppc64le-cloud/pvsadm/releases/download/%s/pvsadm-linux-%s",
		version, releaseArch,
	)

	log.Info("downloading pvsadm", zap.String("version", version), zap.String("url", url))

	cmd := exec.Command("curl", "-sL", url, "-o", installPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("downloading pvsadm: %w", err)
	}
	if err := os.Chmod(installPath, 0755); err != nil {
		return fmt.Errorf("chmod pvsadm: %w", err)
	}
	log.Info("pvsadm installed", zap.String("path", installPath))
	return nil
}

// BuildOptions holds parameters for the qcow2→OVA conversion.
type BuildOptions struct {
	ImageName   string
	ImageURL    string
	Dist        string // rhel, centos, coreos
	ImageSizGB  int    // resultant OVA size, default 11
	TargetDisk  int    // target disk size, default 120
	RHNUser     string
	RHNPassword string
	OSPassword  string
	SkipOSPass  bool
	TempDir     string
}

// Build runs `pvsadm image qcow2ova` and returns the path to the produced
// .ova.gz file.
func (c *Client) Build(opts BuildOptions) (string, error) {
	args := []string{
		"image", "qcow2ova",
		"--image-name", opts.ImageName,
		"--image-url", opts.ImageURL,
		"--image-dist", opts.Dist,
	}
	if opts.ImageSizGB > 0 {
		args = append(args, "--image-size", fmt.Sprintf("%d", opts.ImageSizGB))
	}
	if opts.TargetDisk > 0 {
		args = append(args, "--target-disk-size", fmt.Sprintf("%d", opts.TargetDisk))
	}
	if opts.RHNUser != "" {
		args = append(args, "--rhn-user", opts.RHNUser)
	}
	if opts.RHNPassword != "" {
		args = append(args, "--rhn-password", opts.RHNPassword)
	}
	if opts.SkipOSPass {
		args = append(args, "--skip-os-password")
	} else if opts.OSPassword != "" {
		args = append(args, "--os-password", opts.OSPassword)
	}
	if opts.TempDir != "" {
		args = append(args, "--temp-dir", opts.TempDir)
	}

	c.log.Info("starting qcow2→OVA conversion",
		zap.String("image-name", opts.ImageName),
		zap.String("dist", opts.Dist),
	)

	if err := c.run(args...); err != nil {
		return "", fmt.Errorf("pvsadm qcow2ova: %w", err)
	}

	// pvsadm writes the output to CWD/<image-name>.ova.gz
	out := filepath.Join(".", opts.ImageName+".ova.gz")
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("OVA output file not found at %s after build: %w", out, err)
	}
	c.log.Info("OVA build complete", zap.String("output", out))
	return out, nil
}

// UploadOptions holds parameters for uploading to COS.
type UploadOptions struct {
	Bucket         string
	BucketRegion   string
	COSInstanceName string
	File           string
	ObjectName     string // optional, defaults to filename
}

// Upload runs `pvsadm image upload`.
func (c *Client) Upload(opts UploadOptions) error {
	args := []string{
		"image", "upload",
		"--bucket", opts.Bucket,
		"--file", opts.File,
	}
	if opts.BucketRegion != "" {
		args = append(args, "--bucket-region", opts.BucketRegion)
	}
	if opts.COSInstanceName != "" {
		args = append(args, "--cos-instance-name", opts.COSInstanceName)
	}
	if opts.ObjectName != "" {
		args = append(args, "--cos-object-name", opts.ObjectName)
	}

	c.log.Info("uploading OVA to COS",
		zap.String("bucket", opts.Bucket),
		zap.String("file", opts.File),
	)

	if err := c.run(args...); err != nil {
		return fmt.Errorf("pvsadm upload: %w", err)
	}
	c.log.Info("upload complete")
	return nil
}

// ImportOptions holds parameters for importing into PowerVS.
type ImportOptions struct {
	WorkspaceName string
	Bucket        string
	BucketRegion  string
	Object        string
	PVSImageName  string
	StorageType   string // tier1 or tier3
	AccessKey     string
	SecretKey     string
	PublicBucket  bool
}

// Import runs `pvsadm image import`.
func (c *Client) Import(opts ImportOptions) error {
	args := []string{
		"image", "import",
		"--workspace-name", opts.WorkspaceName,
		"--bucket", opts.Bucket,
		"--object", opts.Object,
		"--pvs-image-name", opts.PVSImageName,
	}
	if opts.BucketRegion != "" {
		args = append(args, "--bucket-region", opts.BucketRegion)
	}
	if opts.StorageType != "" {
		args = append(args, "--pvs-storagetype", opts.StorageType)
	}
	if opts.PublicBucket {
		args = append(args, "--public-bucket")
	} else {
		if opts.AccessKey != "" {
			args = append(args, "--accesskey", opts.AccessKey)
		}
		if opts.SecretKey != "" {
			args = append(args, "--secretkey", opts.SecretKey)
		}
	}

	c.log.Info("importing image into PowerVS",
		zap.String("workspace", opts.WorkspaceName),
		zap.String("pvs-image-name", opts.PVSImageName),
	)

	if err := c.run(args...); err != nil {
		return fmt.Errorf("pvsadm import: %w", err)
	}
	c.log.Info("PowerVS import complete", zap.String("image", opts.PVSImageName))
	return nil
}

// run executes pvsadm with the given args, streaming stdout/stderr to the
// terminal so the user sees live progress (pvsadm logs to stdout).
func (c *Client) run(args ...string) error {
	cmd := exec.Command(c.binaryPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "IBMCLOUD_API_KEY="+c.apiKey)
	return cmd.Run()
}
