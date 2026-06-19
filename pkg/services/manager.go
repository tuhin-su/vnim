package services

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
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
	case "plugin":
		return sm.startPlugin(idx, svc, ns)
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

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

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

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

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

func getOriginalUser() string {
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		return sudoUser
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "root"
}

func getInterfaceMAC(name string, ns string) (string, error) {
	var cmd *exec.Cmd
	if ns != "" {
		cmd = exec.Command("ip", "netns", "exec", ns, "ip", "link", "show", name)
	} else {
		cmd = exec.Command("ip", "link", "show", name)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "link/ether ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("no ethernet MAC address found in output")
}

type templateCtx struct {
	Interface string
	Namespace string
	Mac       string
	IP        string
	IPNoMask  string
	PlanName  string
	StateDir  string
	Owner     string
}

func compileTemplate(tmplStr string, ctx templateCtx) (string, error) {
	tmpl, err := template.New("plugin").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return buf.String(), nil
}

func (sm *ServiceManager) compileAndWriteScript(scriptContent string, filename string, ctx templateCtx) (string, error) {
	compiledScript, err := compileTemplate(scriptContent, ctx)
	if err != nil {
		return "", err
	}

	// Ensure script starts with a shebang line so it is executable by the shell
	trimmed := strings.TrimSpace(compiledScript)
	if !strings.HasPrefix(trimmed, "#!") {
		compiledScript = "#!/bin/sh\n" + compiledScript
	}

	scriptPath := filepath.Join(sm.stateDir, filename)
	if err := os.WriteFile(scriptPath, []byte(compiledScript), 0755); err != nil {
		return "", fmt.Errorf("failed to write script to %s: %w", scriptPath, err)
	}
	return scriptPath, nil
}

func runPluginCmd(svcMode string, owner string, binary string, args []string, logPath string, waitForCompletion bool, ns string) (int, error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create log file: %w", err)
	}

	var cmd *exec.Cmd
	if strings.HasSuffix(binary, ".sh") {
		if ns != "" {
			if svcMode == "exec" && owner != "root" {
				fullArgs := append([]string{"netns", "exec", ns, "setpriv", "--reuid=" + owner, "--regid=" + owner, "--init-groups", "/bin/sh", binary}, args...)
				cmd = exec.Command("ip", fullArgs...)
			} else {
				fullArgs := append([]string{"netns", "exec", ns, "/bin/sh", binary}, args...)
				cmd = exec.Command("ip", fullArgs...)
			}
		} else {
			if svcMode == "exec" && owner != "root" {
				fullArgs := append([]string{"-u", owner, "-E", "/bin/sh", binary}, args...)
				cmd = exec.Command("sudo", fullArgs...)
			} else {
				cmd = exec.Command("/bin/sh", append([]string{binary}, args...)...)
			}
		}
	} else {
		if ns != "" {
			if svcMode == "exec" && owner != "root" {
				fullArgs := append([]string{"netns", "exec", ns, "setpriv", "--reuid=" + owner, "--regid=" + owner, "--init-groups", binary}, args...)
				cmd = exec.Command("ip", fullArgs...)
			} else {
				fullArgs := append([]string{"netns", "exec", ns, binary}, args...)
				cmd = exec.Command("ip", fullArgs...)
			}
		} else {
			if svcMode == "exec" && owner != "root" {
				fullArgs := append([]string{"-u", owner, "-E", binary}, args...)
				cmd = exec.Command("sudo", fullArgs...)
			} else {
				cmd = exec.Command(binary, args...)
			}
		}
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if waitForCompletion {
		defer logFile.Close()
		if err := cmd.Run(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, err
	}
	logFile.Close()

	// Wait briefly to check if it crashed immediately
	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Wait()
	}()

	select {
	case err := <-errChan:
		return 0, fmt.Errorf("plugin exited immediately: %w (check logs at %s)", err, logPath)
	case <-time.After(500 * time.Millisecond):
		// Process is running
		return cmd.Process.Pid, nil
	}
}

func (sm *ServiceManager) buildTemplateCtx(svc config.Service, ns string) templateCtx {
	var macStr string
	if mac, err := getInterfaceMAC(svc.Interface, ns); err == nil {
		macStr = mac
	}

	var ipStr, ipNoMask string
	if ip, ipnet, err := getInterfaceIP(svc.Interface, ns); err == nil {
		ipStr = ipnet.String()
		ipNoMask = ip.String()
	}

	return templateCtx{
		Interface: svc.Interface,
		Namespace: ns,
		Mac:       macStr,
		IP:        ipStr,
		IPNoMask:  ipNoMask,
		PlanName:  sm.planName,
		StateDir:  sm.stateDir,
		Owner:     getOriginalUser(),
	}
}

func (sm *ServiceManager) startPlugin(idx int, svc config.Service, ns string) (int, error) {
	ctx := sm.buildTemplateCtx(svc, ns)
	logPath := filepath.Join(sm.stateDir, fmt.Sprintf("plugin_up_%d.log", idx))

	if svc.Script != "" {
		scriptPath, err := sm.compileAndWriteScript(svc.Script, fmt.Sprintf("plugin_up_%d.sh", idx), ctx)
		if err != nil {
			return 0, err
		}
		return runPluginCmd(svc.Mode, ctx.Owner, scriptPath, nil, logPath, false, ns)
	}

	compiledPath, err := compileTemplate(svc.Path, ctx)
	if err != nil {
		return 0, err
	}

	compiledArgs := make([]string, len(svc.Args))
	for i, arg := range svc.Args {
		cArg, err := compileTemplate(arg, ctx)
		if err != nil {
			return 0, err
		}
		compiledArgs[i] = cArg
	}

	return runPluginCmd(svc.Mode, ctx.Owner, compiledPath, compiledArgs, logPath, false, ns)
}

func (sm *ServiceManager) RunCleanup(idx int, svc config.Service, ns string) error {
	ctx := sm.buildTemplateCtx(svc, ns)
	logPath := filepath.Join(sm.stateDir, fmt.Sprintf("plugin_down_%d.log", idx))

	if svc.CleanupScript != "" {
		scriptPath, err := sm.compileAndWriteScript(svc.CleanupScript, fmt.Sprintf("plugin_down_%d.sh", idx), ctx)
		if err != nil {
			return err
		}
		_, err = runPluginCmd(svc.Mode, ctx.Owner, scriptPath, nil, logPath, true, ns)
		return err
	}

	if svc.CleanupPath != "" {
		compiledPath, err := compileTemplate(svc.CleanupPath, ctx)
		if err != nil {
			return err
		}

		compiledArgs := make([]string, len(svc.CleanupArgs))
		for i, arg := range svc.CleanupArgs {
			cArg, err := compileTemplate(arg, ctx)
			if err != nil {
				return err
			}
			compiledArgs[i] = cArg
		}

		_, err = runPluginCmd(svc.Mode, ctx.Owner, compiledPath, compiledArgs, logPath, true, ns)
		return err
	}

	return nil
}
