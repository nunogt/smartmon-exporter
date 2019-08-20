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

package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/pgier/smartmon-exporter/smart"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

const (
	rootuid = 0
)

var (
	listenAddress = kingpin.Flag("web.listen-address", "Address on which to expose metrics and web interface.").Default(":9151").String()
	outputFile    = kingpin.Flag("output-file", "Filename which to write metrics.").Default("").String()
)

func main() {
	log.AddFlags(kingpin.CommandLine)
	kingpin.Version(version.Print("smartmon_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	if os.Geteuid() != rootuid {
		log.Infoln("Not running as root, some metrics will not be available")
	}

	smartmonCollector, err := smart.NewCollector()
	if err != nil {
		panic("Unable to create collector")
	}
	prometheus.MustRegister(smartmonCollector)

	if strings.TrimSpace(*outputFile) != "" {
		prometheus.WriteToTextfile(*outputFile, prometheus.DefaultGatherer)
	} else {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html>
				 <head><title>S.M.A.R.T. Exporter</title></head>
				 <body>
				 <h1>S.M.A.R.T. Exporter</h1>
				 <p><a href='` + "/metrics" + `'>Metrics</a></p>
				 </body>
				 </html>`))
		})

		log.Infoln("Listening on", *listenAddress)
		log.Fatal(http.ListenAndServe(*listenAddress, nil))
	}

}
