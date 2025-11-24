#!/bin/bash
set -e
set -u

# --- Configuration ---
CLUSTER_NAME="kube-farm"
CLUSTER_ENDPOINT="lk8s.io"
NODE_IP="192.168.1.248"
REGISTRY_IP="10.104.181.19"

# Files
SECRETS_FILE="secrets.yaml"
CONFIG_FILE="controlplane.yaml"
LOCAL_TALOSCONFIG="talosconfig"
PATCH_MACHINE="machineconfig.patch.yaml"
EXTRA_VOLUMES="volumes.patch.yaml"

# --- Helpers ---
log() { echo -e "\033[1;32m[INFO]\033[0m $1"; }
warn() { echo -e "\033[1;33m[WARN]\033[0m $1"; }
error() { echo -e "\033[1;31m[ERROR]\033[0m $1"; exit 1; }

# --- Safety Check: Existing Files ---
# Checks if any generated files exist and asks to overwrite
if [[ -f "$SECRETS_FILE" ]] || [[ -f "$CONFIG_FILE" ]] || [[ -f "$LOCAL_TALOSCONFIG" ]]; then
    warn "Found existing configuration files in this directory:"
    [[ -f "$SECRETS_FILE" ]] && echo " - $SECRETS_FILE"
    [[ -f "$CONFIG_FILE" ]] && echo " - $CONFIG_FILE"
    [[ -f "$LOCAL_TALOSCONFIG" ]] && echo " - $LOCAL_TALOSCONFIG"
    
    echo ""
    read -p "Do you want to delete these files and regenerate the cluster config? (y/N): " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        error "Aborting execution to protect existing files."
    fi
    
    log "Removing old configuration files..."
    rm -f "$SECRETS_FILE" "$CONFIG_FILE" "$LOCAL_TALOSCONFIG"
fi

# --- Pre-flight Checks ---
command -v talosctl >/dev/null 2>&1 || error "talosctl is required."
[[ -f "$PATCH_MACHINE" ]] || error "$PATCH_MACHINE not found."
[[ -f "$EXTRA_VOLUMES" ]] || error "$EXTRA_VOLUMES not found."

# --- Execution ---

log "1. Generating Secrets..."
talosctl gen secrets -o "$SECRETS_FILE"

log "2. Generating Configuration..."
# Using NODE_IP to ensure validation passes
talosctl gen config \
    --with-secrets "$SECRETS_FILE" \
    --with-docs=false \
    --with-examples=false \
    "$CLUSTER_NAME" \
    "https://$NODE_IP:6443" \
    --registry-mirror "local-registry.io=http://${REGISTRY_IP}:5000" \
    --config-patch-control-plane "@$PATCH_MACHINE" \
    --output-types controlplane,talosconfig

log "3. Appending Extra Volumes..."
cat "$EXTRA_VOLUMES" >> "$CONFIG_FILE"

log "----------------------------------------"
log "POINT OF NO RETURN: Applying to $NODE_IP"
log "----------------------------------------"
sleep 2

talosctl apply-config --insecure -n "$NODE_IP" --file "$CONFIG_FILE"

log "4. Configuring local talosctl context..."
talosctl config endpoint "$NODE_IP" "$CLUSTER_ENDPOINT" --talosconfig "./talosconfig"
talosctl config node "$NODE_IP" --talosconfig "./talosconfig"
cp ./talosconfig ~/.talos/config
export TALOSCONFIG=~/.talos/config

log "5. Bootstrapping Cluster..."
# Loop: Hides errors (2>/dev/null) but prints dots so you know it's working.
MAX_RETRIES=30
COUNT=1 

until talosctl bootstrap --nodes "$NODE_IP" 2>/dev/null; do
    if [ $COUNT -ge $MAX_RETRIES ]; then
        error "Timed out waiting for node to accept bootstrap."
    fi
    echo -n "."
    sleep 5
    ((COUNT++))
done
echo ""
log "Bootstrap command accepted."

log "6. Retrieving Kubeconfig..."
talosctl kubeconfig --nodes "$NODE_IP" --force

log "7. Running Health Checks..."
log "(Output will stream below. This may take a few minutes.)"
log "----------------------------------------"

# Streams output live to the terminal
talosctl health --nodes "$NODE_IP" --wait-timeout 10m

log "----------------------------------------"
log "SUCCESS: Cluster $CLUSTER_NAME is ready."
log "----------------------------------------"

