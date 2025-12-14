package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

const pidFileName = "daemon.pid"

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

func IsRunning() (bool, error) {
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

	// Try to send signal 0 to verify if the process is alive
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process is not alive, remove PID file
		os.Remove(pidPath)
		return false, nil
	}

	return true, nil
}

func Start(daemonBinary string) error {
	running, err := IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check if daemon is running: %w", err)
	}
	if running {
		return fmt.Errorf("daemon is already running")
	}

	// Execute the daemon in background
	cmd := exec.Command(daemonBinary)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	pidPath, err := getPIDFilePath()
	if err != nil {
		cmd.Process.Kill()
		return err
	}

	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	time.Sleep(2 * time.Second)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("daemon process died immediately after start")
	}

	return nil
}

func Stop() error {
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
		os.Remove(pidPath)
		return fmt.Errorf("daemon process not found")
	}

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
			os.Remove(pidPath)
			return nil
		}
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("daemon did not stop in time and SIGKILL failed: %w", err)
	}

	time.Sleep(1 * time.Second)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		os.Remove(pidPath)
		return nil
	}

	os.Remove(pidPath)
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

