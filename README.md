# ovatool

A CLI tool for automating OVA image build, upload, and import for IBM Power
environments (PowerVS and PowerVC).

```
ovatool build    — convert qcow2 → OVA via pvsadm
ovatool upload   — push OVA to IBM Cloud Object Storage
ovatool import   — import from COS into PowerVS and/or PowerVC
ovatool run      — full pipeline in one command
ovatool init     — generate .env.template and optionally install pvsadm
```

---

## Prerequisites

| Requirement | Notes |
|---|---|
| ppc64le machine | LPAR or VM — the qcow2→OVA conversion mounts image partitions, so the architecture must match |
| `qemu-img` | Installed by `ovatool init --install-deps` or `dnf install -y qemu-img` |
| `cloud-utils-growpart` | Installed by `ovatool init --install-deps` or `dnf install -y cloud-utils-growpart` |
| `pvsadm` | Installed by `ovatool init --install-pvsadm` |
| `powervc-image` CLI | Required only on the PowerVC node for PowerVC imports |
| IBM Cloud API key | For pvsadm upload/import operations |
| RHN credentials | Required only for RHEL image builds |

---

## Installation

Download the binary from [Releases](https://github.com/ppc64le-cloud/ovatool/releases):

```bash
curl -sL https://github.com/ppc64le-cloud/ovatool/releases/download/v0.1.0/ovatool-linux-ppc64le \
  -o /usr/local/bin/ovatool
chmod +x /usr/local/bin/ovatool
ovatool --help
```

Or build from source:

```bash
git clone https://github.com/ppc64le-cloud/ovatool
cd ovatool
go build -o ovatool .
```

---

## Quick Start

```bash
# 1. Install all dependencies and generate config template
ovatool init --install-deps --install-pvsadm

# 2. Fill in credentials
cp .env.template .env
vi .env

# 3. Run the full pipeline
ovatool run \
  --dist centos \
  --version 9 \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --target all
```

---

## Configuration

All configuration is read from environment variables. ovatool loads `.env`
automatically if it exists in the current directory. Existing environment
variables always take precedence, so CI systems can override by exporting
variables before running the tool.

```bash
ovatool init        # writes .env.template
cp .env.template .env
vi .env             # fill in values
```

| Variable | Required | Default | Description |
|---|---|---|---|
| `IBMCLOUD_API_KEY` | Yes | — | IBM Cloud API key |
| `COS_BUCKET` | Yes | — | COS bucket name |
| `COS_BUCKET_REGION` | No | `us-south` | COS bucket region |
| `COS_INSTANCE_NAME` | No | — | COS instance name |
| `COS_ACCESS_KEY` | For private import | — | COS HMAC access key |
| `COS_SECRET_KEY` | For private import | — | COS HMAC secret key |
| `PVS_WORKSPACE_NAME` | For PVS import | — | PowerVS workspace name |
| `PVS_STORAGE_TYPE` | No | `tier1` | `tier1` or `tier3` |
| `POWERVC_HOST` | For PowerVC | — | PowerVC hostname or IP only — no scheme or port (e.g. `mypowervc.example.com`) |
| `POWERVC_USERNAME` | For PowerVC | — | PowerVC username |
| `POWERVC_PASSWORD` | For PowerVC | — | PowerVC password |
| `POWERVC_PROJECT` | No | `ibm-default` | PowerVC project |
| `POWERVC_STORAGE_TEMPLATE_ID` | For PowerVC | — | Storage template UUID |
| `PVSADM_VERSION` | No | `v0.1.15` | Pinned pvsadm version |
| `TEMP_DIR` | No | `/tmp` | Scratch space for OVA conversion (~250 GB needed) |

---

## Usage

ovatool is designed to be flexible — you can run the full pipeline end-to-end
with a single command, or execute each stage independently depending on your
workflow. The sections below cover every supported combination.

---

### Stage overview

| Stage | Command | What it does |
|---|---|---|
| Build | `ovatool build` | Converts a `.qcow2` image to `.ova.gz` format using pvsadm |
| Upload | `ovatool upload` | Pushes a local `.ova.gz` to an IBM Cloud Object Storage bucket |
| Import (PowerVS) | `ovatool import --target pvs` | Imports the OVA from COS into a PowerVS workspace |
| Import (PowerVC) | `ovatool import --target powervc` | Imports a local OVA into PowerVC (must run on the PowerVC node) |
| Full pipeline | `ovatool run` | Orchestrates any combination of the above stages in one command |

---

### 1. Build only

Use this when you want to convert a cloud image to OVA format and handle the
upload and import steps separately later.

The `--image-url` flag accepts either a **local file path** or a **direct
HTTPS URL**. When a URL is provided, pvsadm downloads the image automatically
before conversion — no separate download step is needed.

```bash
# Using a local file
ovatool build \
  --dist centos \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --version 9

# Using a direct URL (pvsadm downloads the image before converting)
ovatool build \
  --dist centos \
  --image-url https://cloud.centos.org/centos/9-stream/ppc64le/images/CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --version 9
```

For RHEL images, Red Hat subscription credentials are required:

```bash
ovatool build \
  --dist rhel \
  --image-url ./rhel-9.5-ppc64le-kvm.qcow2 \
  --version 9.5 \
  --rhn-user user@example.com \
  --rhn-password secret
```

If the build fails because the pvsadm prep script cannot reach `9.9.9.9`
(the default DNS hardcoded by pvsadm), provide your VM's working nameserver:

```bash
ovatool build \
  --dist centos \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --version 9 \
  --nameserver 9.3.1.200
```

To use a fully customised prep script instead of the pvsadm default:

```bash
ovatool build \
  --dist centos \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --version 9 \
  --prep-template ./image-prep.template
```

---

### 2. Upload only

Use this when the OVA has already been built and you only need to push it to
IBM Cloud Object Storage. A preflight check ensures no object with the same
name already exists in the bucket before the upload begins.

```bash
ovatool upload --file centos-9-23052026.ova.gz
```

---

### 3. Import only

#### Import into PowerVS

Use this when the OVA is already in your COS bucket (either uploaded
previously or sourced from a public bucket such as RHCOS prebuilt images).

```bash
ovatool import --target pvs \
  --object centos-9-23052026.ova.gz \
  --pvs-image-name centos-9-23052026
```

To import from the public RHCOS bucket (no COS credentials required):

```bash
ovatool import --target pvs \
  --object rhcos-419-ppc64le-powervs.ova.gz \
  --pvs-image-name rhcos-419-23052026 \
  --public-bucket
```

#### Import into PowerVC

PowerVC imports require the `powervc-image` CLI, which is only available on
the PowerVC management node. Copy the OVA and the ovatool binary to that node
and run the import there.

```bash
# On the PowerVC node
ovatool import --target powervc \
  --image-path /root/centos-9-23052026.ova.gz \
  --pvc-image-name centos-9-23052026 \
  --os-type rhel
```

#### Import into both PowerVS and PowerVC simultaneously

```bash
ovatool import --target all \
  --object centos-9-23052026.ova.gz \
  --pvs-image-name centos-9-23052026 \
  --image-path ./centos-9-23052026.ova.gz \
  --pvc-image-name centos-9-23052026
```

---

### 4. Build and upload (import later)

Use this when you want to prepare and stage the image in COS, but defer the
import to a separate step or a separate node.

```bash
ovatool run \
  --dist centos \
  --version 9 \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --target build,upload
```

---

### 5. Upload and import (image already built)

Use this when the OVA file already exists locally and you want to upload it
to COS and immediately import it into PowerVS in one step.

```bash
ovatool run \
  --dist centos \
  --image-name centos-9-23052026 \
  --target upload,import-pvs
```

---

### 6. Full pipeline — build, upload, and import

Runs all stages in sequence: converts the qcow2 image to OVA, uploads it to
COS, and imports it into PowerVS and PowerVC.

```bash
ovatool run \
  --dist centos \
  --version 9 \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --target all
```

For RHEL with subscription credentials:

```bash
ovatool run \
  --dist rhel \
  --version 9.5 \
  --image-url ./rhel-9.5-ppc64le-kvm.qcow2 \
  --rhn-user user@example.com \
  --rhn-password secret \
  --target all
```

---

### 7. RHCOS — fetch and import (no build required)

RHCOS OVA images are prebuilt by Red Hat and published to a public COS bucket.
There is no build step — use `ovatool fetch` to download the image, then
import it directly.

```bash
# Download the latest RHCOS OVA for OpenShift 4.19
ovatool fetch --ocp-version 4.19

# Import into PowerVS from the public bucket
ovatool import --target pvs \
  --object rhcos-419-ppc64le-powervs.ova.gz \
  --pvs-image-name rhcos-419-23052026 \
  --public-bucket
```

To list all available RHCOS releases before downloading:

```bash
ovatool fetch --list
ovatool fetch --list --ocp-version 4.19
```

---

## Naming Convention

Auto-generated names follow the project convention: `<dist>-<version>-<ddmmyyyy>`

```
rhel-95-23052025
centos-9-23052025
rhcos-419-23052025
```

Pass `--image-name` to override with a custom name.

---

## Preflight Checks

**Build stage** — ovatool checks for `qemu-img` and `growpart` before invoking pvsadm. If either is missing it exits immediately with a clear install hint rather than failing inside the conversion:

```
preflight failed: missing required system tools
  ✗ qemu-img not found
    required by: build stage (qcow2 → raw conversion)
    fix: dnf install -y qemu-img

  or install all at once: ovatool init --install-deps
```

**Upload stage** — ovatool checks that no object with the same name already exists in the COS bucket before starting a potentially long upload. Pass `--skip-preflight` to bypass this check.

---

## Logging

By default ovatool emits human-readable coloured logs to stdout.

```bash
# JSON logs (for CI / log aggregation)
ovatool --json-logs run ...

# Also write logs to a file
ovatool --log-file run-2025-05-23.log run ...
```

---

## Jenkins Integration

Because ovatool is a self-contained binary, Jenkins just needs to download it
and run it. Credentials are injected via Jenkins Credential Bindings as
environment variables — the same names as the `.env` file, no code changes needed.

```groovy
pipeline {
  agent { label 'ppc64le' }
  environment {
    IBMCLOUD_API_KEY  = credentials('ibmcloud-api-key')
    COS_ACCESS_KEY    = credentials('cos-access-key')
    COS_SECRET_KEY    = credentials('cos-secret-key')
    PVS_WORKSPACE_NAME = 'rdr-ocp-cicd-montreal01'
    COS_BUCKET        = 'ocp4-images-bucket'
    COS_BUCKET_REGION = 'us-south'
    PVS_STORAGE_TYPE  = 'tier1'
    PVSADM_VERSION    = 'v0.1.15'
  }
  stages {
    stage('Install ovatool') {
      steps {
        sh '''
          curl -sL https://github.com/ppc64le-cloud/ovatool/releases/download/v0.1.0/ovatool-linux-ppc64le \
            -o /usr/local/bin/ovatool
          chmod +x /usr/local/bin/ovatool
          ovatool init --install-deps --install-pvsadm
        '''
      }
    }
    stage('Build & Upload') {
      steps {
        sh '''
          ovatool run \
            --dist centos \
            --version 9 \
            --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
            --target build,upload \
            --json-logs \
            --log-file ovatool-run.log
        '''
      }
    }
  }
}
```

---

## Project Structure

```
ovatool/
├── main.go
├── cmd/
│   ├── root.go          # cobra root, global flags, logger, env loading
│   ├── init.go          # generates .env.template, installs pvsadm
│   ├── build.go         # qcow2 → OVA conversion
│   ├── upload.go        # OVA → COS
│   ├── import.go        # COS → PowerVS / PowerVC
│   └── run.go           # full pipeline orchestrator
├── pkg/
│   ├── config/          # env loading, .env template
│   ├── deps/            # system dependency checks and installation
│   ├── image/           # dist types, naming, target parsing
│   ├── pvsadm/          # pvsadm binary wrapper
│   ├── powervc/         # powervc-image binary wrapper
│   └── cos/             # COS preflight checks
├── .env.template        # committed — fill and rename to .env
├── .gitignore
├── go.mod
└── README.md
```
