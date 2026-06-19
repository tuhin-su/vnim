# Contributing to VNIM 🌐

Thank you for your interest in contributing to **VNIM (Virtual Network Interface Manager)**! We welcome contributions from developers, network engineers, writers, and designers of all skill levels. 

This document outlines the guidelines and procedures to make contributing as smooth and rewarding as possible.

---

## 📖 Table of Contents
- [Code of Conduct](#-code-of-conduct)
- [How Can I Contribute?](#-how-can-i-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Features](#suggesting-features)
  - [Submitting Pull Requests](#submitting-pull-requests)
- [Development Setup](#-development-setup)
  - [Prerequisites](#prerequisites)
  - [Step-by-Step Setup](#step-by-step-setup)
  - [Building and Running](#building-and-running)
  - [Running Tests](#running-tests)
- [Codebase Walkthrough](#-codebase-walkthrough)
- [Style & Code Guidelines](#-style--code-guidelines)
  - [Go Guidelines](#go-guidelines)
  - [Git Commit Messages](#git-commit-messages)

---

## 🤝 Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md). Please report any unacceptable behavior to the project maintainers.

---

## 🛠️ How Can I Contribute?

### Reporting Bugs
If you find a bug, please check the [GitHub Issue Tracker](https://github.com/tuhin-su/vnim/issues) to ensure it hasn't already been reported. If not, open a new issue and include:
* A clear, descriptive title.
* The version of VNIM and Go you are using.
* Your Linux kernel/OS version.
* Detailed steps to reproduce the issue, including your YAML configuration if applicable.
* The expected vs. actual behavior.
* Relevant terminal outputs or log files.

### Suggesting Features
We love hearing new ideas! To suggest a feature, open an issue and include:
* A descriptive title.
* The problem you are trying to solve and how this feature addresses it.
* A detailed explanation of how it should work, including CLI flags or YAML properties.
* Mockups or ASCII art if applicable.

### Submitting Pull Requests
1. **Fork the repository** on GitHub.
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/your-username/vnim.git
   cd vnim
   ```
3. **Create a feature branch** using a descriptive name:
   ```bash
   git checkout -b feature/cool-new-interface
   # Or for bug fixes:
   git checkout -b fix/dhcp-lease-issue
   ```
4. **Implement your changes**, add unit tests for any new logic, and make sure existing tests pass.
5. **Verify that the project builds** and tests are green:
   ```bash
   make test
   make build
   ```
6. **Commit your changes** following the [Git Commit Messages](#git-commit-messages) guidelines.
7. **Push to your fork** and **submit a Pull Request (PR)** to the `main` branch of `tuhin-su/vnim`.
8. Ensure the PR description details *what* changes were made, *why* they were made, and *how* to test them.

---

## 💻 Development Setup

### Prerequisites
To develop and build VNIM, you will need:
* **Go 1.26** or higher.
* **Linux Environment** (VNIM relies on Linux-specific namespaces, VETH pairs, and socket connections).
* **make** and **gcc** for building.

### Step-by-Step Setup
1. Fetch dependencies:
   ```bash
   make fetch
   ```

2. Compile a local debug binary:
   ```bash
   make build
   ```
   The compiled binary will be placed inside `build/vnim`.

3. Run the unit test suite:
   ```bash
   make test
   ```

### Developing Privileged Code
Since VNIM manages low-level network interfaces and namespaces, many execution paths require root capabilities (`sudo`).
You can dry-run plans without root to preview commands:
```bash
./build/vnim dry-run examples/bridge.yaml
```

To run VNIM under actual test configurations, run it with `sudo`:
```bash
sudo ./build/vnim up examples/bridge.yaml
```

---

## 🗂️ Codebase Walkthrough

* **[cmd/vnim/main.go](file:///home/master/Desktop/vnim/cmd/vnim/main.go)**: Contains the CLI entry point, argument parsing, and command routing.
* **[pkg/config/](file:///home/master/Desktop/vnim/pkg/config/)**: Handles parsing of YAML plan configuration files, environment variable interpolation (`${USER}` etc.), and structural validation.
* **[pkg/network/](file:///home/master/Desktop/vnim/pkg/network/)**: Core orchestration logic for setting up and tearing down network interfaces (Bridges, TAP/TUN, VETH, VLANs, Bonds, and VXLAN overlays).
* **[pkg/services/](file:///home/master/Desktop/vnim/pkg/services/)**: Provisioning logic for network services, including isolated DHCP, DNS cache, HTTP servers, TFTP, and PXE booting.
* **[pkg/state/](file:///home/master/Desktop/vnim/pkg/state/)**: Handles transaction persistence, tracking active plan deployments, and triggering atomic rollbacks on failure.
* **[pkg/tree/](file:///home/master/Desktop/vnim/pkg/tree/)**: Generates the ASCII-art tree representations of defined network topology plans.

---

## 🎨 Style & Code Guidelines

### Go Guidelines
* **Standard Formatting**: Always run `go fmt ./...` before committing.
* **Linting**: Ensure code is clean and free of warnings.
* **Errors**: Return errors explicitly. Avoid silent failures or panics unless it's a critical startup configuration error.
* **Documentation**: Document all public functions, structures, and package-level constructs. Keep code clean and self-explanatory.
* **Tests**: Write unit tests for all new helper functions, parsers, and controllers.

### Git Commit Messages
We follow a structured commit style similar to [Conventional Commits](https://www.conventionalcommits.org/):
```text
<type>(<scope>): <short summary>

[optional body description]
```
**Types**:
* `feat`: A new feature (e.g., support for WireGuard tunnels)
* `fix`: A bug fix
* `docs`: Documentation changes only
* `style`: Code formatting changes (formatting, missing semicolons, etc.)
* `refactor`: Code changes that neither fix a bug nor add a feature
* `test`: Adding missing tests or correcting existing tests
* `chore`: Updating build tasks, package manager configs, etc.

**Example**:
```text
feat(network): add support for wireguard tunnel interfaces

- Parse wg-tunnel configurations from YAML schemas
- Implement link state creation using wg-quick/ip link commands
- Handle cleanup rules on rollback
```
