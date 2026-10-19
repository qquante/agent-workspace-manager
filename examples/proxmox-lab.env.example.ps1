# Copy the non-secret values you need into your shell before running the lab script.
# Do not commit real token files.

$env:PVE_ENDPOINT = "https://PVE_HOST:8006"
$env:PVE_SSH_HOST = "PVE_HOST"
$env:PVE_SSH_USER = "root"
$env:PVE_NODE = "pve"
$env:PVE_API_USER = "stola@pve"
$env:PVE_TOKEN_ID = "stola-lab"
$env:PVE_TOKEN_FILE = ".\tmp\pve.token"
$env:PVE_INSECURE_SKIP_TLS_VERIFY = "true"

# Later, after `proxmox check` passes:
$env:PVE_STORAGE = "local-lvm"
$env:PVE_TEMPLATE = "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
$env:PVE_BRIDGE = "vmbr0"
$env:PVE_VMID = "101"
