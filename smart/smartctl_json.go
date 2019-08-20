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
	"encoding/json"
	"errors"
	"strings"

	"github.com/blang/semver"
)

// JSONCapable returns true if the current installed version of smartmon tools is capable of outputting JSON
func JSONCapable() bool {
	minVer := semver.MustParse(smartMonMinVersionJSON)
	foundVer, err := Version()
	if err != nil {
		return false
	}
	installedVer, err := semver.ParseTolerant(foundVer)
	if err != nil {
		return false
	}
	if installedVer.LT(minVer) {
		return false
	}
	return true
}

// SmartctlJSONMeta contains metadata included with the JSON output
// of the smartctl command
//   "smartctl": {
//     "version": [
//       7,
//       0
//     ],
//     "svn_revision": "4903",
//     "platform_info": "x86_64-linux-5.2.7-200.fc30.x86_64",
//     "build_info": "(local build)",
//     "argv": [
//       "smartctl",
//       "--scan",
//       "-j"
//     ],
//     "exit_status": 0
//   },
type SmartctlJSONMeta struct {
	Smartctl struct {
		Version []int
	}
	SvnRevision  string
	PlatformInfo string
	BuildInfo    string
	Argv         []string
	ExitStatus   int
}

func useJSON(opts []string) []string {
	return append([]string{smartctlJSONOption}, opts...)
}

type parsedJSON map[string]*json.RawMessage

func parseJSON(data []byte) (parsedJSON, error) {
	parsed := parsedJSON{}
	err := json.Unmarshal(data, &parsed)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

// scanDevicesJSON is similar to deviceList but uses JSON
// output of the smartctl command
func scanDevicesJSON() ([]Device, error) {
	output, err := smartCtl(useJSON(smartctlScanOpts)...)
	if err != nil {
		return nil, err
	}
	mappedJSON, err := parseJSON(output)
	if err != nil {
		return nil, err
	}
	devices := []Device{}

	unparsedDevices, exists := mappedJSON["devices"]
	if !exists {
		return nil, errors.New("unable to find 'devices' entry in JSON output")
	}
	err = json.Unmarshal(*unparsedDevices, &devices)
	if err != nil {
		return nil, err
	}
	return devices, nil
}

var (
	parsableFields = map[string]struct{}{
		"json_format_version": {},
		"smartctl":            {},
		"device":              {},
		"smart_status":        {},
	}
)

// attributes gets just the key, value  pairs that cannot be parsed into
// a known struct
func attributes(mappedJSON map[string]*json.RawMessage) map[string]string {
	cleanedAttributes := map[string]string{}
	for key, val := range mappedJSON {
		if _, found := parsableFields[key]; !found {
			cleanedAttributes[key] = sanitizeLabelValue(string(*val))
		}
	}
	return cleanedAttributes
}

func (d *Device) infoJSON() (*DeviceInfo, error) {
	opts := append(smartctlDeviceInfoOpts, "-d", d.Type, d.Name)
	output, err := smartCtl(useJSON(opts)...)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	mappedJSON, err := parseJSON(output)
	if err != nil {
		return nil, err
	}
	info := DeviceInfo{
		Attributes: attributes(mappedJSON),
	}
	if statusData, ok := mappedJSON["smart_status"]; ok {
		statusDetail, err := parseJSON([]byte(*statusData))
		if err != nil {
			return nil, err
		}
		if passed, ok := statusDetail["passed"]; ok {
			if string(*passed) == "true" {
				info.Available = true
				info.Enabled = true
				info.Healthy = true
			}
		}
	}
	return &info, nil
}

// sanitizeLabelValue removes unnecessary characters from label values
func sanitizeLabelValue(value string) string {
	value = strings.ReplaceAll(value, "\"", "")
	value = strings.ReplaceAll(value, "\n", "")
	return value
}
