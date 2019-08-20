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
	"errors"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	noLabels      = []string{}
	noConstLabels = prometheus.Labels{}

	smartMonVersionDesc = prometheus.NewDesc("smartmon_version", "version reported by smartctl -V", []string{"vesion"}, prometheus.Labels{})
	smartMonRunDesc     = prometheus.NewDesc("smartmon_smartctl_run", "contains current unix time", []string{"disk", "type"}, noConstLabels)
	smartMonActiveDesc  = prometheus.NewDesc("smartmon_device_active", "shows result of smartctl -n standby", []string{"disk", "type"}, noConstLabels)
)

// Collector collects smartmon metrics for Prometheus
type Collector struct {
}

// NewCollector initializes a new prometheus collector for
// smartmon metrics
func NewCollector() (*Collector, error) {
	return &Collector{}, nil
}

// Collect implements the prometheus.Collector interface and
// reads the smartmon metrics
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	version, _ := Version()
	ch <- prometheus.MustNewConstMetric(smartMonVersionDesc, prometheus.GaugeValue, 1.0, version)
	devices, err := getDeviceList()
	if err != nil {
		log.Infoln("unable to scan smart devices: ", err)
		return
	}
	for _, d := range devices {
		active, _ := d.active()

		if active {
			ch <- prometheus.MustNewConstMetric(smartMonActiveDesc, prometheus.GaugeValue, 1.0, d.Name, d.Type)
			CollectInfoMetrics(ch, d)
			CollectVendorAttributes(ch, d)
		} else { // don't collect from inactive devices to avoid waking them up
			ch <- prometheus.MustNewConstMetric(smartMonActiveDesc, prometheus.GaugeValue, 0.0, d.Name, d.Type)
		}
	}
}

func getDeviceList() ([]Device, error) {
	if JSONCapable() {
		return scanDevicesJSON()
	}
	return scanDevices()
}

// Describe implements the prometheus.Collector interface
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

// CollectInfoMetrics collects metrics based on output of
// 'smartctl -i -H -d <type> <dev>'
func CollectInfoMetrics(ch chan<- prometheus.Metric, device Device) {
	info, err := getDevInfo(device)
	if err != nil {
		log.Infoln("error collecting device info for "+device.Name+":", err)
		return
	}
	commonLabels := map[string]string{
		"disk": device.Name,
		"type": device.Type,
	}
	infoLabels := mergeMaps(commonLabels, info.Attributes)
	descInfo := prometheus.NewDesc("smartmon_device_info", "smartmon_device_info", noLabels, infoLabels)
	ch <- prometheus.MustNewConstMetric(descInfo, prometheus.GaugeValue, 1.0)
	descAvailable := prometheus.NewDesc("smartmon_device_smart_available", "smartmon_device_smart_available", noLabels, commonLabels)
	ch <- prometheus.MustNewConstMetric(descAvailable, prometheus.GaugeValue, boolToMetric(info.Available))
	descEnabled := prometheus.NewDesc("smartmon_device_smart_enabled", "smartmon_device_smart_enabled", noLabels, commonLabels)
	ch <- prometheus.MustNewConstMetric(descEnabled, prometheus.GaugeValue, boolToMetric(info.Enabled))
	descHealthy := prometheus.NewDesc("smartmon_device_smart_healthy", "smartmon_device_smart_healthy", noLabels, commonLabels)
	ch <- prometheus.MustNewConstMetric(descHealthy, prometheus.GaugeValue, boolToMetric(info.Healthy))
}

func getDevInfo(device Device) (*DeviceInfo, error) {
	if JSONCapable() {
		return device.infoJSON()
	}
	return device.info()
}

// boolToMetric converts a boolean value to a metric float value of 1.0 or 0.0
func boolToMetric(val bool) float64 {
	if val {
		return 1.0
	}
	return 0.0
}

// CollectVendorAttributes collects smart Attributes based on output of
// 'smartctl -A -d <type> <device>'
func CollectVendorAttributes(ch chan<- prometheus.Metric, dev Device) error {
	if strings.HasPrefix(dev.Type, "nvme") {
		return CollectNvmeVendorAttributes(ch, dev)
	} else if strings.HasPrefix(dev.Type, "sat") {
		return CollectSatVendorAttributes(ch, dev)
	} // TODO: add support for scsi and megaraid devices
	return errors.New("unrecognized device type: " + dev.Type)
}

// CollectNvmeVendorAttributes collects vendor specific attributes for nvme devices
func CollectNvmeVendorAttributes(ch chan<- prometheus.Metric, dev Device) error {
	opts := append(smartctlDeviceMetricOpts, "-d", dev.Type, dev.Name)
	output, err := smartCtl(opts...)
	if err != nil {
		log.Infoln("error collecting vendor specific attributes for "+dev.Name+":", err)
		return err
	}

	labels := map[string]string{}
	labels["disk"] = dev.Name
	labels["type"] = dev.Type
	for _, line := range strings.Split(string(output)[4:], "\n") {
		fields := strings.Split(line, ":")
		if len(fields) == 2 {
			labels[sanitizeLabelName(fields[0])] = strings.TrimSpace(fields[1])
		}
	}
	metricName := "smartmon_attributes"

	vendorAttrDesc := prometheus.NewDesc(metricName, metricName, noLabels, labels)
	ch <- prometheus.MustNewConstMetric(vendorAttrDesc, prometheus.GaugeValue, 1.0)
	return nil
}

// CollectSatVendorAttributes collects smart Attributes based on output of
// 'smartctl -A -d <type> <device>'
func CollectSatVendorAttributes(ch chan<- prometheus.Metric, dev Device) error {
	opts := append(smartctlDeviceMetricOpts, "-d", dev.Type, dev.Name)
	output, _ := smartCtl(opts...)

	constLabels := prometheus.Labels{
		"disk": dev.Name,
		"type": dev.Type,
	}

	for _, line := range strings.Split(string(output)[1:], "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		labels := prometheus.Labels{}
		for key, value := range constLabels {
			labels[key] = value
		}
		labels["smart_id"] = fields[0]
		metricPrefix := "smartmon_" + strings.ToLower(fields[1])

		deviceValueAttrDesc := prometheus.NewDesc(metricPrefix+"_value", metricPrefix+"_value", noLabels, labels)
		value, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(deviceValueAttrDesc, prometheus.GaugeValue, value)

		deviceWorstAttrDesc := prometheus.NewDesc(metricPrefix+"_worst", metricPrefix+"_worst", noLabels, labels)
		value, err = strconv.ParseFloat(fields[4], 64)
		if err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(deviceWorstAttrDesc, prometheus.GaugeValue, value)

		deviceThresholdAttrDesc := prometheus.NewDesc(metricPrefix+"_threshold", metricPrefix+"_threshold", noLabels, labels)
		value, err = strconv.ParseFloat(fields[5], 64)
		if err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(deviceThresholdAttrDesc, prometheus.GaugeValue, value)

		deviceRawAttrDesc := prometheus.NewDesc(metricPrefix+"_raw_value", metricPrefix+"_raw_value", noLabels, labels)
		value, err = strconv.ParseFloat(fields[9], 64)
		if err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(deviceRawAttrDesc, prometheus.GaugeValue, value)

	}
	return nil

}

func mergeMaps(map1 map[string]string, map2 map[string]string) map[string]string {
	combined := map[string]string{}
	for key, val := range map1 {
		combined[key] = val
	}
	for key, val := range map2 {
		combined[key] = val
	}
	return combined
}
