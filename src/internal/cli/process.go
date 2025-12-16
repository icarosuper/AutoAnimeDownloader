package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const pidFileName = "daemon.pid"
const systemdServiceName = "autoanimedownloader.service"
const windowsServiceName = "AutoAnimeDownloader"

func getPIDFilePath() (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	pidDir := filepath.Join(baseFolder, ".autoAnimeDownloader")
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create PID directory: %w", err)
	}

	return filepath.Join(pidDir, pidFileName), nil
}

// isSystemdAvailable checks if systemd is available and the service is installed
func isSystemdAvailable() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	// Check if systemctl is available
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}

	// Check if the service is installed (user service)
	cmd := exec.Command("systemctl", "--user", "list-unit-files", "--type=service", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Check if our service is in the list
	return strings.Contains(string(output), systemdServiceName)
}

// isWindowsServiceAvailable checks if the Windows service is installed
func isWindowsServiceAvailable() bool {
	if runtime.GOOS != "windows" {
		return false
	}

	// Use sc.exe to query the service
	cmd := exec.Command("sc", "query", windowsServiceName)
	err := cmd.Run()
	// Exit code 0 means service exists, non-zero means it doesn't
	return err == nil
}

// startWithSystemd starts the daemon using systemctl
func startWithSystemd() error {
	cmd := exec.Command("systemctl", "--user", "start", systemdServiceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start daemon with systemctl: %w", err)
	}

	// Wait a bit for the service to start
	time.Sleep(1 * time.Second)

	// Verify the service is running
	cmd = exec.Command("systemctl", "--user", "is-active", "--quiet", systemdServiceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("daemon service failed to start")
	}

	return nil
}

// stopWithSystemd stops the daemon using systemctl
func stopWithSystemd() error {
	cmd := exec.Command("systemctl", "--user", "stop", systemdServiceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop daemon with systemctl: %w", err)
	}

	// Wait a bit for the service to stop
	time.Sleep(500 * time.Millisecond)

	// Verify the service is stopped
	cmd = exec.Command("systemctl", "--user", "is-active", "--quiet", systemdServiceName)
	if err := cmd.Run(); err == nil {
		// If is-active returns success, the service is still active
		return fmt.Errorf("daemon service is still running")
	}

	return nil
}

// isRunningWithSystemd checks if the daemon is running using systemctl
func isRunningWithSystemd() (bool, error) {
	cmd := exec.Command("systemctl", "--user", "is-active", "--quiet", systemdServiceName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	// Check exit code
	if exitError, ok := err.(*exec.ExitError); ok {
		// Exit code 0 means active, non-zero means inactive or failed
		return exitError.ExitCode() == 0, nil
	}

	return false, err
}

// startWithWindowsService starts the daemon using Windows Service Manager
func startWithWindowsService() error {
	cmd := exec.Command("net", "start", windowsServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if service is already running
		if strings.Contains(string(output), "already been started") {
			return nil
		}
		return fmt.Errorf("failed to start Windows service: %s: %w", string(output), err)
	}

	// Wait a bit for the service to start
	time.Sleep(1 * time.Second)

	// Verify the service is running
	running, err := isRunningWithWindowsService()
	if err != nil {
		return fmt.Errorf("failed to verify service status: %w", err)
	}
	if !running {
		return fmt.Errorf("daemon service failed to start")
	}

	return nil
}

// stopWithWindowsService stops the daemon using Windows Service Manager
func stopWithWindowsService() error {
	cmd := exec.Command("net", "stop", windowsServiceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if service is already stopped
		if strings.Contains(string(output), "not started") || strings.Contains(string(output), "not running") {
			return nil
		}
		return fmt.Errorf("failed to stop Windows service: %s: %w", string(output), err)
	}

	// Wait a bit for the service to stop
	time.Sleep(1 * time.Second)

	// Verify the service is stopped
	running, err := isRunningWithWindowsService()
	if err != nil {
		return err
	}
	if running {
		return fmt.Errorf("daemon service is still running")
	}

	return nil
}

// isRunningWithWindowsService checks if the daemon is running using Windows Service Manager
func isRunningWithWindowsService() (bool, error) {
	cmd := exec.Command("sc", "query", windowsServiceName)
	output, err := cmd.Output()
	if err != nil {
		// Service doesn't exist
		return false, nil
	}

	// Check if service is in RUNNING state
	outputStr := string(output)
	return strings.Contains(outputStr, "RUNNING"), nil
}

// isRunningWithPID checks if the daemon is running using PID file
func isRunningWithPID() (bool, error) {
	pidPath, err := getPIDFilePath()
	if err != nil {
		return false, err
	}

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		return false, fmt.Errorf("invalid PID in file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}

	// Windows-specific check
	if runtime.GOOS == "windows" {
		// On Windows, Signal(0) may not work reliably
		// Try to release and check if process still exists
		if err := process.Release(); err == nil {
			// Try to find the process again - if it exists, it's running
			_, err := os.FindProcess(pid)
			return err == nil, nil
		}
		return false, nil
	}

	// Linux/Unix: use signal 0
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process is not alive
		return false, nil
	}

	return true, nil
}

func IsRunning() (bool, error) {
	// Try systemd first if available (Linux)
	if isSystemdAvailable() {
		return isRunningWithSystemd()
	}

	// Try Windows Service if available
	if isWindowsServiceAvailable() {
		return isRunningWithWindowsService()
	}

	// Fallback to PID file method
	return isRunningWithPID()
}

func Start(daemonBinary string) error {
	running, err := IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check if daemon is running: %w", err)
	}
	if running {
		return fmt.Errorf("daemon is already running")
	}

	// Use systemd if available (Linux)
	if isSystemdAvailable() {
		return startWithSystemd()
	}

	// Use Windows Service if available
	if isWindowsServiceAvailable() {
		return startWithWindowsService()
	}

	// Fallback to direct process start
	// Execute the daemon in background
	cmd := exec.Command(daemonBinary)

	// Configure process attributes for proper daemon separation
	// Setpgid creates a new process group, which is sufficient to separate
	// the child process from the parent. Setsid is not needed and can cause
	// "operation not permitted" errors in some contexts.
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = getSysProcAttr()
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Release the reference to the child process immediately
	// This ensures the child process is completely separated from the parent
	cmd.Process.Release()

	// Wait a bit for daemon to create its own PID file
	time.Sleep(2 * time.Second)

	// Verify the process is alive using the PID from the file
	// This is more reliable than using cmd.Process after Release()
	pidPath, err := getPIDFilePath()
	if err != nil {
		return fmt.Errorf("failed to get PID file path: %w", err)
	}

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon PID file was not created")
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find daemon process: %w", err)
	}

	// Verify the process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("daemon process died immediately after start")
	}

	return nil
}

func Stop() error {
	// Use systemd if available (Linux)
	if isSystemdAvailable() {
		return stopWithSystemd()
	}

	// Use Windows Service if available
	if isWindowsServiceAvailable() {
		return stopWithWindowsService()
	}

	// Fallback to PID file method
	pidPath, err := getPIDFilePath()
	if err != nil {
		return err
	}

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("daemon is not running (no PID file found)")
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("daemon process not found")
	}

	// Windows-specific handling
	if runtime.GOOS == "windows" {
		// On Windows, use Kill() directly instead of signals
		if err := process.Kill(); err != nil {
			return fmt.Errorf("failed to kill daemon process: %w", err)
		}

		// Wait for process to terminate
		time.Sleep(1 * time.Second)

		// Verify process is terminated
		if err := process.Release(); err == nil {
			// Try to get process info - if this fails, process is gone
			_, err := os.FindProcess(pid)
			if err == nil {
				// Process might still be running, wait a bit more
				time.Sleep(2 * time.Second)
			}
		}

		return nil
	}

	// Linux/Unix: use signals
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to daemon: %w", err)
	}

	// Wait for process to terminate (polling)
	maxWait := 10 * time.Second
	checkInterval := 500 * time.Millisecond
	elapsed := time.Duration(0)

	for elapsed < maxWait {
		time.Sleep(checkInterval)
		elapsed += checkInterval

		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process terminated, daemon will clean up PID file
			return nil
		}
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("daemon did not stop in time and SIGKILL failed: %w", err)
	}

	time.Sleep(1 * time.Second)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process terminated, daemon will clean up PID file
		return nil
	}

	return fmt.Errorf("daemon did not stop even after SIGKILL")
}

func GetPID() (int, error) {
	pidPath, err := getPIDFilePath()
	if err != nil {
		return 0, err
	}

	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("daemon is not running")
		}
		return 0, err
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	return pid, nil
}
