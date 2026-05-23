package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const EnvFile = ".env"

// Config holds all runtime configuration sourced from env vars or flags.
type Config struct {
	// IBM Cloud
	IBMCloudAPIKey string

	// COS
	COSBucket       string
	COSBucketRegion string
	COSInstanceName string
	COSAccessKey    string
	COSSecretKey    string

	// PowerVS
	PVSWorkspaceName string
	PVSStorageType   string // tier1 or tier3

	// PowerVC
	PowerVCHost              string
	PowerVCUsername          string
	PowerVCPassword          string
	PowerVCProject           string
	PowerVCStorageTemplateID string

	// pvsadm
	PVSADMVersion string

	// Image build
	TempDir string
}

// envTemplate is written by `ovatool init`.
const envTemplate = `# ovatool environment configuration
# Copy this file to .env, fill in the values, and ovatool will source it automatically.
# NEVER commit .env to git.

# ── IBM Cloud ──────────────────────────────────────────────────────────────────
IBMCLOUD_API_KEY=

# ── Cloud Object Storage ───────────────────────────────────────────────────────
COS_BUCKET=ocp4-images-bucket
COS_BUCKET_REGION=us-south
COS_INSTANCE_NAME=
# Access key and secret key are required for private bucket imports.
COS_ACCESS_KEY=
COS_SECRET_KEY=

# ── PowerVS ────────────────────────────────────────────────────────────────────
PVS_WORKSPACE_NAME=
PVS_STORAGE_TYPE=tier1

# ── PowerVC ────────────────────────────────────────────────────────────────────
# Hostname or IP only — no scheme or port (e.g. mypowervc.com)
# ovatool builds the full auth URL as: https://<POWERVC_HOST>:5000/v3/
POWERVC_HOST=
POWERVC_USERNAME=
POWERVC_PASSWORD=
POWERVC_PROJECT=ibm-default
# Run: powervc-image list-storage-templates to get this ID
POWERVC_STORAGE_TEMPLATE_ID=

# ── pvsadm ─────────────────────────────────────────────────────────────────────
# Pin a specific pvsadm version. See: https://github.com/ppc64le-cloud/pvsadm/tags
PVSADM_VERSION=v0.1.15

# ── Build ──────────────────────────────────────────────────────────────────────
# Temporary directory used during OVA conversion (needs ~250 GB free).
TEMP_DIR=/tmp
`

// WriteEnvTemplate writes .env.template to the current directory.
func WriteEnvTemplate() error {
	path := ".env.template"
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists, not overwriting", path)
	}
	return os.WriteFile(path, []byte(envTemplate), 0644)
}

// LoadEnvFile reads key=value pairs from .env if it exists and sets them
// as environment variables (existing env vars take precedence).
func LoadEnvFile() error {
	f, err := os.Open(EnvFile)
	if os.IsNotExist(err) {
		return nil // .env is optional
	}
	if err != nil {
		return fmt.Errorf("opening %s: %w", EnvFile, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Existing env var wins (allows CI to override via exported vars).
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	return scanner.Err()
}

// FromEnv populates Config from environment variables.
func FromEnv() *Config {
	return &Config{
		IBMCloudAPIKey:           os.Getenv("IBMCLOUD_API_KEY"),
		COSBucket:                os.Getenv("COS_BUCKET"),
		COSBucketRegion:          getEnvOrDefault("COS_BUCKET_REGION", "us-south"),
		COSInstanceName:          os.Getenv("COS_INSTANCE_NAME"),
		COSAccessKey:             os.Getenv("COS_ACCESS_KEY"),
		COSSecretKey:             os.Getenv("COS_SECRET_KEY"),
		PVSWorkspaceName:         os.Getenv("PVS_WORKSPACE_NAME"),
		PVSStorageType:           getEnvOrDefault("PVS_STORAGE_TYPE", "tier1"),
		PowerVCHost:              os.Getenv("POWERVC_HOST"),
		PowerVCUsername:          os.Getenv("POWERVC_USERNAME"),
		PowerVCPassword:          os.Getenv("POWERVC_PASSWORD"),
		PowerVCProject:           getEnvOrDefault("POWERVC_PROJECT", "ibm-default"),
		PowerVCStorageTemplateID: os.Getenv("POWERVC_STORAGE_TEMPLATE_ID"),
		PVSADMVersion:            getEnvOrDefault("PVSADM_VERSION", "v0.1.15"),
		TempDir:                  getEnvOrDefault("TEMP_DIR", "/tmp"),
	}
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
