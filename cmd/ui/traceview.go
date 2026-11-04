package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/tjscanlan/conduit/internal/storage"
)

// maxTraceSpans bounds how many spans of one trace the detail page loads, so a
// runaway agent can't make the UI fetch and render an unbounded trace.
const maxTraceSpans = 2000

// maxIndentDepth caps the depth-N CSS classes; deeper spans render at this depth.
const maxIndentDepth = 10

// traceView is the trace detail page's model: spans in tree order with
// waterfall geometry, plus trace-level totals.
type traceView struct {
	TraceID      string
	Start        time.Time
	DurationMs   int64
	SpanCount    int
	ErrorCount   int
	InputTokens  uint64
	OutputTokens uint64
	CostUSD      float64
	HasLLM       bool
	Truncated    bool // more than maxTraceSpans spans; totals cover only those shown
	Limit        int
	Agents       []string
	Spans        []spanView
}

type spanView struct {
	storage.SpanRow
	Depth       int     // nesting level, capped at maxIndentDepth
	OffsetMs    int64   // start relative to the trace start
	X, W        float64 // waterfall bar position and width, in 0..1000 viewBox units
	ErrorMsg    string
	Attrs       []attr // sorted; excludes keys shown elsewhere
	HasLLM      bool
	HasChildren bool
	// LLM fields formatted for display ("–" when absent).
	Model, TokensIn, TokensOut, Cost string
}

type attr struct{ Key, Value string }

// Attribute keys rendered in dedicated places rather than the attribute table.
var promotedAttrs = map[string]bool{"agent_name": true, "error.message": true}

func buildTraceView(traceID string, rows []storage.SpanRow) traceView {
	v := traceView{TraceID: traceID, SpanCount: len(rows), Limit: maxTraceSpans}
	if len(rows) == 0 {
		return v
	}

	start, end := rows[0].StartTime, rows[0].EndTime
	agents := map[string]bool{}
	byID := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.StartTime.Before(start) {
			start = r.StartTime
		}
		if r.EndTime.After(end) {
			end = r.EndTime
		}
		if r.Status == "error" {
			v.ErrorCount++
		}
		if r.LLMModel != nil {
			v.HasLLM = true
		}
		if r.LLMInputTokens != nil {
			v.InputTokens += uint64(*r.LLMInputTokens)
		}
		if r.LLMOutputTokens != nil {
			v.OutputTokens += uint64(*r.LLMOutputTokens)
		}
		if r.LLMCostUSD != nil {
			v.CostUSD += *r.LLMCostUSD
		}
		if r.AgentName != "" {
			agents[r.AgentName] = true
		}
		byID[r.SpanID] = true
	}
	v.Start = start
	total := end.Sub(start)
	v.DurationMs = total.Milliseconds()
	for a := range agents {
		v.Agents = append(v.Agents, a)
	}
	sort.Strings(v.Agents)

	// Children by parent; spans whose parent isn't in this trace are roots.
	children := map[string][]storage.SpanRow{}
	var roots []storage.SpanRow
	for _, r := range rows {
		if r.ParentID != "" && byID[r.ParentID] && r.ParentID != r.SpanID {
			children[r.ParentID] = append(children[r.ParentID], r)
		} else {
			roots = append(roots, r)
		}
	}
	byStart := func(s []storage.SpanRow) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].StartTime.Before(s[j].StartTime) })
	}
	byStart(roots)

	visited := make(map[string]bool, len(rows))
	var walk func(r storage.SpanRow, depth int)
	walk = func(r storage.SpanRow, depth int) {
		if visited[r.SpanID] { // duplicate span IDs or a parent cycle
			return
		}
		visited[r.SpanID] = true
		kids := children[r.SpanID]
		byStart(kids)
		v.Spans = append(v.Spans, newSpanView(r, depth, start, total, len(kids) > 0))
		for _, k := range kids {
			walk(k, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	// Spans only reachable through a cycle were skipped above; show them as roots.
	for _, r := range rows {
		if !visited[r.SpanID] {
			walk(r, 0)
		}
	}
	return v
}

func newSpanView(r storage.SpanRow, depth int, traceStart time.Time, total time.Duration, hasChildren bool) spanView {
	sv := spanView{
		SpanRow:     r,
		Depth:       min(depth, maxIndentDepth),
		OffsetMs:    r.StartTime.Sub(traceStart).Milliseconds(),
		ErrorMsg:    r.Attributes["error.message"],
		HasLLM:      r.LLMModel != nil,
		HasChildren: hasChildren,
		W:           1000,
	}
	if total > 0 {
		sv.X = 1000 * float64(r.StartTime.Sub(traceStart)) / float64(total)
		sv.W = 1000 * float64(r.EndTime.Sub(r.StartTime)) / float64(total)
	}
	if sv.HasLLM {
		sv.Model = *r.LLMModel
		sv.TokensIn, sv.TokensOut, sv.Cost = "–", "–", "–"
		if r.LLMInputTokens != nil {
			sv.TokensIn = strconv.FormatUint(uint64(*r.LLMInputTokens), 10)
		}
		if r.LLMOutputTokens != nil {
			sv.TokensOut = strconv.FormatUint(uint64(*r.LLMOutputTokens), 10)
		}
		if r.LLMCostUSD != nil {
			sv.Cost = fmt.Sprintf("$%.4f", *r.LLMCostUSD)
		}
	}
	sv.W = max(sv.W, 2) // keep instantaneous spans visible
	if sv.X+sv.W > 1000 {
		sv.X = max(0, 1000-sv.W)
	}
	for k, val := range r.Attributes {
		if !promotedAttrs[k] {
			sv.Attrs = append(sv.Attrs, attr{k, val})
		}
	}
	sort.Slice(sv.Attrs, func(i, j int) bool { return sv.Attrs[i].Key < sv.Attrs[j].Key })
	return sv
}
