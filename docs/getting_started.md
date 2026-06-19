# VNIM Getting Started Guide

VNIM (Virtual Network Interface Manager) makes it easy to setup local network labs, container switches, virtual machine endpoints, or PXE booting environments in isolated network namespaces.

---

## 📋 Prerequisites

To run VNIM, ensure your Linux system has:
1. **Go Compiler**: Go version 1.22+ (to compile from source).
2. **Network Utilities**: The `ip` tool (part of `iproute2`) is required.
3. **Services Tool**: `dnsmasq` is required to run DHCP, DNS, TFTP, or PXE service configurations.

You can install these packages on Debian/Ubuntu with:
```bash
sudo apt update
sudo apt install -y build-essential iproute2 dnsmasq go-dep
```

---

## 🛠️ Build and Install

Clone the repository and compile using the provided Makefile:

```bash
# 1. Fetch dependencies
make fetch

# 2. Build the binary
make build

# 3. Run tests to ensure validation is correct
make test

# 4. Install the binary globally (copies to /usr/local/bin/vnim)
make install
```

---

## 🚀 Quickstart Walkthrough

Here is how to deploy and tear down your first virtual lab topology.

### 1. View the Lab Tree
Let's see what is inside the demo configuration:
```bash
vnim tree demo.yaml
```
Output:
```text
├── demo-ns (namespace)
│   ├── br-demo (bridge) [svc: dhcp, svc: http:8080, ip: 192.168.50.1/24]
│   │   └── tap-vm (tap)
│   ├── veth-ns (veth) [peer: veth-host]
│   └── dummy0 (dummy) [ip: 172.16.10.1/24]
└── veth.100 (vlan) [id: 100, parent: veth-host]
```

### 2. Dry-Run (Preview execution)
Review the exact shell command script that will be applied as root:
```bash
vnim dry-run demo.yaml
```

### 3. Deploy the Lab
Build the network interfaces and launch services:
```bash
vnim up demo.yaml
```
*Note: If run as a normal user, VNIM will automatically prompt for privileges using `sudo -E` internally.*

### 4. Inspect Active Labs
Check status:
```bash
vnim ps
```
Output:
```text
PLAN            YAML PATH                            CREATED AT                SERVICES
demo            /home/user/vnim/demo.yaml            2026-06-19 17:05:00       dhcp, http
```

### 5. Execute Commands in Isolated Nodes
To execute utilities (e.g. `ping`, `ip addr`, or testing tools) directly inside a namespace context without manually logging in:
```bash
# Ping Node 2 from inside Node 1 namespace
vnim exec node-1 ping 10.10.10.12
```

### 6. Drop into Interactive Namespace Shells
Drop directly into an interactive bash (or default shell) environment isolated inside a namespace:
```bash
vnim shell node-1
```

### 7. Tear Down the Lab
Stop all background processes (DHCP & Web servers), clean state logs, and delete all created interfaces/namespaces:
```bash
vnim down demo
```
*Note: You can tear down a plan by passing either the filename (`demo.yaml`) or the plan name (`demo`).*

