#!/bin/bash

# Function to get available storage
get_available_storage() {
    pvesm status -content images | awk '{if(NR>1)print $1}'
}

# Function to get next available VM ID
get_next_vmid() {
    pvesh get /cluster/nextid
}

# Function to get available bridges
get_available_bridges() {
    ls /sys/class/net | grep -E '^vmbr[0-9]+'
}

# Function to remove quotation marks from a string
clean_string() {
    echo "${1//\"/}"
}

# Function to prompt for required input using whiptail
whiptail_prompt_required() {
    local title="$1"
    local text="$2"
    local default="$3"
    local result

    while true; do
        result=$(whiptail --inputbox "$text" 8 78 "$default" --title "$title" 3>&1 1>&2 2>&3)
        if [ $? -ne 0 ]; then
            echo "User cancelled the operation." >&2
            exit 1
        elif [ -n "$result" ]; then
            echo $(clean_string "$result")
            break
        else
            whiptail --msgbox "This field is required. Please enter a value." 8 78
        fi
    done
}

# Function to prompt for password with confirmation
whiptail_prompt_password() {
    local title="$1"
    local text="$2"
    local password
    local confirm_password

    while true; do
        password=$(whiptail --passwordbox "$text" 8 78 --title "$title" 3>&1 1>&2 2>&3)
        if [ $? -ne 0 ]; then
            echo "User cancelled the operation." >&2
            exit 1
        fi

        if [ -z "$password" ]; then
            whiptail --msgbox "Password cannot be empty. Please try again." 8 78
            continue
        fi

        confirm_password=$(whiptail --passwordbox "Confirm password:" 8 78 --title "$title" 3>&1 1>&2 2>&3)
        if [ $? -ne 0 ]; then
            echo "User cancelled the operation." >&2
            exit 1
        fi

        if [ -z "$confirm_password" ]; then
            whiptail --msgbox "Confirmation password cannot be empty. Exiting." 8 78
            echo "No confirmation password provided. Exiting..." >&2
            exit 1
        fi

        if [ "$password" = "$confirm_password" ]; then
            echo $(clean_string "$password")
            return 0
        else
            if (whiptail --title "Password Mismatch" --yesno "Passwords do not match. Do you want to try again?" 8 78); then
                continue
            else
                echo "User cancelled the operation." >&2
                exit 1
            fi
        fi
    done
}

# Function to check and set SSH key
check_and_set_ssh_key() {
    local default_key_path="/root/.ssh/id_rsa.pub"
    local backup_paths=("/root/.ssh/ci-ssh.pub" "/root/.ssh/authorized_keys")
    local key_path=""
    
    # Try to find an existing SSH key
    for path in "$default_key_path" "${backup_paths[@]}"; do
        if [ -f "$path" ]; then
            key_path="$path"
            break
        fi
    done
    
    # If no key found, ask for path
    if [ -z "$key_path" ]; then
        key_path=$(whiptail --inputbox "Enter the path to your SSH public key file:" 8 78 "$default_key_path" --title "SSH Key Configuration" 3>&1 1>&2 2>&3)
        if [ $? -ne 0 ]; then
            echo "SSH key configuration cancelled." >&2
            return 1
        fi
    fi

    if [ -f "$key_path" ]; then
        SSH_KEY_PATH="$key_path"
        return 0
    else
        if (whiptail --title "SSH Key Not Found" --yesno "SSH key file not found at $key_path. Do you want to proceed without an SSH key (not recommended)?" 8 78); then
            return 0
        else
            return 1
        fi
    fi
}

# Function to create a menu list for whiptail
create_menu_list() {
    local counter=1
    for item in "$@"; do
        echo "$counter $(clean_string "$item")"
        ((counter++))
    done
}

# Function to sanitize template name
sanitize_template_name() {
    local name="$1"
    # Replace spaces and colons with hyphens, remove other non-alphanumeric characters
    name=$(echo "$name" | tr ' :' '-' | tr -cd 'a-zA-Z0-9-')
    # Ensure the name starts with a letter and is lowercase
    name=$(echo "$name" | sed 's/^[^a-zA-Z]*//' | tr '[:upper:]' '[:lower:]')
    # Truncate to 63 characters (DNS label limit)
    echo "${name:0:63}"
}

# Function to check if required packages are installed
check_and_install_dependencies() {
    local dependencies=("libguestfs-tools" "qemu-utils" "wget")
    local missing_deps=()
    
    echo "Checking for required dependencies..."
    for dep in "${dependencies[@]}"; do
        if ! dpkg -l | grep -q "ii  $dep"; then
            missing_deps+=("$dep")
        fi
    done
    
    if [ ${#missing_deps[@]} -gt 0 ]; then
        echo "Installing missing dependencies: ${missing_deps[*]}"
        apt update -y
        apt install -y "${missing_deps[@]}"
    else
        echo "All required dependencies are already installed."
    fi
}

# OS versions and their image URLs
declare -A OS_VERSIONS=(
    ["Ubuntu 20.04"]="https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
    ["Ubuntu 22.04"]="https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"
    ["Ubuntu 24.04"]="https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
    ["Alma Linux 9"]="https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2"
    ["Amazon Linux 2"]="https://cdn.amazonlinux.com/os-images/2.0.20230727.0/kvm/amzn2-kvm-2.0.20230727.0-x86_64.xfs.gpt.qcow2"
    ["CentOS 9"]="https://cloud.centos.org/centos/9-stream/x86_64/images/CentOS-Stream-GenericCloud-9-latest.x86_64.qcow2"
    ["Fedora 38"]="https://download.fedoraproject.org/pub/fedora/linux/releases/38/Cloud/x86_64/images/Fedora-Cloud-Base-38-1.6.x86_64.qcow2"
    ["Oracle Linux 9"]="https://yum.oracle.com/templates/OracleLinux/OL9/u2/x86_64/OL9U2_x86_64-kvm-b197.qcow"
    ["Rocky Linux 9"]="https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
    ["Rocky Linux 8"]="https://dl.rockylinux.org/pub/rocky/8/images/x86_64/Rocky-8-GenericCloud-Base.latest.x86_64.qcow2"
)

# Function to get default cloud-init user based on OS
get_default_ci_user() {
    local os_name="$1"
    case "$os_name" in
        "Ubuntu"*) echo "ubuntu" ;;
        "Alma"*|"CentOS"*|"Rocky"*|"Oracle"*) echo "cloud-user" ;;
        "Fedora"*) echo "fedora" ;;
        "Amazon"*) echo "ec2-user" ;;
        *) echo "admin" ;;
    esac
}

# Function to prompt for OS selection using whiptail
select_os() {
    local options=()
    for os in "${!OS_VERSIONS[@]}"; do
        options+=("$os" "")
    done

    SELECTED_OS=$(whiptail --title "Select OS" --menu \
        "Choose the OS for the template:" 20 60 12 \
        "${options[@]}" \
        3>&1 1>&2 2>&3)
    
    if [ $? -ne 0 ] || [ -z "$SELECTED_OS" ]; then
        echo "No OS selected or user cancelled. Exiting." >&2
        exit 1
    fi
    
    SELECTED_OS=$(clean_string "$SELECTED_OS")
}

# Function to create template
create_template() {
    local OS_NAME=$1
    local TEMPLATE_NAME=$(sanitize_template_name "${OS_NAME}-cloudinit-template")
    
    # Use inherited values as default in prompts
    local storage_options=($(get_available_storage))
    if [ ${#storage_options[@]} -eq 0 ]; then
        whiptail --msgbox "No storage options available. Exiting." 8 60
        exit 1
    fi
    local storage_menu=$(create_menu_list "${storage_options[@]}")
    VM_DISK=$(whiptail --title "VM Disk Selection" --menu "Select VM disk for $TEMPLATE_NAME:" 15 60 5 $storage_menu 3>&1 1>&2 2>&3)
    if [ $? -ne 0 ]; then
        echo "User cancelled the operation." >&2
        exit 1
    fi
    VM_DISK=$(clean_string "${storage_options[$((VM_DISK-1))]}")

    local bridge_options=($(get_available_bridges))
    if [ ${#bridge_options[@]} -eq 0 ]; then
        whiptail --msgbox "No bridge options available. Exiting." 8 60
        exit 1
    fi
    local bridge_menu=$(create_menu_list "${bridge_options[@]}")
    VM_BRIDGE=$(whiptail --title "VM Bridge Selection" --menu "Select VM bridge for $TEMPLATE_NAME:" 15 60 5 $bridge_menu 3>&1 1>&2 2>&3)
    if [ $? -ne 0 ]; then
        echo "User cancelled the operation." >&2
        exit 1
    fi
    VM_BRIDGE=$(clean_string "${bridge_options[$((VM_BRIDGE-1))]}")
    
    # Prompt for VLAN
    if (whiptail --title "VLAN Configuration" --yesno "Do you want to use a VLAN for this template?" 8 78); then
        VLAN_ID=$(whiptail_prompt_required "VLAN ID" "Enter the VLAN ID (1-4094):" "${VLAN_ID:-}")
        # VLAN ID validation
        if ! [[ "$VLAN_ID" =~ ^[0-9]+$ ]] || [ "$VLAN_ID" -lt 1 ] || [ "$VLAN_ID" -gt 4094 ]; then
            whiptail --msgbox "Invalid VLAN ID. Please enter a number between 1 and 4094." 8 78
            exit 1
        fi
    else
        VLAN_ID=""
    fi

    # Prompt for template ID
    NEXT_ID=$(get_next_vmid)
    TEMPLATE_ID=$(whiptail_prompt_required "Template ID" "Enter the template ID for $TEMPLATE_NAME:" "$NEXT_ID")
    
    # Validate Template ID is numeric
    if ! [[ "$TEMPLATE_ID" =~ ^[0-9]+$ ]]; then
        whiptail --msgbox "Invalid Template ID. Please enter a numeric value." 8 78
        exit 1
    fi

    # Check if VM ID already exists
    if qm list | grep -qw "$TEMPLATE_ID"; then
        whiptail --msgbox "VM ID $TEMPLATE_ID already exists. Please choose a different ID." 8 78
        exit 1
    fi

    # Get default specs based on OS type
    local default_memory="2048"
    local default_cores="2"
    local default_disk="20"
    
    # For server OSes, maybe increase defaults
    if [[ "$OS_NAME" == *"Server"* ]]; then
        default_memory="4096"
        default_disk="40"
    fi

    # Prompt for VM specs with inherited values as defaults
    VM_MEMORY=$(whiptail_prompt_required "VM Memory" "Enter memory size in MB for $TEMPLATE_NAME:" "${VM_MEMORY:-$default_memory}")
    VM_CORES=$(whiptail_prompt_required "VM Cores" "Enter number of cores for $TEMPLATE_NAME:" "${VM_CORES:-$default_cores}")
    VM_DISK_SIZE=$(whiptail_prompt_required "VM Disk Size" "Enter disk size in GB for $TEMPLATE_NAME:" "${VM_DISK_SIZE:-$default_disk}")

    # Validate numeric values
    if ! [[ "$VM_MEMORY" =~ ^[0-9]+$ ]] || ! [[ "$VM_CORES" =~ ^[0-9]+$ ]] || ! [[ "$VM_DISK_SIZE" =~ ^[0-9]+$ ]]; then
        whiptail --msgbox "Invalid VM specifications. Please ensure memory, cores, and disk size are numeric values." 8 78
        exit 1
    fi

    # Get default cloud-init user for this OS
    local default_user=$(get_default_ci_user "$OS_NAME")
    
    # Prompt for cloud-init user
    CI_USER=$(whiptail_prompt_required "Cloud-Init User" "Enter the cloud-init user for $TEMPLATE_NAME:" "$default_user")

    # Prompt for cloud-init password (required) with confirmation
    CI_PASSWORD=$(whiptail_prompt_password "Cloud-Init Password" "Enter cloud-init password for $TEMPLATE_NAME:")

    # Check and set SSH key
    if check_and_set_ssh_key; then
        echo "SSH key configuration successful."
    else
        echo "SSH key configuration failed or was cancelled." >&2
        exit 1
    fi

    # Download and prepare the image
    local IMAGE_URL="${OS_VERSIONS[$OS_NAME]}"
    local IMAGE_FILENAME=$(basename "$IMAGE_URL")
    
    # Close all whiptail windows before starting long-running processes
    clear

    echo "Downloading $IMAGE_FILENAME..."
    if [ ! -f "$IMAGE_FILENAME" ]; then
        wget "$IMAGE_URL" -O "$IMAGE_FILENAME" || {
            echo "Failed to download $IMAGE_URL. Exiting."
            exit 1
        }
    else
        echo "$IMAGE_FILENAME already exists. Skipping download."
    fi

    # Handle special cases
    case "$OS_NAME" in
        "Oracle Linux 9")
            echo "Converting Oracle Linux image to qcow2 format..."
            qemu-img convert -O qcow2 -o compat=0.10 "$IMAGE_FILENAME" "${IMAGE_FILENAME%.qcow}.qcow2"
            IMAGE_FILENAME="${IMAGE_FILENAME%.qcow}.qcow2"
            ;;
    esac

    echo "Customizing $IMAGE_FILENAME..."
    # Create a backup of the original image
    cp "$IMAGE_FILENAME" "${IMAGE_FILENAME}.bak" || {
        echo "Failed to create backup of $IMAGE_FILENAME. Continuing without backup."
    }
    
    # Customize the image with virt-customize
    sudo virt-customize -a "$IMAGE_FILENAME" --install qemu-guest-agent,net-tools,bash-completion || {
        echo "virt-customize failed. Attempting to restore from backup."
        if [ -f "${IMAGE_FILENAME}.bak" ]; then
            mv "${IMAGE_FILENAME}.bak" "$IMAGE_FILENAME"
            echo "Restored from backup. Skipping customization step."
        else
            echo "No backup available. Continuing with original image."
        fi
    }
    
    # Try to enable qemu-guest-agent service
    sudo virt-customize -a "$IMAGE_FILENAME" --run-command 'systemctl enable qemu-guest-agent.service' || {
        echo "Failed to enable qemu-guest-agent service. The VM may not report its IP address correctly."
    }
    
    # Truncate machine-id to ensure unique machine ID on clone
    sudo virt-customize -a "$IMAGE_FILENAME" --truncate /etc/machine-id || {
        echo "Failed to truncate machine-id. New VMs may have network issues."
    }

    echo "Creating and configuring template $TEMPLATE_NAME..."
    # Resize the image to the specified disk size
    qemu-img resize "$IMAGE_FILENAME" "${VM_DISK_SIZE}G" || {
        echo "Failed to resize the image. Continuing with original size."
    }
    
    # Create the VM
    qm create "${TEMPLATE_ID}" --name "${TEMPLATE_NAME}" --memory "${VM_MEMORY}" --cores "${VM_CORES}" --cpu host --net0 "virtio,bridge=${VM_BRIDGE}${VLAN_ID:+,tag=$VLAN_ID}" --scsihw virtio-scsi-pci || {
        echo "Failed to create VM. Exiting."
        exit 1
    }
    
    # Import the disk
    qm set "${TEMPLATE_ID}" --scsi0 "${VM_DISK}:0,import-from=$(pwd)/${IMAGE_FILENAME}" || {
        echo "Failed to import disk image. Exiting."
        qm destroy "${TEMPLATE_ID}"
        exit 1
    }
    
    # Configure cloud-init
    qm set "${TEMPLATE_ID}" --ide2 "${VM_DISK}:cloudinit" || {
        echo "Failed to add cloud-init drive. VM may not be properly configured for cloud-init."
    }
    
    # Configure boot, serial, and agent
    qm set "${TEMPLATE_ID}" --boot order=scsi0
    qm set "${TEMPLATE_ID}" --serial0 socket --vga serial0
    qm set "${TEMPLATE_ID}" --agent enabled=1
    qm set "${TEMPLATE_ID}" --onboot 1
    
    # Configure networking
    qm set "${TEMPLATE_ID}" --ipconfig0 "ip6=auto,ip=dhcp"
    
    # Configure cloud-init user and password
    qm set "${TEMPLATE_ID}" --ciuser "${CI_USER}"
    qm set "${TEMPLATE_ID}" --cipassword "${CI_PASSWORD}"
    qm set "${TEMPLATE_ID}" --ciupgrade 1
    
    # Add SSH key if provided
    if [ -n "$SSH_KEY_PATH" ]; then
        qm set "${TEMPLATE_ID}" --sshkeys "$SSH_KEY_PATH" || {
            echo "Failed to add SSH key. You may need to add it manually."
        }
    fi
    
    # Convert to template
    qm template "${TEMPLATE_ID}" || {
        echo "Failed to convert VM to template. You may need to do this manually in the Proxmox UI."
    }

    whiptail --msgbox "TEMPLATE ${TEMPLATE_NAME} successfully created with ID ${TEMPLATE_ID}!" 8 78
    
    # Clean up temporary files
    if [ -f "${IMAGE_FILENAME}.bak" ]; then
        echo "Cleaning up backup file..."
        rm "${IMAGE_FILENAME}.bak"
    fi
}

# Main script execution
# Check if script is run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root" >&2
    exit 1
fi

# Check and install dependencies
check_and_install_dependencies

# Select OS
select_os

if (whiptail --title "Confirm Template Creation" --yesno "You have selected: $SELECTED_OS\n\nDo you want to proceed with creating this template?" 12 78); then
    # Create template for selected OS
    create_template "$SELECTED_OS"

    whiptail --msgbox "Template creation complete. You can now create clones of this template in the Proxmox web interface." 10 78
else
    echo "Operation cancelled by user." >&2
    exit 0
fi