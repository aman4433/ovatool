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
| `qemu-img` | `dnf install -y qemu-img` |
| `cloud-utils-growpart` | `dnf install -y cloud-utils-growpart` |
| `pvsadm` | Installed by `ovatool init --install-pvsadm` |
| `ibmcloud` CLI | Required for COS preflight checks |
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
# 1. Generate config template
ovatool init --install-pvsadm

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
| `POWERVC_HOST` | For PowerVC | — | PowerVC host |
| `POWERVC_USERNAME` | For PowerVC | — | PowerVC username |
| `POWERVC_PASSWORD` | For PowerVC | — | PowerVC password |
| `POWERVC_PROJECT` | No | `ibm-default` | PowerVC project |
| `POWERVC_STORAGE_TEMPLATE_ID` | For PowerVC | — | Storage template UUID |
| `PVSADM_VERSION` | No | `v0.1.15` | Pinned pvsadm version |
| `TEMP_DIR` | No | `/tmp` | Scratch space for OVA conversion (~250 GB needed) |

---

## Usage

### `ovatool run` — full pipeline

```bash
# CentOS — full pipeline
ovatool run \
  --dist centos \
  --version 9 \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --target all

# RHEL — build and upload only
ovatool run \
  --dist rhel \
  --version 9.5 \
  --image-url ./rhel-9.5-ppc64le-kvm.qcow2 \
  --rhn-user user@example.com \
  --rhn-password secret \
  --target build,upload

# RHCOS — import only from public bucket (no build needed)
ovatool run \
  --dist rhcos \
  --image-name rhcos-419-23052025 \
  --object rhcos-419-ppc64le-powervs.ova.gz \
  --public-bucket \
  --target import-pvs
```

### `ovatool build`

```bash
ovatool build \
  --dist centos \
  --image-url ./CentOS-Stream-GenericCloud-9-latest.ppc64le.qcow2 \
  --version 9
```

### `ovatool upload`

```bash
ovatool upload --file centos-9-23052025.ova.gz
```

### `ovatool import`

```bash
# PowerVS
ovatool import \
  --target pvs \
  --object centos-9-23052025.ova.gz \
  --pvs-image-name centos-9-23052025

# PowerVC
ovatool import \
  --target powervc \
  --image-path ./centos-9-23052025.ova.gz \
  --pvc-image-name centos-9-23052025 \
  --os-type rhel

# Both
ovatool import \
  --target all \
  --object centos-9-23052025.ova.gz \
  --pvs-image-name centos-9-23052025 \
  --image-path ./centos-9-23052025.ova.gz \
  --pvc-image-name centos-9-23052025
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

## Idempotency

ovatool runs a preflight check before upload to assert that no object with
the same name already exists in the COS bucket. The pipeline will exit early
with a clear error rather than failing 30 minutes into a build.

Pass `--skip-preflight` to the `upload` command to bypass this check.

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
│   ├── image/           # dist types, naming, target parsing
│   ├── pvsadm/          # pvsadm binary wrapper
│   ├── powervc/         # powervc-image binary wrapper
│   └── cos/             # COS preflight checks
├── .env.template        # committed — fill and rename to .env
├── .gitignore
├── go.mod
└── README.md
```
