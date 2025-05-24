# Configuration & Installation

There are four main requirements for configuring and deploying kproximate:

- [Proxmox API Access](#proxmox-api-access)
- [Proxmox Template](#proxmox-template)
- [Networking](#networking)
- [Installing kproximate](#)

## Proxmox API Access

The [create_kproximate_api_token.sh](https://github.com/paradoxe35/kproximate/tree/main/examples/create_proxmox_api_token.sh) script can be run on a Proxmox host to create a token with the required privileges, it will return the token ID and token which will be required later.

A custom user/token can be used, however the below privileges must be granted to it:

- Datastore.AllocateSpace
- Datastore.Audit
- Sys.Audit
- SDN.Use
- VM.Allocate
- VM.Audit
- VM.Clone
- VM.Config.Cloudinit
- VM.Config.CPU
- VM.Config.Disk
- VM.Config.Memory
- VM.Config.Network
- VM.Config.Options
- VM.Monitor
- VM.PowerMgmt

Alternatively you may use password authentication with a user that has the above permissions granted to it.

## Proxmox Template

There are two types of templates that can be created, each utilizing a different method for joining the cluster: `qemu-exec` and `first boot`.

Both methods require you to run a script on a host within the Proxmox cluster, after configuring the variables at the top of the script with values appropriate for your cluster.

Additionally, consider the VMID, as all kproximate nodes created will be assigned the next available VMIDs following that of the template.

### Join by qemu-exec (recommended)

This [create_cloud_init_template.sh](https://github.com/paradoxe35/kproximate/tree/main/examples/create_cloud_init_template.sh) script creates a (generic) template that joins the Kubernetes cluster using a command that is executed by qemu-exec.

It will create a template with the following features:

- Cloud-init enabled
- qemu-guest-agent installed
- SSH key injection
- Machine ID reset
- Cloud-init disk added
- DHCP enabled
- Virtio drivers installed
- Cloud-init user-data and meta-data files added
- Cloud-init network config added
- etc.

The benefits of this are:

- The template does not contain secrets
- The template can be reused across multiple Kubernetes clusters with any Kubernetes distribution, such as k0s, k3s, or standard Kubernetes.

If using this method you must supply the following values:

```yaml
kproximate:
  config:
    kpQemuExecJoin: true
    kpNodeTemplateName: "ubuntu-22.04-cloudinit-template" # If you chose Ubuntu Jammy
  secrets:
    kpJoinCommand: "<your-join-command>"
```

The value of `kpJoinCommand` is executed on the new node as follows: `bash -c <your-join-command>`.

### Join on first boot

The [create_kproximate_template.sh](https://github.com/paradoxe35/kproximate/tree/main/examples/create_kproximate_template.sh) script generates a template that automatically joins the Kubernetes (k3s) cluster upon first boot.

### Using local storage

To use local storage, set `kpLocalTemplateStorage: true` in your configuration and create a template on each Proxmox node, ensuring each template has the same name but a unique VMID.

Set `kpLocalTemplateStorage` to `true` if your template uses local storage rather than shared storage (such as NFS, CIFS, etc.).

### Custom templates

If creating your own template please consider the following:

- It should have `qemu-guest-agent` installed.
- It should be a cloud-init enabled image in order for ssh key injection to work.
- The final template should have a cloudinit disk added to it.
- Ensure that each time it is booted the template will generate a new machine-id. I found that this was only achieveable when truncating (and not removing) `/etc/machine-id` with the `virt-customize --truncate` command at the end of the configuration steps.
- It should be configured to receive a DHCP IP lease.
- If you are using VLANs ensure it is tagged appropriately, ie the one your kubernetes cluster resides in.

## Networking

The template should be configured to reside in the same network as your Kubernetes cluster, this can be done after it's creation in the Proxmox web gui.

This network should also provide a DHCP server so that new kproximate nodes can acquire an IP address.

Your Proxmox API endpoint should also be accessible from this network. For example, in my case I have a firewall rule between the two VLANS that my Proxmox and Kubernetes clusters are in.

## Installing kproximate

A helm chart is provided at `oci://ghcr.io/paradoxe35` for installing the application into your kubernetes cluster. See [example-values.yaml](https://github.com/paradoxe35/kproximate/tree/main/examples/example-values.yaml) for a basic configuraton example.

Install kproximate:

```bash
helm upgrade --install kproximate oci://ghcr.io/paradoxe35/kproximate -f your-values.yaml -n kproximate --create-namespace
```

See [values.yaml](https://github.com/paradoxe35/kproximate/tree/main/chart/kproximate/values.yaml) in the helm chart for the full set of configuration options and defaults.
Or [k0s-example-values.yaml](https://github.com/paradoxe35/kproximate/tree/main/examples/example-values.yaml) for an example of configuring kproximate to work with a k0s cluster.
