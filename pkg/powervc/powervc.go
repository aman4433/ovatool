package powervc

import (
	"fmt"
	"os"
	"os/exec"

	"go.uber.org/zap"
)

// Client wraps the powervc-image CLI.
type Client struct {
	log      *zap.Logger
	host     string
	username string
	password string
	project  string
}

// NewClient returns a PowerVC client.
func NewClient(log *zap.Logger, host, username, password, project string) *Client {
	return &Client{
		log:      log,
		host:     host,
		username: username,
		password: password,
		project:  project,
	}
}

// ImportOptions holds parameters for powervc-image create import.
type ImportOptions struct {
	ImageName         string
	ImagePath         string
	StorageTemplateID string
	OSType            string // rhel or coreos
}

// Import runs `powervc-image create import` to import an OVA into PowerVC.
func (c *Client) Import(opts ImportOptions) error {
	if err := c.checkBinary(); err != nil {
		return err
	}

	args := []string{
		"create",
		"--project", c.project,
		"import",
		"-n", opts.ImageName,
		"-p", opts.ImagePath,
		"-t", opts.StorageTemplateID,
		"-m", fmt.Sprintf("os-type=%s", opts.OSType),
	}

	c.log.Info("importing image into PowerVC",
		zap.String("image-name", opts.ImageName),
		zap.String("os-type", opts.OSType),
		zap.String("storage-template", opts.StorageTemplateID),
	)

	cmd := exec.Command("powervc-image", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// powervc-image picks up credentials from OS_USERNAME / OS_PASSWORD and
	// the sourced powervcrc. We inject them explicitly so the user doesn't
	// need to source anything manually.
	cmd.Env = append(os.Environ(),
		"OS_USERNAME="+c.username,
		"OS_PASSWORD="+c.password,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("powervc-image import: %w", err)
	}
	c.log.Info("PowerVC import complete", zap.String("image", opts.ImageName))
	return nil
}

// SourcePowerVCRC sources /opt/ibm/powervc/powervcrc by reading its exports
// and setting them in the current process environment. This mirrors what
// `source /opt/ibm/powervc/powervcrc` does in shell.
func SourcePowerVCRC() error {
	const rcPath = "/opt/ibm/powervc/powervcrc"
	data, err := os.ReadFile(rcPath)
	if os.IsNotExist(err) {
		return nil // not running on a PowerVC node, skip
	}
	if err != nil {
		return fmt.Errorf("reading powervcrc: %w", err)
	}

	for _, line := range splitLines(string(data)) {
		// Handle lines like: export KEY=VALUE
		var key, val string
		if n, _ := fmt.Sscanf(line, "export %s", &key); n == 1 {
			if idx := indexOf(key, '='); idx >= 0 {
				val = key[idx+1:]
				key = key[:idx]
				if os.Getenv(key) == "" {
					os.Setenv(key, val)
				}
			}
		}
	}
	return nil
}

func (c *Client) checkBinary() error {
	if _, err := exec.LookPath("powervc-image"); err != nil {
		return fmt.Errorf("powervc-image binary not found — this step must run on a PowerVC node")
	}
	return nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
