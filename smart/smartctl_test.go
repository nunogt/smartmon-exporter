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

import "testing"

func TestVersion(t *testing.T) {
	_, err := Version()
	if err != nil {
		t.Fatal("unable to read smartmon tools version", err)
	}
}

func TestScan(t *testing.T) {
	devices, err := scanDevices()
	if err != nil {
		t.Fatal("unable to scan devices", err)
	}
	if len(devices) < 1 {
		t.Fatal("no smart devices found")
	}
}

func TestActive(t *testing.T) {
	device := Device{
		Name: "/foo", // non-existing device name should not be active
		Type: "nvme",
	}
	if active, _ := device.active(); active {
		t.Fatal("device should not be active")
	}
}
