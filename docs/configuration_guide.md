# VNIM YAML Configuration Guide

This guide explains how to write configuration files for VNIM to build network topologies. VNIM topology files are written in YAML and consist of two root keys: `objects` and `services`.

---

## 🏗️ 1. Objects

The `objects` block defines network interfaces, namespaces, and addresses.

> [!TIP]
> **Static MAC Addresses**
> All interface types (`bridge`, `tap`, `tun`, `veth`, `vlan`, `vxlan`, `dummy`, `bond`) support an optional `mac` parameter (e.g. `mac: "52:54:00:12:34:56"`) to enforce a static hardware MAC address. This is extremely useful for matching DHCP static leases.

### Namespace
Creates a Linux network namespace (`ip netns`).
```yaml
- type: namespace
  name: lab-ns         # Name of the network namespace (max 15 chars)
```

### Bridge
Creates a virtual bridge interface (`ip link add ... type bridge`).
```yaml
- type: bridge
  name: br0            # Interface name (max 15 chars)
  namespace: lab-ns    # (Optional) Namespace to create the bridge in
```

### Address
Assigns an IPv4 CIDR address to an interface.
```yaml
- type: address
  interface: br0            # Target interface name
  address: 192.168.100.1/24 # CIDR formatted IP address
  namespace: lab-ns         # (Optional) Namespace of the interface
```

### TAP Interface
Creates a TAP interface (`mode tap`), commonly used for connecting QEMU/KVM virtual machines.
```yaml
- type: tap
  name: tap-vm1             # Interface name (max 15 chars)
  owner: ${USER}            # (Optional) Owner user. Defaults to current user
  master: br0               # (Optional) Bridge interface to attach to
  namespace: lab-ns         # (Optional) Namespace of the bridge/TAP interface
```

### TUN Interface
Creates a TUN interface (`mode tun`) for Layer 3 VPN configurations.
```yaml
- type: tun
  name: tun-vpn             # Interface name (max 15 chars)
  owner: ${USER}            # (Optional) Owner user. Defaults to current user
  namespace: lab-ns         # (Optional) Namespace of the TUN interface
```

### VETH Pair
Creates a virtual Ethernet pair to link namespaces together or to the host.
```yaml
- type: veth
  name: veth-ns             # Local interface name (max 15 chars)
  peer: veth-host           # Peer interface name (max 15 chars)
  master: br0               # (Optional) Bridge to attach the host/peer end to
  namespace: lab-ns         # (Optional) Namespace to move the local 'name' end into
```

### VLAN
Creates a Layer 2 Tagged VLAN sub-interface.
```yaml
- type: vlan
  name: eth0.100            # VLAN interface name (max 15 chars)
  parent: eth0              # Parent interface name
  vlan_id: 100              # VLAN tag (1 to 4094)
  namespace: lab-ns         # (Optional) Namespace to create the VLAN interface in
```

### VXLAN
Creates a Layer 3 Virtual Extensible LAN interface.
```yaml
- type: vxlan
  name: vxlan10             # VXLAN interface name (max 15 chars)
  parent: eth0              # Underlay parent interface
  vxlan_id: 10              # VNI tag (1 to 16777215)
  group: 239.1.1.1          # (Optional) Multicast group IP
  port: 4789                # (Optional) Destination UDP port (defaults to 4789)
  namespace: lab-ns         # (Optional) Namespace to create the VXLAN interface in
```

### Dummy Interface
Creates a dummy interface, useful for loopbacks or routing targets.
```yaml
- type: dummy
  name: dum0                # Interface name (max 15 chars)
  namespace: lab-ns         # (Optional) Namespace of the dummy interface
```

### Bond
Creates a link aggregation interface (Bonding).
```yaml
- type: bond
  name: bond0               # Bond interface name (max 15 chars)
  interfaces:               # List of member interfaces
    - eth1
    - eth2
  mode: active-backup       # Bonding mode (e.g. balance-rr, active-backup, 802.3ad)
  namespace: lab-ns         # (Optional) Namespace to create the bond in
```

### Route
Adds a static route inside a namespace or on the host.
```yaml
- type: route
  destination: 0.0.0.0/0    # Target IP or CIDR subnet (defaults to 0.0.0.0/0)
  gateway: 10.200.0.1       # Next-hop gateway IP (Required)
  interface: veth-wan       # (Optional) Outbound device interface
  namespace: secure-lan     # (Optional) Namespace context for the route rule
```

### NAT
Sets up a NAT Masquerade rule on the host side and enables system IP forwarding.
```yaml
- type: nat
  interface: veth-wan-host  # Outbound device interface (Required)
  source_subnet: 10.200.0.0/24 # CIDR source subnet to masquerade (Required)
```


---

## ⚙️ 2. Services

The `services` block controls temporary network infrastructure daemons.

### DHCP
Spawns a DHCP daemon (`dnsmasq`) inside the namespace of the interface.
```yaml
- type: dhcp
  interface: br0                 # Interface to bind to
  subnet: 192.168.100.0/24       # (Optional) Subnet CIDR. Auto-calculated if omitted
  range_start: 192.168.100.50    # (Optional) Range start IP. Auto-calculated if omitted
  range_end: 192.168.100.150     # (Optional) Range end IP. Auto-calculated if omitted
  router: 192.168.100.1          # (Optional) Gateway IP. Defaults to interface IP
  dns: 1.1.1.1                   # (Optional) Upstream DNS option. Defaults to Cloudflare
  static_leases:                 # (Optional) Static MAC-to-IP reservations
    - mac: 52:54:00:12:34:56
      ip: 192.168.100.21
    - mac: 52:54:00:ab:cd:ef
      ip: 192.168.100.22
```

### DNS
Spawns a DNS resolver with custom hosts mappings.
```yaml
- type: dns
  interface: br0                 # Interface to bind to
  port: 53                       # (Optional) DNS port (defaults to 53)
  hosts:                         # List of local host mappings
    - name: gateway.lab
      ip: 192.168.100.1
    - name: server.lab
      ip: 192.168.100.10
```

### HTTP Server
Spawns a lightweight, built-in Go static file server inside the namespace.
```yaml
- type: http
  interface: br0                 # Interface to bind to
  port: 8080                     # (Optional) Port to listen on (defaults to 8080)
  root: /var/www/html            # (Optional) Directory to serve (defaults to current dir)
```

### TFTP
Spawns a TFTP server.
```yaml
- type: tftp
  interface: br0                 # Interface to bind to
  root: /var/lib/tftpboot        # (Optional) Root TFTP directory (defaults to /var/lib/tftpboot)
```

### PXE
Spawns a combined DHCP + TFTP server configured for Network PXE Booting.
```yaml
- type: pxe
  interface: br0                 # Interface to bind to
  subnet: 192.168.100.0/24
  range_start: 192.168.100.50
  range_end: 192.168.100.150
  dhcp_bootfile: pxelinux.0       # PXE bootloader filename (Required)
  dhcp_next_server: 192.168.100.1 # (Optional) Next TFTP server IP (defaults to interface IP)
  root: /var/lib/tftpboot        # (Optional) Directory containing boot files
  static_leases:                 # (Optional) Static MAC-to-IP reservations
    - mac: 52:54:00:12:34:56
      ip: 192.168.100.21
```
