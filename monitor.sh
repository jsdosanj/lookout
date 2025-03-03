#!/bin/bash

# Check for the OS type (Linux or macOS)
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="Linux"
    # Hostname and device info
    HOSTNAME=$(hostname)
    MANUFACTURER=$(cat /sys/class/dmi/id/chassis_vendor)
    HARDWARE_INFO=$(lscpu | grep "Model name")

    # OS info
    OS_VERSION=$(lsb_release -d | awk -F"\t" '{print $2}')

    # Users
    USERS=$(who | awk '{print $1}' | sort | uniq)

    # Installed Software (for Linux, using dpkg)
    SOFTWARE_INSTALLED=$(dpkg -l)

    # Check for updates
    UPDATES=$(sudo apt list --upgradable)

elif [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macOS"
    # Hostname and device info
    HOSTNAME=$(hostname)
    MANUFACTURER=$(system_profiler SPHardwareDataType | grep "Manufacturer")
    HARDWARE_INFO=$(system_profiler SPHardwareDataType | grep "Model Identifier")

    # OS info
    OS_VERSION=$(sw_vers -productVersion)

    # Users
    USERS=$(who | awk '{print $1}' | sort | uniq)

    # Installed Software (for macOS, using system profiler)
    SOFTWARE_INSTALLED=$(system_profiler SPApplicationsDataType)

    # Check for updates
    UPDATES=$(softwareupdate -l)
else
    echo "Unsupported OS."
    exit 1
fi

# Create the report
REPORT="/tmp/system_report.txt"
{
    echo "Users: $USERS"
    echo "Device Info:"
    echo "  Hostname: $HOSTNAME"
    echo "  Manufacturer: $MANUFACTURER"
    echo "  Hardware Details: $HARDWARE_INFO"
    echo "Current OS Version: $OS_VERSION"
    echo "Installed Software: $SOFTWARE_INSTALLED"
    echo "Pending OS/App Updates: $UPDATES"
} > "$REPORT"

# Mount the network drive (requires user credentials)
echo "Please enter your network drive credentials."
read -p "Username: " NET_USER
read -s -p "Password: " NET_PASS
echo ""

# For Linux
if [[ "$OS" == "Linux" ]]; then
    sudo mount -t cifs -o username=$NET_USER,password=$NET_PASS //network drive file path /mnt
    cp "$REPORT" /mnt/system_report.txt
    sudo umount /mnt
elif [[ "$OS" == "macOS" ]]; then
    mount_smbfs //$NET_USER@$NET_PASS@network drive file path /Volumes/Server
    cp "$REPORT" /Volumes/Server/system_report.txt
    umount /Volumes/Server
fi

echo "System report saved to the network drive."
