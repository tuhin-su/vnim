# VNIM Architectural Internals

This document details the engineering and architectural decisions in VNIM's implementation.

---

## 🛠️ Modular Subsystem Architecture

VNIM is designed around independent components communicating through strict Go interfaces:

```text
       ┌───────────┐
       │    CLI    │ (cmd/vnim/main.go)
       └─────┬─────┘
             │
      ┌──────┴──────┐
      ▼             ▼
┌───────────┐ ┌───────────┐
│  Config   │ │   State   │ (pkg/config & pkg/state)
└─────┬─────┘ └─────┬─────┘
      │             │
      ├─────────────┤
      ▼             ▼
┌───────────┐ ┌───────────┐
│  Network  │ │ Services  │ (pkg/network & pkg/services)
└───────────┘ └───────────┘
```

1. **CLI Layer (`cmd/vnim`)**: Orchestrates operations and routes requests to subsystems.
2. **Config (`pkg/config`)**: Parses YAML input, resolves environment variables, and executes safety validations.
3. **State (`pkg/state`)**: Persists active state details to `/var/run/vnim/plans/<plan_name>.json`.
4. **Network Driver (`pkg/network`)**: Interacts with the kernel. Defined as a Go interface:
   ```go
   type NetworkManager interface {
       CreateNamespace(name string) error
       CreateBridge(name string, ns string) error
       CreateTap(name, owner, master string, ns string) error
       // ...
   }
   ```
   This interface enables both the live runner (`CmdNetworkManager`) and the command logging simulator (`DryRunNetworkManager`).
5. **Services Controller (`pkg/services`)**: Configures and hosts DHCP/DNS/TFTP/PXE/HTTP.

---

## 🔒 Privilege Model & Escalation Helper

Rather than requiring the user to run the entire tool as `sudo`, VNIM uses a root helper mechanism:

1. **User Space Validation**: Normal users run the command. Parsing, validation (`vnim validate`), tree rendering (`vnim tree`), and state lookups occur in user-space.
2. **Escalation**: If network modifications are requested (`vnim up` or `vnim down`), VNIM executes itself under `sudo -E`.
   - **`-E` (Environment Preservation)** is critical. It preserves the `$USER` variable so TAP/TUN interface ownership is properly mapped to the normal user who launched the deployment.
3. **Internal Helper Subcommands**: The escalated command executes internal commands:
   - `vnim helper-up <yaml>`
   - `vnim helper-down <yaml>`
   These subcommands are hidden from CLI help menus and are only invoked by the supervisor internally.

---

## 💾 State Persistence Schema

Active plan tracking is saved under `/var/run/vnim/plans/<plan_name>.json` in a structured JSON schema:

```json
{
  "name": "demo",
  "yaml_path": "/home/user/vnim/demo.yaml",
  "created_at": "2026-06-19T17:05:00Z",
  "objects": [
    {
      "type": "namespace",
      "name": "demo-ns"
    },
    {
      "type": "bridge",
      "name": "br-demo",
      "namespace": "demo-ns"
    }
  ],
  "services": [
    {
      "type": "dhcp",
      "interface": "br-demo",
      "pid": 284192
    }
  ]
}
```

This persistent tracking ensures that:
- Topologies can be cleanly deleted (`vnim down`) using just the plan name, even if the source YAML configuration file has been modified or deleted.
- Service processes (using registered `pid` fields) are fully tracked and stopped.

---

## 🔄 Transactional Deploys & Rollback

To prevent half-configured, broken network layers on the host:
- VNIM executes creation tasks in topological order (namespaces $\rightarrow$ bridges $\rightarrow$ interfaces $\rightarrow$ addresses $\rightarrow$ services).
- If any creation command fails, VNIM enters a **rollback routine**. It iterates backwards through all objects declared in the configuration, deleting interfaces and namespaces to leave the host in its original state.

---

## 📡 Service Daemons & Signal 0 Cleanup

- **Dnsmasq Integration**: DHCP, DNS, TFTP, and PXE services spawn background `dnsmasq` instances. Dynamic options (like IP ranges or subnet netmasks) are auto-calculated from interface IPs.
- **Go HTTP Server**: Spawns a built-in static HTTP server inside namespaces.
- **Signal 0 Polling**: During teardown, background service processes are terminated using `SIGTERM`. Because these PIDs might be re-parented to `init` (meaning they aren't direct children of the active `vnim` process), VNIM polls the process state using Unix **Signal 0** (`proc.Signal(syscall.Signal(0))`). If the process remains active after 2 seconds, VNIM issues a `SIGKILL` to force-quit the daemon.
