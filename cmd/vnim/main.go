package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tuhin-su/vnim/pkg/config"
	"github.com/tuhin-su/vnim/pkg/network"
	"github.com/tuhin-su/vnim/pkg/services"
	"github.com/tuhin-su/vnim/pkg/state"
	"github.com/tuhin-su/vnim/pkg/tree"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subCmd := os.Args[1]

	// Built-in HTTP server runner (used for background HTTP services)
	if subCmd == "run-http" {
		fs := flag.NewFlagSet("run-http", flag.ExitOnError)
		port := fs.Int("port", 8080, "Port to serve on")
		root := fs.String("root", ".", "Root directory to serve")
		_ = fs.Parse(os.Args[2:])

		if err := services.RunHTTPServer(*port, *root); err != nil {
			fmt.Fprintf(os.Stderr, "HTTP Server Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Secret commands invoked internally as root
	if subCmd == "helper-up" {
		if !isRoot() {
			fmt.Fprintln(os.Stderr, "Error: helper-up must run as root")
			os.Exit(1)
		}
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: missing yaml path")
			os.Exit(1)
		}
		handleHelperUp(os.Args[2])
		os.Exit(0)
	}

	if subCmd == "helper-down" {
		if !isRoot() {
			fmt.Fprintln(os.Stderr, "Error: helper-down must run as root")
			os.Exit(1)
		}
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: missing plan spec")
			os.Exit(1)
		}
		handleHelperDown(os.Args[2])
		os.Exit(0)
	}

	switch subCmd {
	case "up":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: vnim up <lab.yaml>")
			os.Exit(1)
		}
		yamlPath := os.Args[2]
		if !isRoot() {
			escalate("up", yamlPath)
		} else {
			handleHelperUp(yamlPath)
		}

	case "down":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: vnim down <lab.yaml|plan_name>")
			os.Exit(1)
		}
		target := os.Args[2]
		if !isRoot() {
			escalate("down", target)
		} else {
			handleHelperDown(target)
		}

	case "validate":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: vnim validate <lab.yaml>")
			os.Exit(1)
		}
		handleValidate(os.Args[2])

	case "dry-run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: vnim dry-run <lab.yaml>")
			os.Exit(1)
		}
		handleDryRun(os.Args[2])

	case "tree":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: vnim tree <lab.yaml>")
			os.Exit(1)
		}
		handleTree(os.Args[2])

	case "ps":
		handlePs()

	case "exec":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: vnim exec <namespace> <command> [args...]")
			os.Exit(1)
		}
		ns := os.Args[2]
		cmdArgs := os.Args[3:]
		handleExec(ns, cmdArgs)

	case "shell":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: vnim shell <namespace>")
			os.Exit(1)
		}
		ns := os.Args[2]
		handleShell(ns)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\n", subCmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("vnim - Virtual Network Interface Manager")
	fmt.Println("\nUsage:")
	fmt.Println("  vnim up <lab.yaml>          Create and start network topology")
	fmt.Println("  vnim down <lab.yaml|name>   Destroy network topology")
	fmt.Println("  vnim validate <lab.yaml>    Validate network topology configuration")
	fmt.Println("  vnim dry-run <lab.yaml>     Show commands that would be executed")
	fmt.Println("  vnim tree <lab.yaml>        View network topology hierarchy")
	fmt.Println("  vnim ps                     List all active network topologies")
	fmt.Println("  vnim exec <ns> <cmd>...     Run command inside namespace context")
	fmt.Println("  vnim shell <ns>             Open interactive shell inside namespace")
}

func isRoot() bool {
	return os.Getuid() == 0
}

func escalate(subCmd string, target string) {
	self, err := os.Executable()
	if err != nil {
		self = "vnim"
	}

	args := []string{"-E", self, "helper-" + subCmd, target}
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func getPlanName(input string) string {
	if strings.HasSuffix(input, ".yaml") || strings.HasSuffix(input, ".yml") {
		base := filepath.Base(input)
		return strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
	}
	return input
}

func handleValidate(yamlPath string) {
	cfg, err := config.LoadConfig(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to parse configuration: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.GetErrors(); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	fmt.Println("Configuration is valid.")
}

func handleDryRun(yamlPath string) {
	cfg, err := config.LoadConfig(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.GetErrors(); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	planName := getPlanName(yamlPath)
	nm := network.NewDryRunNetworkManager()

	// Dry run namespace creation
	for _, obj := range cfg.Objects {
		if obj.Type == "namespace" {
			_ = nm.CreateNamespace(obj.Name)
		}
	}

	// Dry run bridges
	for _, obj := range cfg.Objects {
		if obj.Type == "bridge" {
			_ = nm.CreateBridge(obj.Name, obj.Namespace)
		}
	}

	// Dry run other interfaces
	for _, obj := range cfg.Objects {
		switch obj.Type {
		case "tap":
			_ = nm.CreateTap(obj.Name, obj.Owner, obj.Master, obj.Namespace)
		case "tun":
			_ = nm.CreateTun(obj.Name, obj.Owner, obj.Namespace)
		case "veth":
			_ = nm.CreateVeth(obj.Name, obj.Peer, obj.Master, obj.Namespace)
		case "vlan":
			_ = nm.CreateVlan(obj.Name, obj.Parent, obj.VlanID, obj.Namespace)
		case "vxlan":
			vni := obj.VxlanID
			if vni == 0 {
				vni = obj.VlanID
			}
			_ = nm.CreateVxlan(obj.Name, obj.Parent, vni, obj.Group, obj.Port, obj.Namespace)
		case "dummy":
			_ = nm.CreateDummy(obj.Name, obj.Namespace)
		case "bond":
			_ = nm.CreateBond(obj.Name, obj.Interfaces, obj.Mode, obj.Namespace)
		}
	}

	// Dry run MAC configurations
	for _, obj := range cfg.Objects {
		if obj.Mac != "" && obj.Type != "address" && obj.Type != "namespace" && obj.Type != "route" && obj.Type != "nat" {
			_ = nm.SetMacAddress(obj.Name, obj.Mac, obj.Namespace)
		}
	}

	// Dry run addresses
	for _, obj := range cfg.Objects {
		if obj.Type == "address" {
			_ = nm.AddAddress(obj.Interface, obj.Address, obj.Namespace)
		}
	}

	// Dry run routes
	for _, obj := range cfg.Objects {
		if obj.Type == "route" {
			_ = nm.AddRoute(obj.Destination, obj.Gateway, obj.Interface, obj.Namespace)
		}
	}

	// Dry run NAT rules
	for _, obj := range cfg.Objects {
		if obj.Type == "nat" {
			_ = nm.AddNat(obj.Interface, obj.SourceSubnet)
		}
	}

	fmt.Printf("Dry-run commands for topology %q:\n", planName)
	for _, cmd := range nm.GetCommands() {
		fmt.Printf("  %s\n", cmd)
	}

	if len(cfg.Services) > 0 {
		fmt.Println("\nServices to start:")
		for _, svc := range cfg.Services {
			fmt.Printf("  - %s on interface %s", svc.Type, svc.Interface)
			var ns string
			for _, obj := range cfg.Objects {
				if obj.Name == svc.Interface {
					ns = obj.Namespace
					break
				}
			}
			if ns != "" {
				fmt.Printf(" (namespace: %s)", ns)
			}
			fmt.Println()

			if svc.Type == "dhcp" || svc.Type == "pxe" {
				fmt.Printf("    * Subnet: %s\n", svc.Subnet)
				fmt.Printf("    * IP range: %s to %s\n", svc.RangeStart, svc.RangeEnd)
				fmt.Printf("    * DNS server option: %s\n", svc.DNS)
				fmt.Printf("    * Router option: %s\n", svc.Router)
			}
			if svc.Type == "dns" {
				port := svc.Port
				if port == 0 {
					port = 53
				}
				fmt.Printf("    * DNS port: %d\n", port)
				for _, h := range svc.Hosts {
					fmt.Printf("    * Host record: %s -> %s\n", h.Name, h.IP)
				}
			}
			if svc.Type == "tftp" {
				root := svc.Root
				if root == "" {
					root = "/var/lib/tftpboot"
				}
				fmt.Printf("    * TFTP root: %s\n", root)
			}
			if svc.Type == "pxe" {
				fmt.Printf("    * TFTP Bootfile: %s\n", svc.DhcpBootfile)
				if svc.DhcpNextServer != "" {
					fmt.Printf("    * TFTP Next Server: %s\n", svc.DhcpNextServer)
				}
			}
			if svc.Type == "http" {
				port := svc.Port
				if port == 0 {
					port = 8080
				}
				root := svc.Root
				if root == "" {
					root = "."
				}
				fmt.Printf("    * HTTP server on port %d serving %s\n", port, root)
			}
		}
	}
}

func handleTree(yamlPath string) {
	cfg, err := config.LoadConfig(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	roots := tree.BuildTree(cfg)
	fmt.Print(tree.RenderTree(roots))
}

func handlePs() {
	states, err := state.ListStates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing plans: %v\n", err)
		os.Exit(1)
	}

	if len(states) == 0 {
		fmt.Println("No active topologies.")
		return
	}

	fmt.Printf("%-15s %-40s %-25s %-30s\n", "PLAN", "YAML PATH", "CREATED AT", "SERVICES")
	fmt.Println(strings.Repeat("-", 113))

	for _, ps := range states {
		var services []string
		for _, s := range ps.Services {
			services = append(services, s.Type)
		}
		svcStr := strings.Join(services, ", ")
		if svcStr == "" {
			svcStr = "none"
		}

		createdStr := ps.CreatedAt.Format("2006-01-02 15:04:05")
		fmt.Printf("%-15s %-40s %-25s %-30s\n", ps.Name, ps.YAMLPath, createdStr, svcStr)
	}
}

func handleHelperUp(yamlPath string) {
	cfg, err := config.LoadConfig(yamlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.GetErrors(); err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	planName := getPlanName(yamlPath)

	if _, err := state.LoadState(planName); err == nil {
		fmt.Fprintf(os.Stderr, "Error: Plan %q is already active. Run 'vnim down %s' first.\n", planName, yamlPath)
		os.Exit(1)
	}

	nm := network.NewCmdNetworkManager()

	// Create namespaces
	for _, obj := range cfg.Objects {
		if obj.Type == "namespace" {
			if err := nm.CreateNamespace(obj.Name); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating namespace: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Create bridges
	for _, obj := range cfg.Objects {
		if obj.Type == "bridge" {
			if err := nm.CreateBridge(obj.Name, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating bridge: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Create other interfaces
	for _, obj := range cfg.Objects {
		switch obj.Type {
		case "tap":
			if err := nm.CreateTap(obj.Name, obj.Owner, obj.Master, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating tap: %v\n", err)
				os.Exit(1)
			}
		case "tun":
			if err := nm.CreateTun(obj.Name, obj.Owner, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating tun: %v\n", err)
				os.Exit(1)
			}
		case "veth":
			if err := nm.CreateVeth(obj.Name, obj.Peer, obj.Master, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating veth: %v\n", err)
				os.Exit(1)
			}
		case "vlan":
			if err := nm.CreateVlan(obj.Name, obj.Parent, obj.VlanID, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating vlan: %v\n", err)
				os.Exit(1)
			}
		case "vxlan":
			vni := obj.VxlanID
			if vni == 0 {
				vni = obj.VlanID
			}
			if err := nm.CreateVxlan(obj.Name, obj.Parent, vni, obj.Group, obj.Port, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating vxlan: %v\n", err)
				os.Exit(1)
			}
		case "dummy":
			if err := nm.CreateDummy(obj.Name, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating dummy: %v\n", err)
				os.Exit(1)
			}
		case "bond":
			if err := nm.CreateBond(obj.Name, obj.Interfaces, obj.Mode, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error creating bond: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Apply MAC addresses if specified
	for _, obj := range cfg.Objects {
		if obj.Mac != "" && obj.Type != "address" && obj.Type != "namespace" && obj.Type != "route" && obj.Type != "nat" {
			if err := nm.SetMacAddress(obj.Name, obj.Mac, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error setting MAC address for interface %s: %v\n", obj.Name, err)
				os.Exit(1)
			}
		}
	}

	for _, obj := range cfg.Objects {
		if obj.Type == "address" {
			if err := nm.AddAddress(obj.Interface, obj.Address, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error adding address: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Add routes
	for _, obj := range cfg.Objects {
		if obj.Type == "route" {
			if err := nm.AddRoute(obj.Destination, obj.Gateway, obj.Interface, obj.Namespace); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error adding route: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Add NAT rules
	for _, obj := range cfg.Objects {
		if obj.Type == "nat" {
			if err := nm.AddNat(obj.Interface, obj.SourceSubnet); err != nil {
				rollback(nm, cfg)
				fmt.Fprintf(os.Stderr, "Error adding NAT: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Start services
	sm := services.NewServiceManager(planName)
	var activeServices []state.ActiveService

	for idx, svc := range cfg.Services {
		var ns string
		for _, obj := range cfg.Objects {
			if obj.Name == svc.Interface {
				ns = obj.Namespace
				break
			}
		}

		pid, err := sm.StartService(idx, svc, ns)
		if err != nil {
			stopActiveServices(activeServices)
			rollback(nm, cfg)
			fmt.Fprintf(os.Stderr, "Error starting service %s: %v\n", svc.Type, err)
			os.Exit(1)
		}

		activeServices = append(activeServices, state.ActiveService{
			Type:      svc.Type,
			Interface: svc.Interface,
			PID:       pid,
		})
	}

	// Save state
	planState := &state.PlanState{
		Name:      planName,
		YAMLPath:  filepath.Clean(yamlPath),
		CreatedAt: time.Now(),
		Objects:   cfg.Objects,
		Services:  activeServices,
	}

	if err := state.SaveState(planName, planState); err != nil {
		stopActiveServices(activeServices)
		rollback(nm, cfg)
		fmt.Fprintf(os.Stderr, "Error saving state file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Topology %q successfully deployed!\n", planName)
}

func handleHelperDown(target string) {
	planName := getPlanName(target)
	ps, err := state.LoadState(planName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Plan %q is not active or state file not found.\n", planName)
		os.Exit(1)
	}

	fmt.Printf("Destroying topology %q...\n", planName)

	// 1. Stop all services
	stopActiveServices(ps.Services)

	// 2. Delete interfaces and namespaces in reverse order
	nm := network.NewCmdNetworkManager()
	for i := len(ps.Objects) - 1; i >= 0; i-- {
		obj := ps.Objects[i]
		if obj.Type == "address" {
			continue
		}
		if obj.Type == "route" {
			fmt.Printf("Deleting route to %s...\n", obj.Destination)
			if err := nm.DeleteRoute(obj.Destination, obj.Gateway, obj.Interface, obj.Namespace); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete route: %v\n", err)
			}
		} else if obj.Type == "nat" {
			fmt.Printf("Deleting NAT rule on %s...\n", obj.Interface)
			if err := nm.DeleteNat(obj.Interface, obj.SourceSubnet); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete NAT rule: %v\n", err)
			}
		} else if obj.Type == "namespace" {
			if err := nm.DeleteNamespace(obj.Name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete namespace %q: %v\n", obj.Name, err)
			}
		} else {
			if err := nm.DeleteInterface(obj.Name, obj.Namespace); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete interface %q: %v\n", obj.Name, err)
			}
		}
	}

	// 3. Remove state files and subdirectories
	if err := state.DeleteState(planName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to delete state: %v\n", err)
	}

	fmt.Printf("Topology %q successfully destroyed.\n", planName)
}

func rollback(nm network.NetworkManager, cfg *config.Config) {
	fmt.Println("Rolling back created resources...")
	for i := len(cfg.Objects) - 1; i >= 0; i-- {
		obj := cfg.Objects[i]
		if obj.Type == "address" {
			continue
		}
		if obj.Type == "route" {
			_ = nm.DeleteRoute(obj.Destination, obj.Gateway, obj.Interface, obj.Namespace)
		} else if obj.Type == "nat" {
			_ = nm.DeleteNat(obj.Interface, obj.SourceSubnet)
		} else if obj.Type == "namespace" {
			_ = nm.DeleteNamespace(obj.Name)
		} else {
			_ = nm.DeleteInterface(obj.Name, obj.Namespace)
		}
	}
}

func stopActiveServices(svcs []state.ActiveService) {
	for _, svc := range svcs {
		if svc.PID > 0 {
			terminateProcess(svc.PID)
		}
	}
}

func terminateProcess(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)

	// Poll using signal 0 to check if process is dead
	for i := 0; i < 20; i++ { // wait up to 2 seconds
		err := proc.Signal(syscall.Signal(0))
		if err != nil {
			// Process is dead
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if still alive
	_ = proc.Signal(syscall.SIGKILL)

	// Poll again to ensure it is killed
	for i := 0; i < 10; i++ {
		err := proc.Signal(syscall.Signal(0))
		if err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func handleExec(ns string, cmdArgs []string) {
	if !namespaceExists(ns) {
		fmt.Fprintf(os.Stderr, "Error: Namespace %q does not exist.\n", ns)
		os.Exit(1)
	}

	var cmd *exec.Cmd
	if !isRoot() {
		// Run via sudo to execute inside the namespace
		args := append([]string{"ip", "netns", "exec", ns}, cmdArgs...)
		cmd = exec.Command("sudo", args...)
	} else {
		cmd = exec.Command("ip", "netns", "exec", ns, cmdArgs[0])
		cmd.Args = append([]string{"ip", "netns", "exec", ns}, cmdArgs...)
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running command in namespace: %v\n", err)
		os.Exit(1)
	}
}

func handleShell(ns string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	fmt.Printf("Dropping into interactive shell inside namespace %q. Type 'exit' to return.\n", ns)
	handleExec(ns, []string{shell})
}

func namespaceExists(ns string) bool {
	_, err := os.Stat(filepath.Join("/var/run/netns", ns))
	if err == nil {
		return true
	}

	c := exec.Command("ip", "netns", "list")
	out, err := c.Output()
	if err != nil {
		return false
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == ns {
			return true
		}
	}
	return false
}
