package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tjscanlan/conduit/internal/storage"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func row(id, parent, name string, startMs, endMs int, status string) storage.SpanRow {
	return storage.SpanRow{
		TraceID: "trace1", SpanID: id, ParentID: parent, SpanName: name, AgentName: "bot", Status: status,
		StartTime:  t0.Add(time.Duration(startMs) * time.Millisecond),
		EndTime:    t0.Add(time.Duration(endMs) * time.Millisecond),
		DurationMs: uint64(endMs - startMs), Attributes: map[string]string{},
	}
}

func names(v traceView) []string {
	var out []string
	for _, s := range v.Spans {
		out = append(out, strings.Repeat(">", s.Depth)+s.SpanName)
	}
	return out
}

func TestBuildTraceViewTreeOrder(t *testing.T) {
	// Rows arrive ordered by start time; the view nests children under parents.
	rows := []storage.SpanRow{
		row("r", "", "root", 0, 1000, "ok"),
		row("a", "r", "plan", 10, 100, "ok"),
		row("b", "r", "act", 200, 900, "error"),
		row("a1", "a", "llm", 20, 90, "ok"),
		row("b1", "b", "tool", 300, 800, "error"),
		row("o", "missing-parent", "orphan", 950, 990, "unset"),
	}
	got := strings.Join(names(buildTraceView("trace1", rows)), ",")
	want := "root,>plan,>>llm,>act,>>tool,orphan"
	if got != want {
		t.Fatalf("order = %s\nwant    %s", got, want)
	}
}

func TestBuildTraceViewTotalsAndGeometry(t *testing.T) {
	in, out, cost, model := uint32(10), uint32(20), 0.5, "m"
	llm := row("c", "r", "llm", 250, 750, "error")
	llm.LLMModel, llm.LLMInputTokens, llm.LLMOutputTokens, llm.LLMCostUSD = &model, &in, &out, &cost
	llm.Attributes = map[string]string{"error.message": "boom", "agent_name": "bot", "zeta": "z", "alpha": "a"}
	inst := row("i", "r", "instant", 1000, 1000, "ok")
	v := buildTraceView("trace1", []storage.SpanRow{row("r", "", "root", 0, 1000, "ok"), llm, inst})

	if v.DurationMs != 1000 || v.SpanCount != 3 || v.ErrorCount != 1 || !v.HasLLM ||
		v.InputTokens != 10 || v.OutputTokens != 20 || v.CostUSD != 0.5 || len(v.Agents) != 1 {
		t.Fatalf("totals = %+v", v)
	}
	s := v.Spans[1]
	if s.SpanName != "llm" || s.X != 250 || s.W != 500 || s.OffsetMs != 250 {
		t.Errorf("geometry for llm = x %v w %v offset %v", s.X, s.W, s.OffsetMs)
	}
	if s.ErrorMsg != "boom" || s.Model != "m" || s.TokensIn != "10" || s.Cost != "$0.5000" {
		t.Errorf("llm view = %+v", s)
	}
	if len(s.Attrs) != 2 || s.Attrs[0].Key != "alpha" || s.Attrs[1].Key != "zeta" {
		t.Errorf("attrs should be sorted and exclude promoted keys: %+v", s.Attrs)
	}
	// An instantaneous span at the very end stays visible and inside the box.
	last := v.Spans[2]
	if last.W < 2 || last.X+last.W > 1000 {
		t.Errorf("instant span geometry x %v w %v", last.X, last.W)
	}
}

func TestBuildTraceViewSurvivesCyclesAndDuplicates(t *testing.T) {
	rows := []storage.SpanRow{
		row("x", "y", "x", 0, 10, "ok"),
		row("y", "x", "y", 5, 10, "ok"), // x <-> y cycle: neither is a root
		row("d", "", "dup", 0, 1, "ok"),
		row("d", "", "dup", 0, 1, "ok"),
		row("self", "self", "self", 0, 1, "ok"),
	}
	v := buildTraceView("trace1", rows)
	seen := map[string]int{}
	for _, s := range v.Spans {
		seen[s.SpanID]++
	}
	for _, id := range []string{"x", "y", "d", "self"} {
		if seen[id] != 1 {
			t.Errorf("span %q rendered %d times, want 1 (all spans shown, none twice)", id, seen[id])
		}
	}
}

func TestBuildTraceViewDepthCapped(t *testing.T) {
	var rows []storage.SpanRow
	parent := ""
	for i := 0; i < maxIndentDepth+5; i++ {
		id := string(rune('a' + i))
		rows = append(rows, row(id, parent, id, i, 100, "ok"))
		parent = id
	}
	for _, s := range buildTraceView("trace1", rows).Spans {
		if s.Depth > maxIndentDepth {
			t.Fatalf("depth %d exceeds cap", s.Depth)
		}
	}
}

func TestTraceDetailEscapesAttributeValues(t *testing.T) {
	r := row("r", "", "root", 0, 10, "error")
	r.Attributes = map[string]string{"input": `<script>alert(1)</script>`, "error.message": `<img src=x onerror=alert(1)>`}
	body := do(newDataRouter(t, []storage.SpanRow{r}), httptest.NewRequest(http.MethodGet, "/traces/trace1", nil)).Body.String()
	if strings.Contains(body, "<script>alert") || strings.Contains(body, "<img src=x") {
		t.Fatal("attribute or error values rendered unescaped")
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("escaped attribute value missing")
	}
}
