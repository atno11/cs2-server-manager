package manager

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func preflightLinux() error         { return checkLinuxCapabilities() }
func checkLinuxCapabilities() error { return checkLinuxCapabilitiesWithProbe("/bin/sleep") }
func checkLinuxCapabilitiesWithProbe(probe string) error {
	if _, err := os.ReadFile("/proc/self/stat"); err != nil {
		return fmt.Errorf("readable /proc required: %w", err)
	}
	if _, err := os.ReadDir("/proc"); err != nil {
		return fmt.Errorf("readable /proc required: %w", err)
	}
	fd := -1
	cmd := exec.Command(probe, "5")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, PidFD: &fd}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("isolated pidfd probe failed: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if fd >= 0 {
			_ = syscall.Close(fd)
		}
	}()
	if fd < 0 {
		return errors.New("pidfd unavailable")
	}
	_, _, probeErr := syscall.Syscall6(sysPidfdSendSignal, uintptr(fd), 0, 0, pidfdSignalProcessGroup, 0, 0)
	if probeErr != 0 {
		return fmt.Errorf("pidfd process-group signal unsupported: %w", probeErr)
	}
	if _, err := groupHasLiveMembers(cmd.Process.Pid, cmd.Process.Pid); err != nil {
		return fmt.Errorf("/proc group inspection unavailable: %w", err)
	}
	if err := pidfdSignalGroup(fd, syscall.SIGKILL); err != nil {
		return fmt.Errorf("pidfd group cleanup failed: %w", err)
	}
	return nil
}
