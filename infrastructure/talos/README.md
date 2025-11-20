#Quickstart - need to move to script

talosctl gen secrets -o secrets.yaml
talosctl gen config \
	--with-secrets secrets.yaml \
    --with-docs=false \
    --with-examples=false \
    $CLUSTER_NAME \
    https://$YOUR_ENDPOINT:6443 \
    --output-types controlplane # can be a comma separated list
talosctl machineconfig patch controlplane.yaml \
	--patch @machineconfig.patch.yaml \
    --output controlplane.yaml
cat volumes.patch.yaml >> controlplane.yaml
## Point of no return
talosctl apply-config --insecure -n lk8s.io --file controlplane.yaml

talosctl config endpoint 192.168.1.248 lk8s.io 
cp ./talosconfig ~/.talos/config
export TALOSCONFIG=~/.talos/config

#### see if you can use DNS eventually
talosctl bootstrap --nodes 192.168.1.248
talosctl kubeconfig --nodes 192.168.1.248
