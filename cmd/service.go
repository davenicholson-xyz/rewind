/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// serviceCmd represents the service command
var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Install and manage rewind as a system service",
	Long: `Install rewind as a system service that automatically runs the watch command.
This command will install the appropriate service file for your operating system:
- Linux: systemd user service in ~/.config/systemd/user/
- macOS: launchd plist in ~/Library/LaunchAgents/`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("Usage: rewind service <install|uninstall|start|stop|status>")
			return
		}

		action := args[0]
		switch action {
		case "install":
			if err := installService(); err != nil {
				fmt.Printf("Error installing service: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Service installed successfully")
		case "uninstall":
			if err := uninstallService(); err != nil {
				fmt.Printf("Error uninstalling service: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Service uninstalled successfully")
		case "start":
			if err := startService(); err != nil {
				fmt.Printf("Error starting service: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Service started successfully")
		case "stop":
			if err := stopService(); err != nil {
				fmt.Printf("Error stopping service: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Service stopped successfully")
		case "status":
			if err := statusService(); err != nil {
				fmt.Printf("Error checking service status: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Println("Usage: rewind service <install|uninstall|start|stop|status>")
		}
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)
}

func getExecutablePath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(executable)
}

func killRewindWatchProcesses() error {
	// Find and kill any rewind watch processes
	cmd := exec.Command("pkill", "-f", "rewind watch")
	err := cmd.Run()
	
	// pkill returns exit code 1 if no processes were found, which is not an error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("failed to kill rewind watch processes: %v", err)
	}
	
	return nil
}

func installService() error {
	execPath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	switch runtime.GOOS {
	case "linux":
		return installSystemdService(execPath)
	case "darwin":
		return installLaunchdService(execPath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func uninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallSystemdService()
	case "darwin":
		return uninstallLaunchdService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func startService() error {
	switch runtime.GOOS {
	case "linux":
		return startSystemdService()
	case "darwin":
		return startLaunchdService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func stopService() error {
	switch runtime.GOOS {
	case "linux":
		return stopSystemdService()
	case "darwin":
		return stopLaunchdService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func statusService() error {
	switch runtime.GOOS {
	case "linux":
		return statusSystemdService()
	case "darwin":
		return statusLaunchdService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// Linux systemd functions
func installSystemdService(execPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	serviceDir := filepath.Join(homeDir, ".config", "systemd", "user")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %v", err)
	}

	serviceFile := filepath.Join(serviceDir, "rewind.service")
	
	// Check if service is currently running
	wasRunning := false
	if err := exec.Command("systemctl", "--user", "is-active", "rewind.service").Run(); err == nil {
		wasRunning = true
		// Stop the service before updating
		exec.Command("systemctl", "--user", "stop", "rewind.service").Run()
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=Rewind file watcher service
After=graphical-session.target

[Service]
Type=simple
ExecStart=%s watch
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, execPath)

	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %v", err)
	}

	// Reload systemd and enable the service
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %v", err)
	}

	if err := exec.Command("systemctl", "--user", "enable", "rewind.service").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}

	// Restart the service if it was running before
	if wasRunning {
		if err := exec.Command("systemctl", "--user", "start", "rewind.service").Run(); err != nil {
			return fmt.Errorf("failed to restart service: %v", err)
		}
	}

	return nil
}

func uninstallSystemdService() error {
	// Stop and disable the service
	exec.Command("systemctl", "--user", "stop", "rewind.service").Run()
	exec.Command("systemctl", "--user", "disable", "rewind.service").Run()

	// Remove service file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	serviceFile := filepath.Join(homeDir, ".config", "systemd", "user", "rewind.service")
	if err := os.Remove(serviceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %v", err)
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

func startSystemdService() error {
	return exec.Command("systemctl", "--user", "start", "rewind.service").Run()
}

func stopSystemdService() error {
	// Stop the systemd service first
	if err := exec.Command("systemctl", "--user", "stop", "rewind.service").Run(); err != nil {
		return err
	}
	
	// Kill any remaining rewind watch processes
	return killRewindWatchProcesses()
}

func statusSystemdService() error {
	cmd := exec.Command("systemctl", "--user", "status", "rewind.service")
	output, err := cmd.Output()
	fmt.Print(string(output))
	return err
}

// macOS launchd functions
func installLaunchdService(execPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	launchDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchDir, 0755); err != nil {
		return fmt.Errorf("failed to create launch directory: %v", err)
	}

	plistFile := filepath.Join(launchDir, "com.rewind.watcher.plist")
	
	// Check if service is already loaded and unload it first
	if err := exec.Command("launchctl", "list", "com.rewind.watcher").Run(); err == nil {
		// Unload the existing service
		exec.Command("launchctl", "unload", plistFile).Run()
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.rewind.watcher</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>watch</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>%s/Library/Logs/rewind.log</string>
	<key>StandardErrorPath</key>
	<string>%s/Library/Logs/rewind.log</string>
</dict>
</plist>
`, execPath, homeDir, homeDir)

	if err := os.WriteFile(plistFile, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist file: %v", err)
	}

	// Load the service (this will start it automatically due to RunAtLoad)
	if err := exec.Command("launchctl", "load", plistFile).Run(); err != nil {
		return fmt.Errorf("failed to load service: %v", err)
	}

	return nil
}

func uninstallLaunchdService() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}

	plistFile := filepath.Join(homeDir, "Library", "LaunchAgents", "com.rewind.watcher.plist")

	// Unload the service
	exec.Command("launchctl", "unload", plistFile).Run()

	// Remove plist file
	if err := os.Remove(plistFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %v", err)
	}

	return nil
}

func startLaunchdService() error {
	return exec.Command("launchctl", "start", "com.rewind.watcher").Run()
}

func stopLaunchdService() error {
	// Stop the launchd service first
	if err := exec.Command("launchctl", "stop", "com.rewind.watcher").Run(); err != nil {
		return err
	}
	
	// Kill any remaining rewind watch processes
	return killRewindWatchProcesses()
}

func statusLaunchdService() error {
	cmd := exec.Command("launchctl", "list", "com.rewind.watcher")
	output, err := cmd.Output()
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			fmt.Println("Service is not loaded")
			return nil
		}
		return err
	}
	fmt.Print(string(output))
	return nil
}
