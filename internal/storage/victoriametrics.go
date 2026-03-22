package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// VictoriaMetricsClient writes metrics using VictoriaMetrics' Prometheus import endpoint.
type VictoriaMetricsClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewVictoriaMetricsClient(baseURL string) *VictoriaMetricsClient {
	return &VictoriaMetricsClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type VMMetric struct {
	Name      string
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
}

// WriteMetrics sends a batch of metrics in Prometheus text format to VictoriaMetrics.
func (c *VictoriaMetricsClient) WriteMetrics(ctx context.Context, metrics []VMMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, m := range metrics {
		buf.WriteString(promLine(m))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/import/prometheus", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("victoriametrics: status %d", resp.StatusCode)
	}
	return nil
}

func promLine(m VMMetric) string {
	var sb strings.Builder
	sb.WriteString(m.Name)
	if len(m.Labels) > 0 {
		sb.WriteByte('{')
		first := true
		for k, v := range m.Labels {
			if !first {
				sb.WriteByte(',')
			}
			sb.WriteString(k)
			sb.WriteString(`="`)
			sb.WriteString(strings.ReplaceAll(v, `"`, `\"`))
			sb.WriteByte('"')
			first = false
		}
		sb.WriteByte('}')
	}
	fmt.Fprintf(&sb, " %g %d\n", m.Value, m.Timestamp.UnixMilli())
	return sb.String()
}
