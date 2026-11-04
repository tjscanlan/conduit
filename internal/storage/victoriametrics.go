package storage

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
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

// promLine renders one sample in the Prometheus text format. Label values
// are agent-controlled (span names, attributes), so they are escaped exactly
// as the format requires: backslash, double quote, and newline. Escaping only
// the quote let a value end the line and inject arbitrary samples, such as
// forged metrics for another agent. Labels are sorted for stable output.
func promLine(m VMMetric) string {
	var sb strings.Builder
	sb.WriteString(m.Name)
	if len(m.Labels) > 0 {
		keys := make([]string, 0, len(m.Labels))
		for k := range m.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(k)
			sb.WriteString(`="`)
			sb.WriteString(labelValueEscaper.Replace(m.Labels[k]))
			sb.WriteByte('"')
		}
		sb.WriteByte('}')
	}
	fmt.Fprintf(&sb, " %g %d\n", m.Value, m.Timestamp.UnixMilli())
	return sb.String()
}

var labelValueEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
