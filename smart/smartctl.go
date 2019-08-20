// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package smart

import (
	"bufio"
	"bytes"
	"errors"
	"os/exec"
	"regexp"
	"strings"

	"github.com/blang/semver"
)

const (
	// smartMonMinVersion is the min version of smartmon supported by this library
	smartMonMinVersion = "6.6.0"
	// smartMonMinVersionJSON is the min version of smartmon capable of outputting JSON
	smartMonMinVersionJSON = "7.0.0"

	smartctlCmd       = "smartctl"
	smartMetricPrefix = "smartmon_"
)

var (
	smartctlVersionOpts = []string{"-V"}
	smartctlScanOpts    = []string{"--scan"}
	// smartctlDeviceActiveOpts uses the -n option to avoid waking a device in sleep or standby
	smartctlDeviceActiveOpts = []string{"-n", "standby"}
	// smartctlDeviceInfoOpts gets the info and health of the device
	smartctlDeviceInfoOpts = []string{"-i", "-H"}
	// smartctlDeviceMetricOpts
	smartctlDeviceMetricOpts = []string{"-A"}
	smartctlJSONOption       = "-j"

	smartctlDeviceRegex = regexp.MustCompile("^(/.+) -d ([\\w]+) # (.+), (.+)")
	smartctlInfoRegex   = regexp.MustCompile("^([^:]+): (.+)$")
)

// Device represents a SMART capable device
type Device struct {
	Name     string
	InfoName string
	Type     string
	Protocol string
}

// DeviceStatus contains the status reported by the -H option
type DeviceStatus struct {
	Passed  bool
	Details map[string]string
}

// DeviceInfo contains info reported by the -i option
// "model_name": "SAMSUNG MZVLB512HAJQ-000L7",
// "serial_number": "S3TNNX1K710265",
// "firmware_version": "4L2QEXA7",
// "nvme_pci_vendor": {
//   "id": 5197,
//   "subsystem_id": 5197
// },
// "nvme_ieee_oui_identifier": 9528,
// "nvme_total_capacity": 512110190592,
// "nvme_unallocated_capacity": 0,
// "nvme_controller_id": 4,
// "nvme_number_of_namespaces": 1,
// "nvme_namespaces": [
//   {
// 	"id": 1,
// 	"size": {
// 	  "blocks": 1000215216,
// 	  "bytes": 512110190592
// 	},
// 	"capacity": {
// 	  "blocks": 1000215216,
// 	  "bytes": 512110190592
// 	},
// 	"utilization": {
// 	  "blocks": 251500984,
// 	  "bytes": 128768503808
// 	},
// 	"formatted_lba_size": 512,
// 	"eui64": {
// 	  "oui": 9528,
// 	  "ext_id": 581996836738
// 	}
//   }
// ],
// "user_capacity": {
//   "blocks": 1000215216,
//   "bytes": 512110190592
// },
// "logical_block_size": 512,
// "local_time": {
//   "time_t": 1566314980,
//   "asctime": "Tue Aug 20 10:29:40 2019 CDT"
// }
type DeviceInfo struct {
	Available  bool
	Enabled    bool
	Healthy    bool
	Attributes map[string]string
}

func smartCtrlAvailable() bool {
	_, err := exec.LookPath("smartctl")
	return err != nil
}

// smartCtl runs the smartctl command with the given options and returns the combined output
func smartCtl(opts ...string) ([]byte, error) {
	smartctlCmd := exec.Command(smartctlCmd, opts...)
	output, err := smartctlCmd.CombinedOutput()
	if err != nil {
		return nil, errors.New("Failed to execute command: " + err.Error())
	}
	return output, nil
}

// Version gets the current version of the smartmon tools, returns an error
// if smartmon tools cannot be found.
func Version() (string, error) {
	output, err := smartCtl(smartctlVersionOpts...)
	if err != nil {
		return "", err
	}
	return strings.Fields(firstLine(output))[1], nil
}

// scanDevices gets the list of available smart devices as
// reported by 'smartctl --scan'
func scanDevices() ([]Device, error) {
	output, err := smartCtl(smartctlScanOpts...)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	devices := []Device{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		matches := smartctlDeviceRegex.FindSubmatch([]byte(line))
		if len(matches) < 4 {
			return nil, errors.New("Unable to parse device line: " + line)
		}
		device := Device{
			Name:     string(matches[1]),
			Type:     string(matches[2]),
			InfoName: string(matches[3]),
			Protocol: string(matches[4]),
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// CheckSupportedVersion verifies that the smartctl command is available and
// compares the current version reported by smartctl to
// the minimum version supported by the library.  Returns an error if the smartctl
// command cannot be found, or if the version is lower than the minimum
func CheckSupportedVersion() error {
	minVer := semver.MustParse(smartMonMinVersion)
	foundVer, err := Version()
	if err != nil {
		return errors.New("Unable to determine installed smartctl version:" + err.Error())
	}
	installedVer, err := semver.Parse(foundVer)
	if err != nil {
		return errors.New("Unable to parse installed smartctl version:" + err.Error())
	}
	if installedVer.LT(minVer) {
		return errors.New("Installed smartctl version " + installedVer.String() + " is lower than the required minimum " + minVer.String())
	}
	return nil
}

// firstLine reads the first line from a string
func firstLine(text []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(text))
	if !scanner.Scan() {
		panic("Unable to read first line")
	}
	return scanner.Text()
}

// active returns true if the device is in an active state
// i.e. not in sleep or standby
func (d *Device) active() (bool, error) {
	opts := append(smartctlDeviceActiveOpts, "-d", d.Type, d.Name)
	_, err := smartCtl(opts...)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *Device) info() (*DeviceInfo, error) {
	opts := append(smartctlDeviceInfoOpts, "-d", d.Type, d.Name)
	output, err := smartCtl(opts...)
	if err != nil {
		return nil, err
	}

	//smartAvailable, smartEnabled, smartHealthy := 0.0, 0.0, 0.0
	info := DeviceInfo{
		Attributes: map[string]string{},
	}
	for _, line := range strings.Split(string(output), "\n") {
		matches := smartctlInfoRegex.FindStringSubmatch(line)
		if matches != nil && len(matches) > 2 {
			name, val := matches[1], matches[2]
			info.Attributes[sanitizeLabelName(name)] = strings.TrimSpace(val)
			if strings.HasPrefix(name, "SMART support is") {
				switch {
				case strings.HasPrefix(val, "Available"):
					info.Available = true
				case strings.HasPrefix(val, "Enabled"):
					info.Enabled = true
				}
			} else if strings.HasPrefix(name, "SMART Health Status") {
				if strings.HasPrefix(val, "OK") {
					info.Healthy = true
					info.Available = true
					info.Enabled = true
				}
			} else if strings.HasPrefix(name, "SMART overall-health self-assessment test result") {
				if strings.HasPrefix(val, "PASSED") {
					info.Healthy = true
					info.Available = true
					info.Enabled = true
				}
			}
		}
	}
	return &info, nil
}

// sanitizedLabelName formats a string to be an acceptable label name
func sanitizeLabelName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return strings.ToLower(name)
}
