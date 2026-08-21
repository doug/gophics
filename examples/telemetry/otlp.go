package main

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// OTLP/JSON — the OpenTelemetry Protocol's JSON encoding — is what this loads.
// It is what an `otlphttp` exporter posts with Content-Type: application/json,
// what the Collector's file exporter writes, and what `telemetrygen` emits, so
// a real capture from a real service can be dropped straight into this demo.
//
// The shape is three levels deep: resourceSpans (one per producing process,
// carrying the resource attributes that identify it), each holding scopeSpans
// (one per instrumentation library), each holding the spans themselves.
//
// Two wrinkles the spec imposes and this decoder handles:
//
//   - 64-bit integers are encoded as JSON *strings*, because JSON numbers can't
//     carry them losslessly. Timestamps and int attributes therefore arrive
//     quoted, and sometimes unquoted from encoders that ignore that rule, so
//     every number here accepts both.
//   - The HTTP semantic conventions were renamed in v1.21 (http.method →
//     http.request.method, http.status_code → http.response.status_code, and so
//     on). Captures in the wild use either, so both spellings are read.
//
// Files may hold one JSON object or a stream of them (the file exporter writes
// one per line); the decoder loops until EOF, so both work.

// sampleOTLP is a small, readable capture in the real wire format — 83 spans
// over five services, one of which is having a bad afternoon. It ships with the
// example so the OTLP path can be demonstrated with no file and no collector,
// including in the browser.
//
//go:embed otlp-sample.json
var sampleOTLP string

// DecodeOTLP reads OTLP/JSON traces and converts them to Spans, interning names
// into v as it goes. Timestamps are returned in milliseconds on the file's own
// clock; Store.Replace rebases them.
func DecodeOTLP(r io.Reader, v *vocab) ([]Span, error) {
	dec := json.NewDecoder(r)
	var out []Span
	for {
		var req otlpRequest
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("otlp: %w", err)
		}
		for _, rs := range req.ResourceSpans {
			out = append(out, rs.spans(v)...)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("otlp: no spans found (is this an OTLP/JSON trace export?)")
	}
	return out, nil
}

type otlpRequest struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource struct {
		Attributes []attr `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Spans []otlpSpan `json:"spans"`
	} `json:"scopeSpans"`
}

// spans flattens one resource's spans, applying the resource-level identity
// (service name, host) to each.
func (rs resourceSpans) spans(v *vocab) []Span {
	res := attrs(rs.Resource.Attributes)
	service := res.first("service.name")
	if service == "" {
		service = "unknown"
	}
	resHost := res.first("host.name", "service.instance.id", "k8s.pod.name")

	svc := v.svc.intern(service)
	var out []Span
	for _, ss := range rs.ScopeSpans {
		for _, sp := range ss.Spans {
			out = append(out, sp.decode(v, svc, resHost))
		}
	}
	return out
}

type otlpSpan struct {
	TraceID    string      `json:"traceId"`
	Name       string      `json:"name"`
	StartNanos json.Number `json:"startTimeUnixNano"`
	EndNanos   json.Number `json:"endTimeUnixNano"`
	Attributes []attr      `json:"attributes"`
	Status     struct {
		Code json.Number `json:"code"`
	} `json:"status"`
}

func (sp otlpSpan) decode(v *vocab, svc uint16, resHost string) Span {
	a := attrs(sp.Attributes)

	// Route: the low-cardinality template if the instrumentation recorded one,
	// else the raw path, else the span name — which for HTTP servers is
	// conventionally "GET /route" already.
	route := a.first("http.route", "url.path", "http.target")
	if method := a.first("http.request.method", "http.method"); method != "" && route != "" {
		route = method + " " + route
	}
	if route == "" {
		route = sp.Name
	}
	host := a.first("server.address", "net.host.name", "host.name")
	if host == "" {
		host = resHost
	}
	if host == "" {
		host = "—"
	}

	start, end := u64(sp.StartNanos), u64(sp.EndNanos)
	dur := int32(0)
	if end > start {
		dur = int32((end - start) / 1000) // nanoseconds → microseconds
	}

	out := Span{
		At:    int32(start / 1e6), // nanoseconds → milliseconds
		Dur:   dur,
		Bytes: -1,
		Svc:   svc,
		Route: v.route.intern(route),
		Host:  v.host.intern(host),
		Code:  status(a, sp.Status.Code),
	}
	if n := a.firstInt("http.response.body.size", "http.response_content_length", "http.response.size"); n >= 0 {
		out.Bytes = int32(n)
	}
	// A short or malformed trace ID decodes to whatever prefix is valid; the
	// rest stays zero rather than dropping the span.
	if raw, err := hex.DecodeString(sp.TraceID); err == nil {
		copy(out.Trace[:], raw)
	}
	return out
}

// status prefers the HTTP status code, and falls back to the span's own OTLP
// status — 2 is STATUS_CODE_ERROR — so non-HTTP spans still colour correctly.
func status(a attrs, spanStatus json.Number) uint16 {
	if n := a.firstInt("http.response.status_code", "http.status_code"); n > 0 {
		return uint16(n)
	}
	if spanStatus.String() == "2" || spanStatus.String() == "STATUS_CODE_ERROR" {
		return 500
	}
	return 200
}

// --- Attributes --------------------------------------------------------------

// attr is one key/value pair. OTLP wraps every value in a type tag, so a string
// arrives as {"stringValue":"GET"} and an int as {"intValue":"200"} — quoted,
// per the 64-bit rule.
type attr struct {
	Key   string `json:"key"`
	Value struct {
		StringValue *string      `json:"stringValue"`
		IntValue    *json.Number `json:"intValue"`
		DoubleValue *json.Number `json:"doubleValue"`
		BoolValue   *bool        `json:"boolValue"`
	} `json:"value"`
}

func (a attr) str() string {
	switch {
	case a.Value.StringValue != nil:
		return *a.Value.StringValue
	case a.Value.IntValue != nil:
		return a.Value.IntValue.String()
	case a.Value.DoubleValue != nil:
		return a.Value.DoubleValue.String()
	case a.Value.BoolValue != nil:
		return strconv.FormatBool(*a.Value.BoolValue)
	}
	return ""
}

type attrs []attr

// first returns the value of the first key present, trying each in turn — which
// is how the old and new semantic-convention spellings are both accepted.
func (as attrs) first(keys ...string) string {
	for _, k := range keys {
		for _, a := range as {
			if a.Key == k {
				if v := strings.TrimSpace(a.str()); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// firstInt is first, parsed; -1 means absent or unparseable.
func (as attrs) firstInt(keys ...string) int64 {
	if v := as.first(keys...); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return -1
}

// u64 reads a JSON number that may have arrived quoted or bare.
func u64(n json.Number) uint64 {
	v, err := strconv.ParseUint(strings.Trim(n.String(), `"`), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
