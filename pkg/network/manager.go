package network

import (
	"fmt"
	"os/exec"
	"strings"
)

type NetworkManager interface {
	CreateNamespace(name string) error
	CreateBridge(name string, ns string) error
	CreateTap(name, owner, master string, ns string) error
	CreateTun(name, owner string, ns string) error
	CreateVeth(name, peer, master string, ns string) error
	CreateVlan(name, parent string, vlanID int, ns string) error
	CreateVxlan(name, parent string, vxlanID int, group string, port int, ns string) error
	CreateDummy(name string, ns string) error
	CreateBond(name string, interfaces []string, mode string, ns string) error
	AddAddress(interfaceName, address string, ns string) error
	AddRoute(destination, gateway, interfaceName, ns string) error
	DeleteRoute(destination, gateway, interfaceName, ns string) error
	AddNat(interfaceName, sourceSubnet string) error
	DeleteNat(interfaceName, sourceSubnet string) error
	SetMacAddress(interfaceName, mac string, ns string) error

	DeleteInterface(name string, ns string) error
	DeleteNamespace(name string) error

	GetCommands() []string
}

// CmdNetworkManager executes commands live on the host
type CmdNetworkManager struct {
	commands []string
}

func NewCmdNetworkManager() *CmdNetworkManager {
	return &CmdNetworkManager{
		commands: make([]string, 0),
	}
}

func (m *CmdNetworkManager) run(ns string, cmd string, args ...string) error {
	var fullCmd []string
	if ns != "" {
		fullCmd = append([]string{"netns", "exec", ns, cmd}, args...)
		cmd = "ip"
		args = fullCmd
	}

	commandStr := cmd + " " + strings.Join(args, " ")
	m.commands = append(m.commands, commandStr)

	c := exec.Command(cmd, args...)
	output, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run command %q: %w (output: %q)", commandStr, err, string(output))
	}
	return nil
}

func (m *CmdNetworkManager) GetCommands() []string {
	return m.commands
}

func (m *CmdNetworkManager) CreateNamespace(name string) error {
	cmdStr := fmt.Sprintf("ip netns add %s", name)
	m.commands = append(m.commands, cmdStr)
	c := exec.Command("ip", "netns", "add", name)
	if output, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create namespace %q: %w (output: %q)", name, err, string(output))
	}
	return nil
}

func (m *CmdNetworkManager) DeleteNamespace(name string) error {
	cmdStr := fmt.Sprintf("ip netns del %s", name)
	m.commands = append(m.commands, cmdStr)
	c := exec.Command("ip", "netns", "del", name)
	if output, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete namespace %q: %w (output: %q)", name, err, string(output))
	}
	return nil
}

func (m *CmdNetworkManager) CreateBridge(name string, ns string) error {
	if err := m.run(ns, "ip", "link", "add", name, "type", "bridge"); err != nil {
		return err
	}
	return m.run(ns, "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) CreateTap(name, owner, master string, ns string) error {
	args := []string{"tuntap", "add", "dev", name, "mode", "tap"}
	if owner != "" {
		args = append(args, "user", owner)
	}
	if err := m.run(ns, "ip", args...); err != nil {
		return err
	}
	if master != "" {
		if err := m.run(ns, "ip", "link", "set", name, "master", master); err != nil {
			return err
		}
	}
	return m.run(ns, "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) CreateTun(name, owner string, ns string) error {
	args := []string{"tuntap", "add", "dev", name, "mode", "tun"}
	if owner != "" {
		args = append(args, "user", owner)
	}
	if err := m.run(ns, "ip", args...); err != nil {
		return err
	}
	return m.run(ns, "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) CreateVeth(name, peer, master string, ns string) error {
	// Create veth pair in default namespace
	if err := m.run("", "ip", "link", "add", name, "type", "veth", "peer", "name", peer); err != nil {
		return err
	}

	if ns != "" {
		// Move name to namespace
		if err := m.run("", "ip", "link", "set", name, "netns", ns); err != nil {
			return err
		}
		// Bring up peer in default namespace
		if err := m.run("", "ip", "link", "set", peer, "up"); err != nil {
			return err
		}
		// Bring up name in namespace
		if err := m.run(ns, "ip", "link", "set", name, "up"); err != nil {
			return err
		}
		// If master is set, attach peer to master in default namespace
		if master != "" {
			if err := m.run("", "ip", "link", "set", peer, "master", master); err != nil {
				return err
			}
		}
	} else {
		// No namespace, bring up both in default namespace
		if err := m.run("", "ip", "link", "set", name, "up"); err != nil {
			return err
		}
		if err := m.run("", "ip", "link", "set", peer, "up"); err != nil {
			return err
		}
		if master != "" {
			if err := m.run("", "ip", "link", "set", name, "master", master); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *CmdNetworkManager) CreateVlan(name, parent string, vlanID int, ns string) error {
	if ns != "" {
		// Create in default namespace and move to namespace
		if err := m.run("", "ip", "link", "add", "link", parent, "name", name, "type", "vlan", "id", fmt.Sprintf("%d", vlanID)); err != nil {
			return err
		}
		if err := m.run("", "ip", "link", "set", name, "netns", ns); err != nil {
			return err
		}
		return m.run(ns, "ip", "link", "set", name, "up")
	}

	if err := m.run("", "ip", "link", "add", "link", parent, "name", name, "type", "vlan", "id", fmt.Sprintf("%d", vlanID)); err != nil {
		return err
	}
	return m.run("", "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) CreateVxlan(name, parent string, vxlanID int, group string, port int, ns string) error {
	args := []string{"link", "add", name, "type", "vxlan", "id", fmt.Sprintf("%d", vxlanID), "dev", parent}
	if group != "" {
		args = append(args, "group", group)
	}
	if port > 0 {
		args = append(args, "dstport", fmt.Sprintf("%d", port))
	} else {
		args = append(args, "dstport", "4789") // Standard VXLAN port
	}

	if ns != "" {
		if err := m.run("", "ip", args...); err != nil {
			return err
		}
		if err := m.run("", "ip", "link", "set", name, "netns", ns); err != nil {
			return err
		}
		return m.run(ns, "ip", "link", "set", name, "up")
	}

	if err := m.run("", "ip", args...); err != nil {
		return err
	}
	return m.run("", "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) CreateDummy(name string, ns string) error {
	if err := m.run(ns, "ip", "link", "add", name, "type", "dummy"); err != nil {
		return err
	}
	return m.run(ns, "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) CreateBond(name string, interfaces []string, mode string, ns string) error {
	args := []string{"link", "add", name, "type", "bond"}
	if mode != "" {
		args = append(args, "mode", mode)
	}

	if err := m.run(ns, "ip", args...); err != nil {
		return err
	}

	for _, member := range interfaces {
		if err := m.run(ns, "ip", "link", "set", member, "master", name); err != nil {
			return err
		}
	}

	return m.run(ns, "ip", "link", "set", name, "up")
}

func (m *CmdNetworkManager) AddAddress(interfaceName, address string, ns string) error {
	return m.run(ns, "ip", "addr", "add", address, "dev", interfaceName)
}

func (m *CmdNetworkManager) DeleteInterface(name string, ns string) error {
	return m.run(ns, "ip", "link", "del", name)
}

func (m *CmdNetworkManager) AddRoute(destination, gateway, interfaceName, ns string) error {
	args := []string{"route", "add"}
	if destination == "" {
		args = append(args, "default")
	} else {
		args = append(args, destination)
	}
	args = append(args, "via", gateway)
	if interfaceName != "" {
		args = append(args, "dev", interfaceName)
	}
	return m.run(ns, "ip", args...)
}

func (m *CmdNetworkManager) DeleteRoute(destination, gateway, interfaceName, ns string) error {
	args := []string{"route", "del"}
	if destination == "" {
		args = append(args, "default")
	} else {
		args = append(args, destination)
	}
	args = append(args, "via", gateway)
	if interfaceName != "" {
		args = append(args, "dev", interfaceName)
	}
	return m.run(ns, "ip", args...)
}

func (m *CmdNetworkManager) AddNat(interfaceName, sourceSubnet string) error {
	// Enable IP forwarding
	cmdStr1 := "sysctl -w net.ipv4.ip_forward=1"
	m.commands = append(m.commands, cmdStr1)
	c1 := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	if output, err := c1.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w (output: %q)", err, string(output))
	}

	// Add iptables MASQUERADE rule
	cmdStr2 := fmt.Sprintf("iptables -t nat -A POSTROUTING -o %s -s %s -j MASQUERADE", interfaceName, sourceSubnet)
	m.commands = append(m.commands, cmdStr2)
	c2 := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", interfaceName, "-s", sourceSubnet, "-j", "MASQUERADE")
	if output, err := c2.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add NAT MASQUERADE rule: %w (output: %q)", err, string(output))
	}
	return nil
}

func (m *CmdNetworkManager) DeleteNat(interfaceName, sourceSubnet string) error {
	cmdStr := fmt.Sprintf("iptables -t nat -D POSTROUTING -o %s -s %s -j MASQUERADE", interfaceName, sourceSubnet)
	m.commands = append(m.commands, cmdStr)
	c := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-o", interfaceName, "-s", sourceSubnet, "-j", "MASQUERADE")
	if output, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete NAT MASQUERADE rule: %w (output: %q)", err, string(output))
	}
	return nil
}

func (m *CmdNetworkManager) SetMacAddress(interfaceName, mac string, ns string) error {
	return m.run(ns, "ip", "link", "set", "dev", interfaceName, "address", mac)
}

// DryRunNetworkManager simulates actions and collects the commands
type DryRunNetworkManager struct {
	commands []string
}

func NewDryRunNetworkManager() *DryRunNetworkManager {
	return &DryRunNetworkManager{
		commands: make([]string, 0),
	}
}

func (m *DryRunNetworkManager) formatCmd(ns string, cmd string, args ...string) string {
	if ns != "" {
		return fmt.Sprintf("ip netns exec %s %s %s", ns, cmd, strings.Join(args, " "))
	}
	return fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
}

func (m *DryRunNetworkManager) GetCommands() []string {
	return m.commands
}

func (m *DryRunNetworkManager) CreateNamespace(name string) error {
	m.commands = append(m.commands, fmt.Sprintf("ip netns add %s", name))
	return nil
}

func (m *DryRunNetworkManager) DeleteNamespace(name string) error {
	m.commands = append(m.commands, fmt.Sprintf("ip netns del %s", name))
	return nil
}

func (m *DryRunNetworkManager) CreateBridge(name string, ns string) error {
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "add", name, "type", "bridge"))
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", name, "up"))
	return nil
}

func (m *DryRunNetworkManager) CreateTap(name, owner, master string, ns string) error {
	args := []string{"tuntap", "add", "dev", name, "mode", "tap"}
	if owner != "" {
		args = append(args, "user", owner)
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", args...))
	if master != "" {
		m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", name, "master", master))
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", name, "up"))
	return nil
}

func (m *DryRunNetworkManager) CreateTun(name, owner string, ns string) error {
	args := []string{"tuntap", "add", "dev", name, "mode", "tun"}
	if owner != "" {
		args = append(args, "user", owner)
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", args...))
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", name, "up"))
	return nil
}

func (m *DryRunNetworkManager) CreateVeth(name, peer, master string, ns string) error {
	m.commands = append(m.commands, fmt.Sprintf("ip link add %s type veth peer name %s", name, peer))
	if ns != "" {
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s netns %s", name, ns))
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s up", peer))
		m.commands = append(m.commands, fmt.Sprintf("ip netns exec %s ip link set %s up", ns, name))
		if master != "" {
			m.commands = append(m.commands, fmt.Sprintf("ip link set %s master %s", peer, master))
		}
	} else {
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s up", name))
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s up", peer))
		if master != "" {
			m.commands = append(m.commands, fmt.Sprintf("ip link set %s master %s", name, master))
		}
	}
	return nil
}

func (m *DryRunNetworkManager) CreateVlan(name, parent string, vlanID int, ns string) error {
	if ns != "" {
		m.commands = append(m.commands, fmt.Sprintf("ip link add link %s name %s type vlan id %d", parent, name, vlanID))
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s netns %s", name, ns))
		m.commands = append(m.commands, fmt.Sprintf("ip netns exec %s ip link set %s up", ns, name))
	} else {
		m.commands = append(m.commands, fmt.Sprintf("ip link add link %s name %s type vlan id %d", parent, name, vlanID))
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s up", name))
	}
	return nil
}

func (m *DryRunNetworkManager) CreateVxlan(name, parent string, vxlanID int, group string, port int, ns string) error {
	args := []string{"link", "add", name, "type", "vxlan", "id", fmt.Sprintf("%d", vxlanID), "dev", parent}
	if group != "" {
		args = append(args, "group", group)
	}
	if port > 0 {
		args = append(args, "dstport", fmt.Sprintf("%d", port))
	} else {
		args = append(args, "dstport", "4789")
	}

	if ns != "" {
		m.commands = append(m.commands, fmt.Sprintf("ip %s", strings.Join(args, " ")))
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s netns %s", name, ns))
		m.commands = append(m.commands, fmt.Sprintf("ip netns exec %s ip link set %s up", ns, name))
	} else {
		m.commands = append(m.commands, fmt.Sprintf("ip %s", strings.Join(args, " ")))
		m.commands = append(m.commands, fmt.Sprintf("ip link set %s up", name))
	}
	return nil
}

func (m *DryRunNetworkManager) CreateDummy(name string, ns string) error {
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "add", name, "type", "dummy"))
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", name, "up"))
	return nil
}

func (m *DryRunNetworkManager) CreateBond(name string, interfaces []string, mode string, ns string) error {
	args := []string{"link", "add", name, "type", "bond"}
	if mode != "" {
		args = append(args, "mode", mode)
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", args...))

	for _, member := range interfaces {
		m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", member, "master", name))
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", name, "up"))
	return nil
}

func (m *DryRunNetworkManager) AddAddress(interfaceName, address string, ns string) error {
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "addr", "add", address, "dev", interfaceName))
	return nil
}

func (m *DryRunNetworkManager) DeleteInterface(name string, ns string) error {
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "del", name))
	return nil
}

func (m *DryRunNetworkManager) AddRoute(destination, gateway, interfaceName, ns string) error {
	args := []string{"route", "add"}
	if destination == "" {
		args = append(args, "default")
	} else {
		args = append(args, destination)
	}
	args = append(args, "via", gateway)
	if interfaceName != "" {
		args = append(args, "dev", interfaceName)
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", args...))
	return nil
}

func (m *DryRunNetworkManager) DeleteRoute(destination, gateway, interfaceName, ns string) error {
	args := []string{"route", "del"}
	if destination == "" {
		args = append(args, "default")
	} else {
		args = append(args, destination)
	}
	args = append(args, "via", gateway)
	if interfaceName != "" {
		args = append(args, "dev", interfaceName)
	}
	m.commands = append(m.commands, m.formatCmd(ns, "ip", args...))
	return nil
}

func (m *DryRunNetworkManager) AddNat(interfaceName, sourceSubnet string) error {
	m.commands = append(m.commands, "sysctl -w net.ipv4.ip_forward=1")
	m.commands = append(m.commands, fmt.Sprintf("iptables -t nat -A POSTROUTING -o %s -s %s -j MASQUERADE", interfaceName, sourceSubnet))
	return nil
}

func (m *DryRunNetworkManager) DeleteNat(interfaceName, sourceSubnet string) error {
	m.commands = append(m.commands, fmt.Sprintf("iptables -t nat -D POSTROUTING -o %s -s %s -j MASQUERADE", interfaceName, sourceSubnet))
	return nil
}

func (m *DryRunNetworkManager) SetMacAddress(interfaceName, mac string, ns string) error {
	m.commands = append(m.commands, m.formatCmd(ns, "ip", "link", "set", "dev", interfaceName, "address", mac))
	return nil
}
