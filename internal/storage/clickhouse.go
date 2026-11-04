package storage

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseStore struct {
	conn driver.Conn
}

// Ready pings ClickHouse.
func (s *ClickHouseStore) Ready(ctx context.Context) error {
	return s.conn.Ping(ctx)
}

// Retention bounds for SPAN_RETENTION_DAYS.
const (
	DefaultRetentionDays = 90
	MaxRetentionDays     = 3650
)

// StoreOptions configures NewClickHouseStore.
type StoreOptions struct {
	// RetentionDays is the spans TTL (1..MaxRetentionDays; 0 means the default).
	RetentionDays int
	// ManageSchema makes this process apply schema migrations and then
	// reconcile the spans TTL to RetentionDays. Exactly one service (the
	// collector) should set it; others only read.
	ManageSchema bool
}

func NewClickHouseStore(dsn string, so StoreOptions) (*ClickHouseStore, error) {
	days := so.RetentionDays
	if days == 0 {
		days = DefaultRetentionDays
	}
	if days < 1 || days > MaxRetentionDays {
		return nil, fmt.Errorf("retention must be 1..%d days, got %d", MaxRetentionDays, days)
	}
	opts, err := clickhouseOptions(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, err
	}

	s := &ClickHouseStore{conn: conn}
	if so.ManageSchema {
		ctx := context.Background()
		if _, err := s.Migrate(ctx); err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		if err := s.reconcileRetention(ctx, days); err != nil {
			return nil, fmt.Errorf("reconcile retention: %w", err)
		}
	}
	return s, nil
}

// clickhouseOptions parses the DSN. TLS follows the DSN (secure=true); it was
// previously forced off, silently downgrading "secure" DSNs to plaintext.
func clickhouseOptions(dsn string) (*clickhouse.Options, error) {
	return clickhouse.ParseDSN(dsn)
}

var ttlDaysRe = regexp.MustCompile(`TTL toDateTime\(\w+\) \+ toIntervalDay\((\d+)\)`)

// retentionTables maps each table with a TTL to its time column.
var retentionTables = []struct{ table, column string }{
	{"spans", "start_time"},
	{"logs", "timestamp"},
}

// ttlDays extracts the retention from a spans table's engine_full, as
// ClickHouse normalizes it ("TTL toDateTime(start_time) + toIntervalDay(90)").
func ttlDays(engineFull string) (int, bool) {
	m := ttlDaysRe.FindStringSubmatch(engineFull)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// reconcileRetention changes each retained table's TTL when it differs from
// days. Shortening it lets ClickHouse drop rows that are now too old.
func (s *ClickHouseStore) reconcileRetention(ctx context.Context, days int) error {
	for _, t := range retentionTables {
		var engineFull string
		if err := s.conn.QueryRow(ctx,
			`SELECT engine_full FROM system.tables WHERE database = currentDatabase() AND name = ?`, t.table,
		).Scan(&engineFull); err != nil {
			return fmt.Errorf("%s: %w", t.table, err)
		}
		if current, ok := ttlDays(engineFull); ok && current == days {
			continue
		}
		// Table and column names are constants above; days is a validated int.
		if err := s.conn.Exec(ctx, fmt.Sprintf(`ALTER TABLE %s MODIFY TTL toDateTime(%s) + INTERVAL %d DAY`,
			t.table, t.column, days)); err != nil {
			return fmt.Errorf("%s: %w", t.table, err)
		}
	}
	return nil
}

// LogRow is one row of the logs table.
type LogRow struct {
	Timestamp  time.Time
	AgentName  string
	Severity   string
	Body       string
	Attributes map[string]string
	TraceID    string
	SpanID     string
}

// InsertLog writes one log record with the batch protocol (full-precision timestamps).
func (s *ClickHouseStore) InsertLog(ctx context.Context, row LogRow) error {
	batch, err := s.conn.PrepareBatch(ctx, `INSERT INTO logs (timestamp, agent_name, severity, body, attributes, trace_id, span_id)`)
	if err != nil {
		return err
	}
	if err := batch.Append(row.Timestamp, row.AgentName, row.Severity, row.Body, row.Attributes, row.TraceID, row.SpanID); err != nil {
		_ = batch.Abort()
		return err
	}
	return batch.Send()
}

type SpanRow struct {
	TraceID         string
	SpanID          string
	ParentID        string
	AgentName       string
	SpanName        string
	StartTime       time.Time
	EndTime         time.Time
	DurationMs      uint64
	Status          string
	Attributes      map[string]string
	LLMModel        *string
	LLMPrompt       *string
	LLMCompletion   *string
	LLMInputTokens  *uint32
	LLMOutputTokens *uint32
	LLMCostUSD      *float64
}

// InsertSpan writes one span. It uses the native batch protocol rather than
// Exec with ? placeholders: positional binding formats time.Time at second
// precision, which silently truncated every DateTime64(9) timestamp.
func (s *ClickHouseStore) InsertSpan(ctx context.Context, row SpanRow) error {
	batch, err := s.conn.PrepareBatch(ctx, `
		INSERT INTO spans (
			trace_id, span_id, parent_id, agent_name, span_name,
			start_time, end_time, duration_ms, status, attributes,
			llm_model, llm_prompt, llm_completion,
			llm_input_tokens, llm_output_tokens, llm_cost_usd
		)`)
	if err != nil {
		return err
	}
	if err := batch.Append(
		row.TraceID, row.SpanID, row.ParentID, row.AgentName, row.SpanName,
		row.StartTime, row.EndTime, row.DurationMs, row.Status, row.Attributes,
		row.LLMModel, row.LLMPrompt, row.LLMCompletion,
		row.LLMInputTokens, row.LLMOutputTokens, row.LLMCostUSD,
	); err != nil {
		_ = batch.Abort()
		return err
	}
	return batch.Send()
}

// TraceSummary is one row of the trace list: a whole trace rolled up from its spans.
type TraceSummary struct {
	TraceID      string
	RootName     string // name of the root span (or earliest span if none is a root)
	Started      time.Time
	Ended        time.Time
	SpanCount    uint64
	ErrorCount   uint64
	Agents       []string
	InputTokens  uint64
	OutputTokens uint64
	CostUSD      float64
}

// TraceFilter selects traces for the list page.
type TraceFilter struct {
	Window     time.Duration // only traces with spans starting within this window (required, >0)
	Agent      string        // only traces that include a span from this agent
	ErrorsOnly bool          // only traces with at least one error span
	Limit      int

	// Keyset cursor: only traces strictly older than (BeforeStarted, BeforeTraceID)
	// in (started DESC, trace_id DESC) order. Zero BeforeStarted means the first page.
	BeforeStarted time.Time
	BeforeTraceID string
}

// QueryTraces returns trace summaries, newest first. The time window bounds
// the scan; it is passed as whole seconds rather than a time.Time, whose
// positional binding would be truncated to seconds anyway.
func (s *ClickHouseStore) QueryTraces(ctx context.Context, f TraceFilter) ([]TraceSummary, error) {
	window := int64(f.Window / time.Second)
	if window <= 0 || f.Limit <= 0 {
		return nil, fmt.Errorf("query traces: window and limit must be positive")
	}
	where := "start_time >= now64(9) - toIntervalSecond(?)"
	args := []any{window}
	if f.Agent != "" {
		// Select whole traces that include the agent, so totals still cover every span.
		where += " AND trace_id IN (SELECT trace_id FROM spans WHERE agent_name = ? AND start_time >= now64(9) - toIntervalSecond(?))"
		args = append(args, f.Agent, window)
	}
	var conds []string
	if f.ErrorsOnly {
		conds = append(conds, "error_count > 0")
	}
	if !f.BeforeStarted.IsZero() {
		// The cursor time travels as integer nanoseconds: binding a time.Time
		// would truncate it to seconds and skip or repeat traces at page edges.
		conds = append(conds, "(started, trace_id) < (fromUnixTimestamp64Nano(toInt64(?), 'UTC'), ?)")
		args = append(args, f.BeforeStarted.UnixNano(), f.BeforeTraceID)
	}
	having := ""
	if len(conds) > 0 {
		having = "HAVING " + strings.Join(conds, " AND ")
	}
	args = append(args, f.Limit)

	rows, err := s.conn.Query(ctx, `
		SELECT trace_id,
		       argMin(span_name, (parent_id != '', start_time)) AS root_name,
		       min(start_time)                                   AS started,
		       max(end_time)                                     AS ended,
		       count()                                           AS span_count,
		       countIf(status = 'error')                         AS error_count,
		       arraySort(groupUniqArray(20)(agent_name))         AS agents,
		       sum(toUInt64(ifNull(llm_input_tokens, 0)))        AS input_tokens,
		       sum(toUInt64(ifNull(llm_output_tokens, 0)))       AS output_tokens,
		       sum(ifNull(llm_cost_usd, 0))                      AS cost_usd
		FROM spans
		WHERE `+where+`
		GROUP BY trace_id
		`+having+`
		ORDER BY started DESC, trace_id DESC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TraceSummary
	for rows.Next() {
		var t TraceSummary
		if err := rows.Scan(&t.TraceID, &t.RootName, &t.Started, &t.Ended, &t.SpanCount, &t.ErrorCount,
			&t.Agents, &t.InputTokens, &t.OutputTokens, &t.CostUSD); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// LogFilter selects log records for the logs page.
type LogFilter struct {
	Window     time.Duration // required, >0: bounds the scan
	Agent      string
	Severities []string // allowed severities (from an allowlist); empty = all
	Search     string   // case-insensitive substring of the body
	TraceID    string
	Limit      int
}

// QueryLogs returns log records, newest first. All filter values are bound
// parameters; Severities must come from schema's fixed set.
func (s *ClickHouseStore) QueryLogs(ctx context.Context, f LogFilter) ([]LogRow, error) {
	window := int64(f.Window / time.Second)
	if window <= 0 || f.Limit <= 0 {
		return nil, fmt.Errorf("query logs: window and limit must be positive")
	}
	where := []string{"timestamp >= now64(9) - toIntervalSecond(?)"}
	args := []any{window}
	if f.Agent != "" {
		where, args = append(where, "agent_name = ?"), append(args, f.Agent)
	}
	if len(f.Severities) > 0 {
		where, args = append(where, "severity IN ?"), append(args, f.Severities)
	}
	if f.Search != "" {
		where, args = append(where, "positionCaseInsensitiveUTF8(body, ?) > 0"), append(args, f.Search)
	}
	if f.TraceID != "" {
		where, args = append(where, "trace_id = ?"), append(args, f.TraceID)
	}
	args = append(args, f.Limit)
	rows, err := s.conn.Query(ctx, `
		SELECT timestamp, agent_name, toString(severity), body, attributes, trace_id, span_id
		FROM logs WHERE `+strings.Join(where, " AND ")+`
		ORDER BY timestamp DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogRow
	for rows.Next() {
		var r LogRow
		if err := rows.Scan(&r.Timestamp, &r.AgentName, &r.Severity, &r.Body, &r.Attributes, &r.TraceID, &r.SpanID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryAgents returns the distinct agent names seen within window, sorted,
// at most limit of them (for the trace list's agent suggestions).
func (s *ClickHouseStore) QueryAgents(ctx context.Context, window time.Duration, limit int) ([]string, error) {
	secs := int64(window / time.Second)
	if secs <= 0 || limit <= 0 {
		return nil, fmt.Errorf("query agents: window and limit must be positive")
	}
	rows, err := s.conn.Query(ctx, `
		SELECT DISTINCT agent_name FROM spans
		WHERE start_time >= now64(9) - toIntervalSecond(?) AND agent_name != ''
		ORDER BY agent_name LIMIT ?`, secs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// QueryTrace returns up to limit spans of one trace, earliest first.
func (s *ClickHouseStore) QueryTrace(ctx context.Context, traceID string, limit int) ([]SpanRow, error) {
	rows, err := s.conn.Query(ctx,
		`SELECT trace_id, span_id, parent_id, agent_name, span_name,
		        start_time, end_time, duration_ms, status, attributes,
		        llm_model, llm_input_tokens, llm_output_tokens, llm_cost_usd
		 FROM spans WHERE trace_id = ? ORDER BY start_time ASC LIMIT ?`, traceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SpanRow
	for rows.Next() {
		var r SpanRow
		if err := rows.Scan(
			&r.TraceID, &r.SpanID, &r.ParentID, &r.AgentName, &r.SpanName,
			&r.StartTime, &r.EndTime, &r.DurationMs, &r.Status, &r.Attributes,
			&r.LLMModel, &r.LLMInputTokens, &r.LLMOutputTokens, &r.LLMCostUSD,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
