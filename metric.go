package main

import (
	"os"
	"text/template"
)

// MetricDesc provides the static metric information
type MetricDesc struct {
	Name        string
	Help        string
	Type        string
	ConstLabels map[string]string
}

// Metric provides the information necessary to print a metric
type Metric struct {
	Desc   *MetricDesc
	Value  string
	Labels map[string]string
}

const metricTemplate = `# HELP {{.Desc.Name}} SMART metric {{.Desc.Help}}
# TYPE {{.Desc.Name}} {{.Desc.Type}}
{{.Desc.Name}}{{"{" -}}{{template "labelsTemplate" .Labels}}{{- "}"}} {{.Value}}
`
const labelsTemplate = `{{$first := true}}{{range $key, $value := . -}}
{{if $first}}{{$first = false}}{{else}},{{end}}
{{- $key}}="{{$value -}}"
{{- end -}}
`

func init() {
	metricTmpl = template.Must(template.New("metricTemplate").Parse(metricTemplate))
	template.Must(metricTmpl.New("labelsTemplate").Parse(labelsTemplate))
}

var metricTmpl *template.Template

func printMetrics(m []*Metric) {
	for _, metric := range m {
		printMetric(metric)
	}
}

func printMetric(m *Metric) {
	metricTmpl.Execute(os.Stdout, m)
}

// NewInfoMetric creates a new Metric struct which provides an
// informational message such as an error
func NewInfoMetric(name string, msg string) *Metric {
	labels := map[string]string{
		"info": msg,
	}
	return NewHelpMetric(name, labels, "1", "Status information related to metric collection")
}

// NewMetric creates a basic metric struct with a default help string and
// the default metric type (guage).
func NewMetric(name string, labels map[string]string, value string) *Metric {
	return NewHelpMetric(name, labels, value, "SMART metric "+name)
}

// NewHelpMetric creates a basic metric struct with a default help string and
// the default metric type (guage).
func NewHelpMetric(name string, labels map[string]string, value string, help string) *Metric {
	return &Metric{
		Desc: &MetricDesc{
			Name: name,
			Help: "SMART metric " + name,
			Type: "guage",
		},
		Value:  "1",
		Labels: labels,
	}
}
