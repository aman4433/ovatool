// ovatool parameterized pipeline
//
// Trigger via "Build with Parameters" in Jenkins. Check only the stages you
// need — unchecked stages are skipped entirely and shown as grey in the UI.
//
// Credentials required in Jenkins credential store:
//   ibmcloud-api-key   — IBM Cloud API key
//   cos-access-key     — COS HMAC access key
//   cos-secret-key     — COS HMAC secret key
//   rhn-username       — Red Hat subscription username (RHEL builds only)
//   rhn-password       — Red Hat subscription password (RHEL builds only)
//   powervc-password   — PowerVC user password (PowerVC imports only)

pipeline {
  agent { label 'ppc64le' }

  parameters {

    // ── Image identity ──────────────────────────────────────────────────────
    choice(
      name: 'DIST',
      choices: ['centos', 'rhel', 'rhcos', 'other'],
      description: 'Image distribution to build. rhcos skips the build stage — OVAs are prebuilt by Red Hat. Select "other" and fill in DIST_OTHER for any unlisted distribution.'
    )
    string(
      name: 'DIST_OTHER',
      defaultValue: '',
      description: 'Custom distribution name passed to --dist when DIST is set to "other" (e.g. ubuntu, sles).'
    )
    string(
      name: 'VERSION',
      defaultValue: '',
      description: 'Image version (e.g. 9, 9.5, 4.19). Used to auto-generate the image name if IMAGE_NAME is left blank.'
    )
    string(
      name: 'IMAGE_NAME',
      defaultValue: '',
      description: 'Override the auto-generated image name. Leave blank to let ovatool generate one from DIST and VERSION.'
    )

    // ── qcow2 source — provide exactly one ─────────────────────────────────
    string(
      name: 'IMAGE_URL',
      defaultValue: '',
      description: 'URL or local agent path to the source qcow2 image (e.g. https://cloud.centos.org/.../image.qcow2). Leave blank if using IMAGE_COS_OBJECT.'
    )
    string(
      name: 'IMAGE_COS_OBJECT',
      defaultValue: '',
      description: 'COS object key of the source qcow2 image already uploaded to COS (e.g. centos-9-latest.qcow2). Alternative to IMAGE_URL — leave blank if using IMAGE_URL.'
    )
    string(
      name: 'IMAGE_COS_BUCKET',
      defaultValue: '',
      description: 'COS bucket containing the source qcow2. Leave blank to use the same bucket as OVA uploads (COS_BUCKET).'
    )

    // ── Build options ───────────────────────────────────────────────────────
    string(
      name: 'NAMESERVER',
      defaultValue: '',
      description: 'DNS nameserver to inject into the pvsadm prep script (e.g. 9.3.1.200). Leave blank if 9.9.9.9 is reachable from the build agent.'
    )

    // ── Stages to run ───────────────────────────────────────────────────────
    booleanParam(
      name: 'BUILD',
      defaultValue: true,
      description: 'Convert the qcow2 image to OVA format. Automatically skipped for rhcos.'
    )
    booleanParam(
      name: 'UPLOAD',
      defaultValue: true,
      description: 'Upload the OVA to IBM Cloud Object Storage.'
    )
    booleanParam(
      name: 'IMPORT_PVS',
      defaultValue: true,
      description: 'Import the OVA from COS into the PowerVS workspace.'
    )
    booleanParam(
      name: 'IMPORT_POWERVC',
      defaultValue: false,
      description: 'Copy the OVA to the PowerVC node via SSH and import it into PowerVC.'
    )

    // ── Import names ────────────────────────────────────────────────────────
    string(
      name: 'PVS_IMAGE_NAME',
      defaultValue: '',
      description: 'Name to register the image under in PowerVS. Defaults to IMAGE_NAME if blank.'
    )
    string(
      name: 'PVC_IMAGE_NAME',
      defaultValue: '',
      description: 'Name to register the image under in PowerVC. Defaults to IMAGE_NAME if blank.'
    )

    // ── PowerVC node (only needed when IMPORT_POWERVC is checked) ───────────
    string(
      name: 'POWERVC_NODE',
      defaultValue: '',
      description: 'Hostname or IP of the PowerVC management node. Required only when IMPORT_POWERVC is checked.'
    )

    // ── COS config ──────────────────────────────────────────────────────────
    string(
      name: 'COS_BUCKET',
      defaultValue: 'ocp4-images-bucket',
      description: 'COS bucket to upload OVAs into and import from.'
    )
    string(
      name: 'COS_BUCKET_REGION',
      defaultValue: 'us-south',
      description: 'IBM Cloud region of the COS bucket (e.g. us-south, eu-de, jp-tok).'
    )

    // ── PowerVS config ──────────────────────────────────────────────────────
    string(
      name: 'PVS_WORKSPACE_NAME',
      defaultValue: '',
      description: 'PowerVS workspace name to import the image into (e.g. rdr-ocp-cicd-montreal01).'
    )
    choice(
      name: 'PVS_STORAGE_TYPE',
      choices: ['tier1', 'tier3'],
      description: 'PowerVS storage tier for the imported image. tier1 = NVMe (faster), tier3 = SSD.'
    )

    // ── PowerVC config (only needed when IMPORT_POWERVC is checked) ─────────
    string(
      name: 'POWERVC_HOST',
      defaultValue: '',
      description: 'PowerVC hostname or IP — no scheme or port (e.g. mypowervc.example.com). Required only when IMPORT_POWERVC is checked.'
    )
    string(
      name: 'POWERVC_USERNAME',
      defaultValue: 'admin',
      description: 'PowerVC username. Required only when IMPORT_POWERVC is checked.'
    )
    string(
      name: 'POWERVC_PROJECT',
      defaultValue: 'ibm-default',
      description: 'PowerVC project (OpenStack project/tenant). Required only when IMPORT_POWERVC is checked.'
    )
    string(
      name: 'POWERVC_STORAGE_TEMPLATE_ID',
      defaultValue: '',
      description: 'PowerVC storage template UUID. Required only when IMPORT_POWERVC is checked.'
    )
  }

  environment {
    // Credentials — set these up in Jenkins > Manage Credentials
    IBMCLOUD_API_KEY  = credentials('ibmcloud-api-key')
    COS_ACCESS_KEY    = credentials('cos-access-key')
    COS_SECRET_KEY    = credentials('cos-secret-key')
    RHN_USER          = credentials('rhn-username')
    RHN_PASSWORD      = credentials('rhn-password')
    POWERVC_PASSWORD  = credentials('powervc-password')

    // Promote params to env vars so ovatool picks them up automatically
    COS_BUCKET                  = "${params.COS_BUCKET}"
    COS_BUCKET_REGION           = "${params.COS_BUCKET_REGION}"
    PVS_WORKSPACE_NAME          = "${params.PVS_WORKSPACE_NAME}"
    PVS_STORAGE_TYPE            = "${params.PVS_STORAGE_TYPE}"
    POWERVC_HOST                = "${params.POWERVC_HOST}"
    POWERVC_USERNAME            = "${params.POWERVC_USERNAME}"
    POWERVC_PROJECT             = "${params.POWERVC_PROJECT}"
    POWERVC_STORAGE_TEMPLATE_ID = "${params.POWERVC_STORAGE_TEMPLATE_ID}"
  }

  stages {

    stage('Setup') {
      steps {
        sh '''
          curl -sL https://github.com/aman4433/ovatool/releases/download/v0.1.0/ovatool-linux-ppc64le \
            -o /usr/local/bin/ovatool
          chmod +x /usr/local/bin/ovatool
          ovatool init --install-deps --install-pvsadm
        '''
      }
    }

    stage('Build') {
      // Skipped automatically for rhcos (prebuilt OVAs) or if BUILD is unchecked
      when { expression {
        def effectiveDist = (params.DIST == 'other') ? params.DIST_OTHER : params.DIST
        return params.BUILD && effectiveDist != 'rhcos'
      }}
      steps {
        sh """
          DIST_VAL='${params.DIST == 'other' ? params.DIST_OTHER : params.DIST}'
          ovatool build \
            --dist "\$DIST_VAL" \
            ${params.IMAGE_URL        ? "--image-url '${params.IMAGE_URL}'"                   : ''} \
            ${params.IMAGE_COS_OBJECT ? "--image-cos-object '${params.IMAGE_COS_OBJECT}'"     : ''} \
            ${params.IMAGE_COS_BUCKET ? "--image-cos-bucket '${params.IMAGE_COS_BUCKET}'"     : ''} \
            ${params.VERSION          ? "--version '${params.VERSION}'"                       : ''} \
            ${params.IMAGE_NAME       ? "--image-name '${params.IMAGE_NAME}'"                 : ''} \
            ${params.NAMESERVER       ? "--nameserver '${params.NAMESERVER}'"                 : ''} \
            ${params.DIST == 'rhel'   ? "--rhn-user '\$RHN_USER' --rhn-password '\$RHN_PASSWORD'" : ''}
        """
      }
    }

    stage('Upload') {
      when { expression { return params.UPLOAD } }
      steps {
        sh 'ovatool upload --file $(ls *.ova.gz | head -1)'
      }
    }

    stage('Import PowerVS') {
      when { expression { return params.IMPORT_PVS } }
      steps {
        sh """
          ovatool import --target pvs \
            --object \$(ls *.ova.gz | head -1) \
            --pvs-image-name '${params.PVS_IMAGE_NAME ?: params.IMAGE_NAME}'
        """
      }
    }

    stage('Import PowerVC') {
      // Copies the OVA to the PowerVC node via SSH then runs ovatool there.
      // The ovatool binary must already be present on the PowerVC node at /root/ovatool.
      when { expression { return params.IMPORT_POWERVC && params.POWERVC_NODE != '' } }
      steps {
        sh """
          OVA=\$(ls *.ova.gz | head -1)
          scp \$OVA root@${params.POWERVC_NODE}:/root/
          ssh root@${params.POWERVC_NODE} \
            "POWERVC_PASSWORD='\$POWERVC_PASSWORD' \
             /root/ovatool import --target powervc \
              --image-path /root/\$OVA \
              --pvc-image-name '${params.PVC_IMAGE_NAME ?: params.IMAGE_NAME}'"
        """
      }
    }
  }

  post {
    success {
      echo "Pipeline complete. Check the build log for the wiki update section to paste into the team wiki page."
    }
    failure {
      echo "Pipeline failed — review the stage logs above for details."
    }
  }
}
