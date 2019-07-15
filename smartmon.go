package main

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	smartctlCmd       = "smartctl"
	smartMetricPrefix = "smartmon_"
)

var (
	smartctlVersionOpts      = []string{"-V"}
	smartctlDeviceListOpts   = []string{"--scan-open"}
	smartctlDeviceActiveOpts = []string{"-n", "standby", "-d"}
	smartctlDeviceInfoOpts   = []string{"-i", "-H", "-d"}
	smartctlDeviceMetricOpts = []string{"-A", "-d"}

	smartctlInfoRegex = regexp.MustCompile("^([^:]+): (.+)$")
)

func smartCtrlAvailable() error {
	_, err := exec.LookPath("smartctl")
	return err
}

// smartCtrlCheckVersion compares the current version reported by smartctl to
// the given min version.  Returns an error if either version cannot be parsed to
// a valid float, or if the current version is lower than the min version.
func smartCtrlCheckVersion(minVerString string) error {
	minVer, err := strconv.ParseFloat(minVerString, 64)
	if err != nil {
		return errors.New("Unable to parse installed smartctl version:" + err.Error())
	}
	installedVerString := smartMonVersion()
	installedVer, err := strconv.ParseFloat(installedVerString, 64)
	if err != nil {
		return errors.New("Unable to parse installed smartctl version:" + err.Error())
	}
	if installedVer < minVer {
		return errors.New("Installed smartctl version " + installedVerString + " is lower than the required minimum " + minVerString)
	}
	return nil
}

func smartMonVersionMetric() *Metric {
	return &Metric{
		Desc: &MetricDesc{
			Name: "smartmon_version",
			Help: "version reported by smartctl -V",
			Type: "guage",
		},
		Value: "1",
		Labels: map[string]string{"version": smartMonVersion(),
			"foo": "bar"},
	}
}

func runSmartCmd(opts ...string) string {
	smartctlCmd := exec.Command(smartctlCmd, opts...)
	output, err := smartctlCmd.CombinedOutput()
	if err != nil {
		panic("Failed to execute command: " + err.Error())
	}
	return string(output)
}

// smartMonVersion gets the version number reported by 'smartclt -V'
func smartMonVersion() string {
	output := runSmartCmd(smartctlVersionOpts...)
	return strings.Fields(firstLine(output))[1]
}

func smartMonDeviceList() map[string]string {
	output := runSmartCmd(smartctlDeviceListOpts...)
	lines := strings.Split(output, "\n")
	devices := map[string]string{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(fields[0], "/dev") {
			devices[fields[0]] = fields[2]
		}
	}
	return devices
}

// smartMonRunMetric contains the current unix time
// smartctl_run{disk=\"${disk}\",type=\"${type}\"}" "$(TZ=UTC date '+%s')"
func smartMonRunMetric(dev string, devType string) *Metric {
	unixTime := strconv.FormatInt(time.Now().Unix(), 10)
	return &Metric{
		Desc: &MetricDesc{
			Name: "smartctl_run",
			Type: "guage",
			Help: "contains current unix time",
		},
		Labels: map[string]string{
			"disk": dev,
			"type": devType,
		},
		Value: unixTime,
	}
}

func smartMonDeviceActive(dev string, devType string) *Metric {
	opts := append(smartctlDeviceActiveOpts, devType, dev)
	smartctlCmd := exec.Command(smartctlCmd, opts...)
	_, err := smartctlCmd.CombinedOutput()
	active := "1"
	if err != nil {
		active = "0"
	}
	return &Metric{
		Desc: &MetricDesc{
			Name: "smartctl_run",
			Type: "guage",
			Help: "shows result of smartctl -n standby",
		},
		Labels: map[string]string{
			"disk": dev,
			"type": devType,
		},
		Value: active,
	}
}

// smartMonInfoMetrics provides metrics based on output of
// 'smartctl -i -H -d <type> <dev>'
func smartMonInfoMetrics(dev string, devType string) []*Metric {
	opts := append(smartctlDeviceInfoOpts, devType, dev)
	output := runSmartCmd(opts...)

	smartAvailable, smartEnabled, smartHealthy := "0", "0", "0"
	smartInfo := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		matches := smartctlInfoRegex.FindStringSubmatch(line)
		if matches != nil && len(matches) > 2 {
			name, val := matches[1], matches[2]
			smartInfo[name] = strings.TrimSpace(val)
			if strings.HasPrefix(name, "SMART support is") {
				switch {
				case strings.HasPrefix(val, "Available"):
					smartAvailable = "1"
				case strings.HasPrefix(val, "Enabled"):
					smartEnabled = "1"
				}
			} else if strings.HasPrefix(name, "SMART Health Status") {
				if strings.HasPrefix(val, "OK") {
					smartHealthy = "1"
				}
			} else if strings.HasPrefix(name, "SMART overall-health self-assessment test result") {
				if strings.HasPrefix(val, "Passed") {
					smartHealthy = "1"
				}
			}

		}
	}
	commonLabels := map[string]string{
		"disk": dev,
		"type": devType,
	}
	infoLabels := smartMonInfoLabels(commonLabels, smartInfo)
	infoMetrics := []*Metric{}
	infoMetrics = append(infoMetrics, NewMetric("smartmon_device_info", infoLabels, "1"))
	infoMetrics = append(infoMetrics, NewMetric("smartmon_device_smart_available", commonLabels, smartAvailable))
	infoMetrics = append(infoMetrics, NewMetric("smartmon_device_smart_enabled", commonLabels, smartEnabled))
	infoMetrics = append(infoMetrics, NewMetric("smartmon_device_smart_healthy", commonLabels, smartHealthy))
	return infoMetrics
}

func smartMonInfoLabels(labels map[string]string, smartInfo map[string]string) map[string]string {
	infoLabels := map[string]string{
		"vendor":           smartInfo["Vendor"],
		"product":          smartInfo["Product"],
		"revision":         smartInfo["Revision"],
		"lun_id":           smartInfo["Logical Unit id"],
		"model_family":     smartInfo["Model Family"],
		"device_model":     smartInfo["Device Model"],
		"serial_number":    smartInfo["Serial Number"],
		"firmware_version": smartInfo["Firmware Version"],
	}
	for key, val := range labels {
		infoLabels[key] = val
	}
	return infoLabels
}

// smartMonDeviceAttributes gathers metrics for smartmon devices using the smartctl
// command 'smartctl -A -d <type> <device>'
func smartMonDeviceAttributes(device string, devType string) []*Metric {
	if strings.HasPrefix(devType, "sat") {
		return smartMonDeviceSatAttributes(device, devType)
	} // TODO: add support for scsi and megaraid devices
	return []*Metric{
		NewMetric("smartmon_info",
			map[string]string{"disk": device},
			"type is not sat, scsi or megaraid but "+devType),
	}
}

func smartMonDeviceSatAttributes(device string, devType string) []*Metric {
	opts := append(smartctlDeviceMetricOpts, devType, device)
	output := runSmartCmd(opts...)

	commonLabels := map[string]string{
		"disk": device,
		"type": devType,
	}

	attrMetrics := []*Metric{}
	for _, line := range strings.Split(output[1:], "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		labels := map[string]string{}
		for name, val := range commonLabels {
			labels[name] = val
		}
		labels["smart_id"] = fields[0]
		metricName := "smartmon_" + strings.ToLower(fields[1]) + "_value"
		attrMetrics = append(attrMetrics, NewMetric(metricName, labels, fields[3]))
		metricName = "smartmon_" + strings.ToLower(fields[1]) + "_worst"
		attrMetrics = append(attrMetrics, NewMetric(metricName, labels, fields[4]))
		metricName = "smartmon_" + strings.ToLower(fields[1]) + "_threshold"
		attrMetrics = append(attrMetrics, NewMetric(metricName, labels, fields[5]))
		metricName = "smartmon_" + strings.ToLower(fields[1]) + "_raw_value"
		attrMetrics = append(attrMetrics, NewMetric(metricName, labels, fields[9]))
	}
	return attrMetrics
}
