package metrics

import (
	"encoding/json"
	"fmt"
)

type Metrics struct {
	Metrics []Metric `json:"metrics"`
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func NewMetric(name string, description string, label string, metricType MetricType, value interface{}) Metric {
	m := Metric{
		MetricName:  name,
		Description: description,
		MetricLabel: label,
		MetricType:  metricType,
	}
	switch value.(type) {
	case float64:
		m.FloatValue = value.(float64)
	case int64:
		m.IntValue = value.(int64)
	case bool:
		m.BoolValue = value.(bool)
	case string:
		m.StringValue = value.(string)
	}
	return m
}

type MetricType int

const (
	MetricTypeFloat MetricType = iota
	MetricTypeInt
	MetricTypeBool
	MetricTypeString
)

func (mt MetricType) MarshalJSON() ([]byte, error) {
	var s string
	switch mt {
	case MetricTypeFloat:
		s = "float"
	case MetricTypeInt:
		s = "int"
	case MetricTypeBool:
		s = "bool"
	case MetricTypeString:
		s = "string"
	default:
		s = "unknown"
	}
	return json.Marshal(s)
}

func (mt *MetricType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch s {
	case "float":
		*mt = MetricTypeFloat
	case "int":
		*mt = MetricTypeInt
	case "bool":
		*mt = MetricTypeBool
	case "string":
		*mt = MetricTypeString
	default:
		return fmt.Errorf("unknown metric type: %s", s)
	}
	return nil
}

type Metric struct {
	MetricName  string     `json:"metricName"`
	MetricLabel string     `json:"metricLabel,omitempty"`
	Description string     `json:"description,omitempty"`
	MetricType  MetricType `json:"metricType"`
	FloatValue  float64    `json:"floatValue,omitempty"`
	IntValue    int64      `json:"intValue,omitempty"`
	BoolValue   bool       `json:"boolValue,omitempty"`
	StringValue string     `json:"stringValue,omitempty"`
}

func (m *Metrics) AddMetric(metric Metric) {
	m.Metrics = append(m.Metrics, metric)
}

func (m *Metrics) GetMetrics() []Metric {
	return m.Metrics
}
