package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ifaceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
)

type Config struct {
	Objects  []Object  `yaml:"objects"`
	Services []Service `yaml:"services"`
}

type Object struct {
	Type       string   `yaml:"type"`
	Name       string   `yaml:"name"`
	Owner      string   `yaml:"owner,omitempty"`
	Master     string   `yaml:"master,omitempty"`
	Peer       string   `yaml:"peer,omitempty"`
	Parent     string   `yaml:"parent,omitempty"`
	VlanID     int      `yaml:"vlan_id,omitempty"`
	VxlanID    int      `yaml:"vxlan_id,omitempty"`
	Group      string   `yaml:"group,omitempty"`
	Port       int      `yaml:"port,omitempty"`
	Interfaces []string `yaml:"interfaces,omitempty"`
	Mode       string   `yaml:"mode,omitempty"`
	Namespace    string   `yaml:"namespace,omitempty"`
	Interface    string   `yaml:"interface,omitempty"`
	Address      string   `yaml:"address,omitempty"`
	Destination  string   `yaml:"destination,omitempty"`
	Gateway      string   `yaml:"gateway,omitempty"`
	SourceSubnet string   `yaml:"source_subnet,omitempty"`
	Mac          string   `yaml:"mac,omitempty"`
}

type Service struct {
	Type           string   `yaml:"type"`
	Interface      string   `yaml:"interface"`
	Subnet         string   `yaml:"subnet,omitempty"`
	RangeStart     string   `yaml:"range_start,omitempty"`
	RangeEnd       string   `yaml:"range_end,omitempty"`
	Router         string   `yaml:"router,omitempty"`
	DNS            string   `yaml:"dns,omitempty"`
	Port           int      `yaml:"port,omitempty"`
	Root           string   `yaml:"root,omitempty"`
	Hosts          []Host   `yaml:"hosts,omitempty"`
	DhcpNextServer string        `yaml:"dhcp_next_server,omitempty"`
	DhcpBootfile   string        `yaml:"dhcp_bootfile,omitempty"`
	StaticLeases   []StaticLease `yaml:"static_leases,omitempty"`

	// Plugin Hook configurations
	Mode          string   `yaml:"mode,omitempty"`           // "exec" or "su-exec"
	Path          string   `yaml:"path,omitempty"`           // Path to startup binary/script
	Args          []string `yaml:"args,omitempty"`           // Startup arguments
	Script        string   `yaml:"script,omitempty"`         // Inline startup script
	CleanupPath   string   `yaml:"cleanup_path,omitempty"`   // Path to cleanup binary/script
	CleanupArgs   []string `yaml:"cleanup_args,omitempty"`   // Cleanup arguments
	CleanupScript string   `yaml:"cleanup_script,omitempty"` // Inline cleanup script
}

type StaticLease struct {
	Mac string `yaml:"mac"`
	IP  string `yaml:"ip"`
}

type Host struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Resolve environment variables in raw YAML content.
	// If running under sudo, temporarily override USER to the original user for environment expansion.
	originalUser := os.Getenv("USER")
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		os.Setenv("USER", sudoUser)
		defer os.Setenv("USER", originalUser)
	}
	resolvedData := os.ExpandEnv(string(data))

	var cfg Config
	err = yaml.Unmarshal([]byte(resolvedData), &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	cfg.normalizeAndDefault()

	return &cfg, nil
}

func (c *Config) normalizeAndDefault() {
	currentUser := os.Getenv("SUDO_USER")
	if currentUser == "" {
		currentUser = os.Getenv("USER")
	}
	if currentUser == "" {
		currentUser = "root"
	}

	for i := range c.Objects {
		c.Objects[i].Type = strings.ToLower(strings.TrimSpace(c.Objects[i].Type))
		if (c.Objects[i].Type == "tap" || c.Objects[i].Type == "tun") && c.Objects[i].Owner == "" {
			c.Objects[i].Owner = currentUser
		}
	}

	for i := range c.Services {
		c.Services[i].Type = strings.ToLower(strings.TrimSpace(c.Services[i].Type))
		if c.Services[i].Type == "plugin" {
			c.Services[i].Mode = strings.ToLower(strings.TrimSpace(c.Services[i].Mode))
			if c.Services[i].Mode == "" {
				c.Services[i].Mode = "su-exec"
			}
		}
	}
}

func (c *Config) Validate() []error {
	var errs []error

	validTypes := map[string]bool{
		"bridge":    true,
		"tap":       true,
		"tun":       true,
		"veth":      true,
		"vlan":      true,
		"vxlan":     true,
		"bond":      true,
		"dummy":     true,
		"namespace": true,
		"address":   true,
		"route":     true,
		"nat":       true,
	}

	validServices := map[string]bool{
		"dhcp":   true,
		"dns":    true,
		"http":   true,
		"tftp":   true,
		"pxe":    true,
		"plugin": true,
	}

	definedInterfaces := make(map[string]bool)
	definedNamespaces := make(map[string]bool)

	// First pass: collect namespaces and interfaces, validate interface names
	for idx, obj := range c.Objects {
		if obj.Type == "" {
			errs = append(errs, fmt.Errorf("object at index %d has no type", idx))
			continue
		}
		if !validTypes[obj.Type] {
			errs = append(errs, fmt.Errorf("object %q at index %d: invalid type %q", obj.Name, idx, obj.Type))
			continue
		}

		if obj.Type == "namespace" {
			if obj.Name == "" {
				errs = append(errs, fmt.Errorf("namespace at index %d has no name", idx))
				continue
			}
			if definedNamespaces[obj.Name] {
				errs = append(errs, fmt.Errorf("duplicate namespace name %q", obj.Name))
			}
			definedNamespaces[obj.Name] = true
			continue
		}

		if obj.Type == "address" || obj.Type == "route" || obj.Type == "nat" {
			continue
		}

		if obj.Name == "" {
			errs = append(errs, fmt.Errorf("object of type %q at index %d has no name", obj.Type, idx))
			continue
		}

		if len(obj.Name) > 15 {
			errs = append(errs, fmt.Errorf("interface name %q exceeds Linux limit of 15 characters", obj.Name))
		}

		if !ifaceNameRegex.MatchString(obj.Name) {
			errs = append(errs, fmt.Errorf("interface name %q contains invalid characters", obj.Name))
		}

		if definedInterfaces[obj.Name] {
			errs = append(errs, fmt.Errorf("duplicate interface name %q", obj.Name))
		}
		definedInterfaces[obj.Name] = true

		if obj.Type == "veth" && obj.Peer != "" {
			if len(obj.Peer) > 15 {
				errs = append(errs, fmt.Errorf("veth peer name %q exceeds Linux limit of 15 characters", obj.Peer))
			}
			if !ifaceNameRegex.MatchString(obj.Peer) {
				errs = append(errs, fmt.Errorf("veth peer name %q contains invalid characters", obj.Peer))
			}
			if definedInterfaces[obj.Peer] {
				errs = append(errs, fmt.Errorf("duplicate interface name (used by veth peer) %q", obj.Peer))
			}
			definedInterfaces[obj.Peer] = true
		}
	}

	// Second pass: validate fields and references
	for idx, obj := range c.Objects {
		if !validTypes[obj.Type] {
			continue
		}

		if obj.Mac != "" {
			_, err := net.ParseMAC(obj.Mac)
			if err != nil {
				errs = append(errs, fmt.Errorf("object %q has invalid MAC address %q: %w", obj.Name, obj.Mac, err))
			}
		}

		if obj.Namespace != "" && !definedNamespaces[obj.Namespace] {
			errs = append(errs, fmt.Errorf("object %q references undefined namespace %q", obj.Name, obj.Namespace))
		}

		switch obj.Type {
		case "tap":
			if obj.Master != "" && !definedInterfaces[obj.Master] {
				errs = append(errs, fmt.Errorf("tap %q references undefined master bridge %q", obj.Name, obj.Master))
			}
		case "veth":
			if obj.Peer == "" {
				errs = append(errs, fmt.Errorf("veth %q requires a peer interface name", obj.Name))
			}
			if obj.Master != "" && !definedInterfaces[obj.Master] {
				errs = append(errs, fmt.Errorf("veth %q references undefined master bridge %q", obj.Name, obj.Master))
			}
		case "vlan":
			if obj.Parent == "" {
				errs = append(errs, fmt.Errorf("vlan %q requires a parent interface", obj.Name))
			}
			vlanID := obj.VlanID
			if vlanID < 1 || vlanID > 4094 {
				errs = append(errs, fmt.Errorf("vlan %q has invalid ID %d (must be between 1 and 4094)", obj.Name, vlanID))
			}
		case "vxlan":
			if obj.Parent == "" {
				errs = append(errs, fmt.Errorf("vxlan %q requires a parent interface", obj.Name))
			}
			vxlanID := obj.VxlanID
			if vxlanID == 0 {
				vxlanID = obj.VlanID // support both
			}
			if vxlanID < 1 || vxlanID > 16777215 {
				errs = append(errs, fmt.Errorf("vxlan %q has invalid ID %d (must be between 1 and 16777215)", obj.Name, vxlanID))
			}
			if obj.Group != "" {
				ip := net.ParseIP(obj.Group)
				if ip == nil || !ip.IsMulticast() {
					errs = append(errs, fmt.Errorf("vxlan %q group %q is not a valid multicast IP", obj.Name, obj.Group))
				}
			}
		case "bond":
			if len(obj.Interfaces) == 0 {
				errs = append(errs, fmt.Errorf("bond %q requires at least one interface member", obj.Name))
			}
			for _, member := range obj.Interfaces {
				if !definedInterfaces[member] {
					errs = append(errs, fmt.Errorf("bond %q references undefined interface member %q", obj.Name, member))
				}
			}
		case "address":
			if obj.Interface == "" {
				errs = append(errs, fmt.Errorf("address entry at index %d has no interface defined", idx))
			}
			if obj.Address == "" {
				errs = append(errs, fmt.Errorf("address entry for interface %q has no address defined", obj.Interface))
			} else {
				_, _, err := net.ParseCIDR(obj.Address)
				if err != nil {
					errs = append(errs, fmt.Errorf("address %q for interface %q is not in valid CIDR format (e.g. 192.168.1.1/24)", obj.Address, obj.Interface))
				}
			}
		case "route":
			if obj.Gateway == "" {
				errs = append(errs, fmt.Errorf("route entry requires a gateway IP"))
			} else if net.ParseIP(obj.Gateway) == nil {
				errs = append(errs, fmt.Errorf("route entry gateway %q is not a valid IP", obj.Gateway))
			}
			if obj.Destination != "" {
				if !strings.Contains(obj.Destination, "/") {
					// Default to host route (/32) if no mask
					if net.ParseIP(obj.Destination) == nil {
						errs = append(errs, fmt.Errorf("route entry destination %q is not a valid IP", obj.Destination))
					}
				} else {
					_, _, err := net.ParseCIDR(obj.Destination)
					if err != nil {
						errs = append(errs, fmt.Errorf("route entry destination %q is not a valid CIDR", obj.Destination))
					}
				}
			}
		case "nat":
			if obj.Interface == "" {
				errs = append(errs, fmt.Errorf("nat entry requires an interface"))
			}
			if obj.SourceSubnet == "" {
				errs = append(errs, fmt.Errorf("nat entry requires a source_subnet"))
			} else {
				_, _, err := net.ParseCIDR(obj.SourceSubnet)
				if err != nil {
					errs = append(errs, fmt.Errorf("nat entry source_subnet %q is not in valid CIDR format", obj.SourceSubnet))
				}
			}
		}
	}

	// Validate services
	for idx, svc := range c.Services {
		if svc.Type == "" {
			errs = append(errs, fmt.Errorf("service at index %d has no type", idx))
			continue
		}
		if !validServices[svc.Type] {
			errs = append(errs, fmt.Errorf("service %q at index %d: invalid type", svc.Type, idx))
			continue
		}

		if svc.Interface == "" {
			errs = append(errs, fmt.Errorf("service of type %q at index %d: interface is required", svc.Type, idx))
		} else if !definedInterfaces[svc.Interface] {
			errs = append(errs, fmt.Errorf("service of type %q references undefined interface %q", svc.Type, svc.Interface))
		}

		// DHCP/PXE specific validations
		if svc.Type == "dhcp" || svc.Type == "pxe" {
			if svc.Subnet != "" {
				_, _, err := net.ParseCIDR(svc.Subnet)
				if err != nil {
					errs = append(errs, fmt.Errorf("service %s: invalid subnet CIDR %q", svc.Type, svc.Subnet))
				}
			}
			if svc.RangeStart != "" && net.ParseIP(svc.RangeStart) == nil {
				errs = append(errs, fmt.Errorf("service %s: invalid range_start IP %q", svc.Type, svc.RangeStart))
			}
			if svc.RangeEnd != "" && net.ParseIP(svc.RangeEnd) == nil {
				errs = append(errs, fmt.Errorf("service %s: invalid range_end IP %q", svc.Type, svc.RangeEnd))
			}

			// Validate static leases
			for lidx, lease := range svc.StaticLeases {
				if lease.Mac == "" {
					errs = append(errs, fmt.Errorf("service %s: static lease at index %d has no MAC address defined", svc.Type, lidx))
				} else {
					_, err := net.ParseMAC(lease.Mac)
					if err != nil {
						errs = append(errs, fmt.Errorf("service %s: static lease at index %d has invalid MAC address %q: %w", svc.Type, lidx, lease.Mac, err))
					}
				}

				if lease.IP == "" {
					errs = append(errs, fmt.Errorf("service %s: static lease at index %d has no IP address defined", svc.Type, lidx))
				} else {
					if net.ParseIP(lease.IP) == nil {
						errs = append(errs, fmt.Errorf("service %s: static lease at index %d has invalid IP address %q", svc.Type, lidx, lease.IP))
					}
				}
			}
		}

		// HTTP specific validations
		if svc.Type == "http" {
			if svc.Root != "" {
				_, err := os.Stat(svc.Root)
				if err != nil {
					errs = append(errs, fmt.Errorf("service http: root directory %q does not exist: %w", svc.Root, err))
				}
			}
		}

		// PXE specific validations
		if svc.Type == "pxe" {
			if svc.DhcpBootfile == "" {
				errs = append(errs, fmt.Errorf("service pxe requires dhcp_bootfile to be set"))
			}
		}

		// Plugin specific validations
		if svc.Type == "plugin" {
			if svc.Script == "" && svc.Path == "" {
				errs = append(errs, fmt.Errorf("service plugin: either 'script' or 'path' must be specified"))
			}
			if svc.Script != "" && svc.Path != "" {
				errs = append(errs, fmt.Errorf("service plugin: cannot specify both 'script' and 'path'"))
			}
			if svc.Mode != "exec" && svc.Mode != "su-exec" {
				errs = append(errs, fmt.Errorf("service plugin: invalid mode %q (must be 'exec' or 'su-exec')", svc.Mode))
			}
		}
	}

	return errs
}

func (c *Config) GetErrors() error {
	errs := c.Validate()
	if len(errs) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("validation failed with the following errors:\n")
	for _, err := range errs {
		sb.WriteString(fmt.Sprintf("  - %s\n", err.Error()))
	}
	return errors.New(sb.String())
}
