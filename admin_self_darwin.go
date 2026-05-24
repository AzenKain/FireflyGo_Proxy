//go:build darwin
// +build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func relaunchWithAdminIfNeeded() (bool, error) {
	if os.Geteuid() == 0 {
		return false, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("get executable path: %w", err)
	}

	workDir, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("get working directory: %w", err)
	}

	wrapperPID := os.Getpid()
	launcherPID := os.Getppid()

	args := make([]string, 0, len(os.Args))
	args = append(args, shellQuote(exePath))
	for _, arg := range os.Args[1:] {
		args = append(args, shellQuote(arg))
	}
	if !hasFlagArg("parent-pid") {
		args = append(args, shellQuote("-parent-pid"), shellQuote(strconv.Itoa(wrapperPID)))
	}

	command := fmt.Sprintf(
		"cd %s && %s > /dev/null 2>&1 &",
		shellQuote(workDir),
		strings.Join(args, " "),
	)
	script := fmt.Sprintf("do shell script %s with administrator privileges", appleScriptString(command))

	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		return false, formatCommandError("relaunch proxy as admin", err, out)
	}

	waitForRelaunchedProxyShutdown(launcherPID)
	return true, nil
}

func waitForRelaunchedProxyShutdown(launcherPID int) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(stop)

	select {
	case <-stop:
	case <-parentProcessDone(launcherPID):
	}
}

func hasFlagArg(name string) bool {
	for _, arg := range os.Args[1:] {
		trimmed := strings.TrimLeft(arg, "-")
		if trimmed == name || strings.HasPrefix(trimmed, name+"=") {
			return true
		}
	}
	return false
}
