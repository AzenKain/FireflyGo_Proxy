//go:build darwin
// +build darwin

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type darwinProxyEndpoint struct {
	enabled bool
	server  string
	port    string
}

type darwinProxySettings struct {
	web    darwinProxyEndpoint
	secure darwinProxyEndpoint
}

var darwinProxyState = struct {
	sync.Mutex
	captured bool
	previous map[string]darwinProxySettings
	applied  map[string]struct{}
}{}

func enabledNetworkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").CombinedOutput()
	if err != nil {
		return nil, formatCommandError("list network services", err, out)
	}

	var services []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk") || strings.HasPrefix(line, "*") {
			continue
		}
		services = append(services, line)
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("no enabled network services found")
	}

	return services, nil
}

func proxyNetworkServices() ([]string, error) {
	service, defaultErr := defaultNetworkService()
	if defaultErr == nil && service != "" {
		return []string{service}, nil
	}

	services, activeErr := activeNetworkServices()
	if activeErr == nil && len(services) > 0 {
		return services, nil
	}

	if defaultErr != nil || activeErr != nil {
		return nil, errors.Join(defaultErr, activeErr)
	}
	return nil, fmt.Errorf("no active network services found")
}

func defaultNetworkService() (string, error) {
	device, err := defaultNetworkDevice()
	if err != nil {
		return "", err
	}

	servicesByDevice, err := networkServicesByDevice()
	if err != nil {
		return "", err
	}

	service := servicesByDevice[device]
	if service == "" {
		return "", fmt.Errorf("network service for default device %s not found", device)
	}
	return service, nil
}

func defaultNetworkDevice() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		return "", formatCommandError("get default network route", err, out)
	}

	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "interface" {
			device := strings.TrimSpace(value)
			if device != "" {
				return device, nil
			}
		}
	}

	return "", fmt.Errorf("default network interface not found")
}

func networkServicesByDevice() (map[string]string, error) {
	out, err := exec.Command("networksetup", "-listnetworkserviceorder").CombinedOutput()
	if err != nil {
		return nil, formatCommandError("list network service order", err, out)
	}

	services := make(map[string]string)
	currentService := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if service, ok := parseNetworkServiceOrderName(line); ok {
			currentService = service
			continue
		}
		if currentService == "" {
			continue
		}
		if device, ok := parseNetworkServiceOrderDevice(line); ok {
			services[device] = currentService
			currentService = ""
		}
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("no network service devices found")
	}
	return services, nil
}

func parseNetworkServiceOrderName(line string) (string, bool) {
	if !strings.HasPrefix(line, "(") {
		return "", false
	}

	end := strings.Index(line, ")")
	if end <= 1 {
		return "", false
	}

	if _, err := strconv.Atoi(line[1:end]); err != nil {
		return "", false
	}

	service := strings.TrimSpace(line[end+1:])
	return service, service != ""
}

func parseNetworkServiceOrderDevice(line string) (string, bool) {
	_, value, ok := strings.Cut(line, "Device:")
	if !ok {
		return "", false
	}

	value = strings.TrimSpace(value)
	if idx := strings.IndexAny(value, ",)"); idx != -1 {
		value = value[:idx]
	}

	device := strings.TrimSpace(value)
	return device, device != ""
}

func activeNetworkServices() ([]string, error) {
	services, err := enabledNetworkServices()
	if err != nil {
		return nil, err
	}

	active := make([]string, 0, len(services))
	for _, service := range services {
		ok, err := isNetworkServiceActive(service)
		if err != nil {
			continue
		}
		if ok {
			active = append(active, service)
		}
	}

	if len(active) == 0 {
		return nil, fmt.Errorf("no active network services found")
	}
	return active, nil
}

func isNetworkServiceActive(service string) (bool, error) {
	out, err := exec.Command("networksetup", "-getinfo", service).CombinedOutput()
	if err != nil {
		return false, formatCommandError("get network service info "+service, err, out)
	}

	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key != "IP address" && key != "IPv6 IP address" {
			continue
		}

		value = strings.TrimSpace(value)
		if value != "" && !strings.EqualFold(value, "none") {
			return true, nil
		}
	}

	return false, nil
}

func getProxySettings(service string) (darwinProxySettings, error) {
	web, webErr := getProxyEndpoint("-getwebproxy", service)
	secure, secureErr := getProxyEndpoint("-getsecurewebproxy", service)
	return darwinProxySettings{
		web:    web,
		secure: secure,
	}, errors.Join(webErr, secureErr)
}

func getProxyEndpoint(flag string, service string) (darwinProxyEndpoint, error) {
	out, err := exec.Command("networksetup", flag, service).CombinedOutput()
	if err != nil {
		return darwinProxyEndpoint{}, formatCommandError(flag+" "+service, err, out)
	}

	values := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}

	return darwinProxyEndpoint{
		enabled: values["Enabled"] == "Yes",
		server:  values["Server"],
		port:    values["Port"],
	}, nil
}

func setProxyForServices(services []string, host string, port string) error {
	var errs []error
	for _, service := range services {
		errs = append(errs,
			runNetworkSetup("-setwebproxy", service, host, port),
			runNetworkSetup("-setsecurewebproxy", service, host, port),
			runNetworkSetup("-setwebproxystate", service, "on"),
			runNetworkSetup("-setsecurewebproxystate", service, "on"),
		)
	}

	return errors.Join(errs...)
}

func restoreProxySettings(settings map[string]darwinProxySettings) error {
	var errs []error
	for service, setting := range settings {
		if setting.web.server != "" && setting.web.port != "" {
			errs = append(errs, runNetworkSetup("-setwebproxy", service, setting.web.server, setting.web.port))
		}
		errs = append(errs, runNetworkSetup("-setwebproxystate", service, proxyState(setting.web.enabled)))

		if setting.secure.server != "" && setting.secure.port != "" {
			errs = append(errs, runNetworkSetup("-setsecurewebproxy", service, setting.secure.server, setting.secure.port))
		}
		errs = append(errs, runNetworkSetup("-setsecurewebproxystate", service, proxyState(setting.secure.enabled)))
	}

	return errors.Join(errs...)
}

func disableProxyForServices(services []string) error {
	var errs []error
	for _, service := range services {
		errs = append(errs,
			runNetworkSetup("-setwebproxystate", service, "off"),
			runNetworkSetup("-setsecurewebproxystate", service, "off"),
		)
	}

	return errors.Join(errs...)
}

func proxyState(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func runNetworkSetup(args ...string) error {
	out, err := exec.Command("networksetup", args...).CombinedOutput()
	if err != nil {
		return formatCommandError("networksetup "+strings.Join(args, " "), err, out)
	}
	return nil
}

func formatCommandError(action string, err error, out []byte) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, msg)
}

func captureProxySettings(services []string) (map[string]darwinProxySettings, error) {
	settings := make(map[string]darwinProxySettings, len(services))
	var errs []error

	for _, service := range services {
		proxySettings, err := getProxySettings(service)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		settings[service] = proxySettings
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return settings, nil
}

func captureMissingProxySettings(services []string) error {
	missing := make([]string, 0, len(services))
	for _, service := range services {
		if _, ok := darwinProxyState.previous[service]; !ok {
			missing = append(missing, service)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	settings, err := captureProxySettings(missing)
	if err != nil {
		return err
	}
	for service, setting := range settings {
		darwinProxyState.previous[service] = setting
	}
	return nil
}

func serviceSet(services []string) map[string]struct{} {
	set := make(map[string]struct{}, len(services))
	for _, service := range services {
		set[service] = struct{}{}
	}
	return set
}

func appliedServicesNotIn(selected map[string]struct{}) []string {
	services := make([]string, 0, len(darwinProxyState.applied))
	for service := range darwinProxyState.applied {
		if _, ok := selected[service]; !ok {
			services = append(services, service)
		}
	}
	return services
}

func appliedServices() []string {
	services := make([]string, 0, len(darwinProxyState.applied))
	for service := range darwinProxyState.applied {
		services = append(services, service)
	}
	return services
}

func proxySettingsForServices(settings map[string]darwinProxySettings, services []string) map[string]darwinProxySettings {
	filtered := make(map[string]darwinProxySettings, len(services))
	for _, service := range services {
		if setting, ok := settings[service]; ok {
			filtered[service] = setting
		}
	}
	return filtered
}

func resetDarwinProxyState() {
	darwinProxyState.previous = nil
	darwinProxyState.applied = nil
	darwinProxyState.captured = false
}

func restoreAppliedProxySettings() error {
	if len(darwinProxyState.applied) == 0 {
		return nil
	}

	err := restoreProxySettings(proxySettingsForServices(
		darwinProxyState.previous,
		appliedServices(),
	))
	if err == nil {
		darwinProxyState.applied = nil
	}
	return err
}

func setProxy(enable bool, host string, port string) error {
	darwinProxyState.Lock()
	defer darwinProxyState.Unlock()

	if enable {
		if host == "" || port == "" {
			return fmt.Errorf("host and port are required to enable proxy")
		}

		services, err := proxyNetworkServices()
		if err != nil {
			return errors.Join(err, restoreAppliedProxySettings())
		}

		if darwinProxyState.previous == nil {
			darwinProxyState.previous = make(map[string]darwinProxySettings)
		}
		if err := captureMissingProxySettings(services); err != nil {
			return err
		}
		darwinProxyState.captured = true

		if err := setProxyForServices(services, host, port); err != nil {
			restoreErr := restoreProxySettings(darwinProxyState.previous)
			resetDarwinProxyState()
			return errors.Join(err, restoreErr)
		}

		selected := serviceSet(services)
		removed := appliedServicesNotIn(selected)
		restoreErr := restoreProxySettings(proxySettingsForServices(darwinProxyState.previous, removed))
		if restoreErr != nil {
			for _, service := range removed {
				selected[service] = struct{}{}
			}
		}
		darwinProxyState.applied = selected
		return restoreErr
	}

	if darwinProxyState.captured {
		err := restoreProxySettings(darwinProxyState.previous)
		if err == nil {
			resetDarwinProxyState()
		}
		return err
	}

	services, err := enabledNetworkServices()
	if err != nil {
		return err
	}
	return disableProxyForServices(services)
}
