package main

import (
	"os"
)

const (
	rootuid            = 0
	smartMonMinVersion = "6.6"
)

func must(err error, msg string) {
	if err != nil {
		printMetric(NewInfoMetric("smartmon_collector_error", msg+":"+err.Error()))
		os.Exit(1)
	}
}

func main() {
	//log.Println("Collecting metrics from smartctl")
	//log.Println("User ID: ", os.Getuid(), ", Effective user ID: ", os.Geteuid())
	if os.Geteuid() != rootuid {
		printMetric(NewInfoMetric("smartmon_current_user_warning", "Not running as root, not all metrics will be available"))
	}

	must(smartCtrlAvailable(), "Unable to locate smartctl in path")
	must(smartCtrlCheckVersion(smartMonMinVersion), "Failed to verify smartctl version")
	printMetric(smartMonVersionMetric())
	for dev, devType := range smartMonDeviceList() {
		printMetric(smartMonRunMetric(dev, devType))
		activeMetric := smartMonDeviceActive(dev, devType)
		printMetric(activeMetric)
		if activeMetric.Value == "1" {
			printMetrics(smartMonInfoMetrics(dev, devType))
			printMetrics(smartMonDeviceAttributes(dev, devType))
		}
	}
	//fmt.Println(smartMonDeviceList())

}
