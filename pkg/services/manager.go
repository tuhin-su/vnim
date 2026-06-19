package services

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tuhin-su/vnim/pkg/config"
)

type ServiceManager struct {
	planName string
	stateDir string
}

func NewServiceManager(planName string) *ServiceManager {
	return &ServiceManager{
		planName: planName,
		stateDir: filepath.Join("/var/run/vnim/plans", planName),
	}
}

func getInterfaceIP(name string, ns string) (net.IP, *net.IPNet, error) {
	var cmd *exec.Cmd
	if ns != "" {
		cmd = exec.Command("ip", "netns", "exec", ns, "ip", "-4", "addr", "show", name)
	} else {
		cmd = exec.Command("ip", "-4", "addr", "show", name)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to show addr: %w (output: %q)", err, string(out))
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip, ipnet, err := net.ParseCIDR(parts[1])
				if err == nil {
					return ip, ipnet, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("no IPv4 address found for interface %s", name)
}

func fillDefaultDHCP(svc *config.Service, ns string) error {
	// Wait a moment for interface to be fully up and address assigned
	time.Sleep(500 * time.Millisecond)

	ip, ipnet, err := getInterfaceIP(svc.Interface, ns)
	if err != nil {
		return fmt.Errorf("failed to get interface IP for default DHCP: %w", err)
	}

	ones, bits := ipnet.Mask.Size()
	if bits != 32 {
		return fmt.Errorf("interface must have an IPv4 address to run DHCP")
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("interface has no IPv4 address")
	}

	if svc.Subnet == "" {
		svc.Subnet = ipnet.String()
	}
	if svc.Router == "" {
		svc.Router = ip.String()
	}
	if svc.DNS == "" {
		svc.DNS = "1.1.1.1"
	}

	if svc.RangeStart == "" {
		startIP := make(net.IP, 4)
		copy(startIP, ip4)
		if ones <= 24 {
			startIP[3] = 50
		} else {
			startIP[3] = ip4[3] + 1
		}
		svc.RangeStart = startIP.String()
	}

	if svc.RangeEnd == "" {
		endIP := make(net.IP, 4)
		copy(endIP, ip4)
		if ones <= 24 {
			endIP[3] = 150
		} else {
			endIP[3] = ip4[3] + 2
		}
		svc.RangeEnd = endIP.String()
	}

	return nil
}

func (sm *ServiceManager) StartService(idx int, svc config.Service, ns string) (int, error) {
	if err := os.MkdirAll(sm.stateDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create plan state dir: %w", err)
	}

	switch svc.Type {
	case "dhcp":
		if err := fillDefaultDHCP(&svc, ns); err != nil {
			return 0, err
		}
		return sm.startDnsmasq(idx, svc, ns)
	case "dns":
		return sm.startDnsmasq(idx, svc, ns)
	case "tftp":
		return sm.startDnsmasq(idx, svc, ns)
	case "pxe":
		if err := fillDefaultDHCP(&svc, ns); err != nil {
			return 0, err
		}
		return sm.startDnsmasq(idx, svc, ns)
	case "http":
		return sm.startHTTP(idx, svc, ns)
	default:
		return 0, fmt.Errorf("unknown service type %q", svc.Type)
	}
}

func (sm *ServiceManager) startDnsmasq(idx int, svc config.Service, ns string) (int, error) {
	confPath := filepath.Join(sm.stateDir, fmt.Sprintf("dnsmasq_%d.conf", idx))
	logPath := filepath.Join(sm.stateDir, fmt.Sprintf("dnsmasq_%d.log", idx))

	var conf strings.Builder
	conf.WriteString("keep-in-foreground\n")
	conf.WriteString(fmt.Sprintf("interface=%s\n", svc.Interface))
	conf.WriteString("bind-interfaces\n")

	// Avoid port conflicts for non-DNS services
	if svc.Type != "dns" && svc.Type != "pxe" {
		conf.WriteString("port=0\n")
	} else if svc.Type == "dns" {
		port := svc.Port
		if port == 0 {
			port = 53
		}
		conf.WriteString(fmt.Sprintf("port=%d\n", port))
		for _, host := range svc.Hosts {
			conf.WriteString(fmt.Sprintf("address=/%s/%s\n", host.Name, host.IP))
		}
	}

	if svc.Type == "dhcp" || svc.Type == "pxe" {
		conf.WriteString(fmt.Sprintf("dhcp-range=%s,%s,12h\n", svc.RangeStart, svc.RangeEnd))
		if svc.Router != "" {
			conf.WriteString(fmt.Sprintf("dhcp-option=option:router,%s\n", svc.Router))
		}
		if svc.DNS != "" {
			conf.WriteString(fmt.Sprintf("dhcp-option=option:dns-server,%s\n", svc.DNS))
		}
		for _, lease := range svc.StaticLeases {
			conf.WriteString(fmt.Sprintf("dhcp-host=%s,%s\n", lease.Mac, lease.IP))
		}
	}

	if svc.Type == "tftp" || svc.Type == "pxe" {
		conf.WriteString("enable-tftp\n")
		root := svc.Root
		if root == "" {
			root = "/var/lib/tftpboot"
		}
		conf.WriteString(fmt.Sprintf("tftp-root=%s\n", root))
	}

	if svc.Type == "pxe" {
		if svc.DhcpNextServer != "" {
			conf.WriteString(fmt.Sprintf("dhcp-boot=%s,,%s\n", svc.DhcpBootfile, svc.DhcpNextServer))
		} else {
			conf.WriteString(fmt.Sprintf("dhcp-boot=%s\n", svc.DhcpBootfile))
		}
	}

	if err := os.WriteFile(confPath, []byte(conf.String()), 0644); err != nil {
		return 0, fmt.Errorf("failed to write dnsmasq config: %w", err)
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create log file: %w", err)
	}

	// Prepare dnsmasq command
	args := []string{"dnsmasq", "-C", confPath}
	var cmd *exec.Cmd
	if ns != "" {
		cmd = exec.Command("ip", append([]string{"netns", "exec", ns}, args...)...)
	} else {
		cmd = exec.Command(args[0], args[1:]...)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("failed to start dnsmasq: %w", err)
	}

	// Close file handle in parent process, it stays open in the child
	logFile.Close()

	// Wait briefly to check if it crashed immediately
	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Wait()
	}()

	select {
	case err := <-errChan:
		return 0, fmt.Errorf("dnsmasq exited immediately: %w (check logs at %s)", err, logPath)
	case <-time.After(500 * time.Millisecond):
		// Command is running
		return cmd.Process.Pid, nil
	}
}

func (sm *ServiceManager) startHTTP(idx int, svc config.Service, ns string) (int, error) {
	logPath := filepath.Join(sm.stateDir, fmt.Sprintf("http_%d.log", idx))
	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create log file: %w", err)
	}

	port := svc.Port
	if port == 0 {
		port = 8080
	}

	root := svc.Root
	if root == "" {
		root = "."
	}

	// Invoke ourselves with the run-http subcommand
	selfPath, err := os.Executable()
	if err != nil {
		selfPath = "vnim"
	}

	args := []string{"run-http", "--port", strconv.Itoa(port), "--root", root}
	var cmd *exec.Cmd
	if ns != "" {
		cmd = exec.Command("ip", append([]string{"netns", "exec", ns, selfPath}, args...)...)
	} else {
		cmd = exec.Command(selfPath, args...)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("failed to start HTTP server: %w", err)
	}

	logFile.Close()

	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Wait()
	}()

	select {
	case err := <-errChan:
		return 0, fmt.Errorf("HTTP server exited immediately: %w (check logs at %s)", err, logPath)
	case <-time.After(500 * time.Millisecond):
		return cmd.Process.Pid, nil
	}
}

func RunHTTPServer(port int, rootDir string) error {
	if rootDir == "" {
		rootDir = "."
	}
	fs := http.FileServer(http.Dir(rootDir))
	http.Handle("/", fs)

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("Starting built-in HTTP server on %s serving %s...\n", addr, rootDir)
	return http.ListenAndServe(addr, nil)
}
