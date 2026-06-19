# 🌐 VNIM (Virtual Network Interface Manager)

[![Go Version](https://img.shields.io/github/go-mod/go-version/tuhin-su/vnim?color=00ADD8&logo=go&logoColor=white)](https://golang.org)
[![Platform](https://img.shields.io/badge/platform-Linux-E34F26?logo=linux&logoColor=white)](#)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](file:///home/master/Desktop/vnim/LICENSE)
[![Status](https://img.shields.io/badge/build-passing-brightgreen.svg)](#)

VNIM is a professional Linux network orchestration tool for defining, spinning up, and managing complex virtual network topologies inside isolated network namespaces via simple YAML configs.

---

## 📖 Table of Contents
* [🚀 Getting Started](#-getting-started)
* [⚡ Quickstart Build](#-quickstart-build)
* [🧩 Features](#-features)
* [🔒 Permission & Ownership Model](#-permission--ownership-model)
* [🌐 Plan Lifecycle commands](#-plan-lifecycle-commands)
* [📚 Full Documentation Guides](#-full-documentation-guides)
* [🤝 Contributing & Open Source](#-contributing--open-source)

---

## 🚀 Getting Started

VNIM allows you to describe bridges, namespaces, TAP/TUN adapters, VETH pairs, VLANs, Bonds, and VXLAN interfaces along with dynamic infrastructure services (DHCP, DNS, TFTP, PXE, and HTTP servers) in a single YAML plan.

### Visualise Topologies Instantly
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

---

## ⚡ Quickstart Build

Get VNIM compiled and installed on your Linux machine:

```bash
# 1. Fetch module dependencies
make fetch

# 2. Compile the local binary
make build

# 3. Run validation unit tests
make test

# 4. Install the binary to /usr/local/bin
sudo make install
```

---

## 🧩 Features

- **🌐 Isolated Environments**: Create complex multi-node structures inside virtual namespaces (`ip netns`).
- **🎛️ Dynamic Service Provisioning**: Auto-inject temporary DHCP, DNS caching, HTTP servers, TFTP, and PXE environments.
- **🔄 Transactional Rollbacks**: If any creation step fails during setup, VNIM automatically tears down all intermediate interfaces to leave your host clean.
- **🔍 Pure-Dry Run Simulation**: Dry-run previews generate the actual `ip link` shell commands without root permissions.
- **⏱️ Signal 0 Process Auditing**: Background services are cleanly terminated using SIGTERM/SIGKILL polling verified via Unix Signal 0.

---

## 🔒 Permission & Ownership Model

> [!IMPORTANT]
> **No Manual Sudo Chains**
> VNIM runs privileged operations internally. Normal users can validate plans, view trees, and trigger deployments. If root actions are required, VNIM escalates privileges using `sudo -E` internally.

> [!NOTE]
> **Ownership Delegation**
> When provisioning user-facing interfaces (like TAP/TUN adapters), VNIM preserves the caller environment to delegate interface ownership to the non-root execution user (`${USER}`).

```yaml
objects:
  - type: tap
    name: vm-net
    owner: ${USER} # Automatically resolves to the user who ran the command
```

---

## 🌐 Plan Lifecycle Commands

Provision a topology:
```bash
vnim up lab.yaml
```

Teardown a topology and stop all daemons:
```bash
vnim down lab.yaml
# Or via plan name:
vnim down lab
```

Preview execution commands:
```bash
vnim dry-run lab.yaml
```

List active deployments and their services:
```bash
vnim ps
```

---

## 📚 Full Documentation Guides

For more details on writing plans, CLI switches, and design choices:

1. **[docs/getting_started.md](file:///home/master/Desktop/vnim/docs/getting_started.md)**: Full installation, dependency checks, and command walk-throughs.
2. **[docs/configuration_guide.md](file:///home/master/Desktop/vnim/docs/configuration_guide.md)**: Complete YAML schemas and properties for all interfaces and service configurations.
3. **[docs/architecture.md](file:///home/master/Desktop/vnim/docs/architecture.md)**: Subsystem layout, state JSON structures, and rollback workflows.
4. **[examples/](file:///home/master/Desktop/vnim/examples/)**: Check out production-ready network plans for bridges, node bridges, Bonds/VLANs, and VXLAN overlays.

---

## 🤝 Contributing & Open Source

We welcome and appreciate all forms of contributions! Whether you're reporting a bug, proposing a new feature, or submitting a pull request, you are making VNIM better for everyone.

To get started, please refer to:
* 📄 **[CONTRIBUTING.md](file:///home/master/Desktop/vnim/CONTRIBUTING.md)**: Our developer guide for building, testing, and formatting.
* ⚖️ **[LICENSE](file:///home/master/Desktop/vnim/LICENSE)**: Distributed under the permissive MIT License.
* 🕊️ **[CODE_OF_CONDUCT.md](file:///home/master/Desktop/vnim/CODE_OF_CONDUCT.md)**: Standards of behavior within our community.
* 🛡️ **[SECURITY.md](file:///home/master/Desktop/vnim/SECURITY.md)**: Responsible disclosure guidelines for security vulnerabilities.
