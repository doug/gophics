package wgpu

import (
	"encoding/json"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// GPU resource instrumentation.
//
// Every GPU resource this package creates is counted here: how many, how long
// creation took, and how many bytes it reserved. The point is to make claims
// about GPU performance checkable. A frame that allocates a buffer per draw
// and a frame that reuses one look identical from the outside and identical in
// a wall-clock average that is dominated by something else; they differ here.
//
// Off by default, and off costs one atomic load per resource creation --
// creation is already a syscall-scale operation, so the guard is not
// measurable against it. Turn it on with EnableStats(true).
//
// The counters are process-global rather than per-Device on purpose. A caller
// measuring frame churn wants the total the process paid, and threading a
// stats handle through every construction path would be a large change to
// argument lists for something that is off in production.

// ResourceKind identifies a category of GPU resource.
type ResourceKind int

// Resource kinds tracked by the instrumentation.
const (
	ResourceRenderPipeline ResourceKind = iota
	ResourceComputePipeline
	ResourceBuffer
	ResourceTexture
	ResourceBindGroup
	ResourceShaderModule
	numResourceKinds
)

// String returns the resource kind's name.
func (k ResourceKind) String() string {
	switch k {
	case ResourceRenderPipeline:
		return "render_pipeline"
	case ResourceComputePipeline:
		return "compute_pipeline"
	case ResourceBuffer:
		return "buffer"
	case ResourceTexture:
		return "texture"
	case ResourceBindGroup:
		return "bind_group"
	case ResourceShaderModule:
		return "shader_module"
	default:
		return "unknown"
	}
}

var statsEnabled atomic.Bool

type kindCounters struct {
	count atomic.Int64
	nanos atomic.Int64
	bytes atomic.Int64
}

var (
	resourceStats [numResourceKinds]kindCounters

	// labelMu guards labelCounts. Label tracking answers "which pipeline got
	// rebuilt", which a bare count cannot. It is only populated while stats
	// are enabled, so the map cannot grow in production.
	labelMu     sync.Mutex
	labelCounts = map[string]int64{}
)

// EnableStats turns resource instrumentation on or off. It does not reset the
// counters; call ResetStats for that.
func EnableStats(on bool) { statsEnabled.Store(on) }

// StatsEnabled reports whether resource instrumentation is recording.
func StatsEnabled() bool { return statsEnabled.Load() }

// recordResource notes one resource creation. bytes may be 0 for kinds that do
// not reserve a size. It is a no-op unless stats are enabled.
func recordResource(kind ResourceKind, label string, bytes uint64, d time.Duration) {
	if !statsEnabled.Load() {
		return
	}
	if kind < 0 || kind >= numResourceKinds {
		return
	}
	c := &resourceStats[kind]
	c.count.Add(1)
	c.nanos.Add(int64(d))
	c.bytes.Add(int64(bytes)) //nolint:gosec // sizes are bounded by device limits

	if label == "" {
		label = "(unlabeled)"
	}
	key := kind.String() + ":" + label
	labelMu.Lock()
	labelCounts[key]++
	labelMu.Unlock()
}

// ResourceStat is one kind's totals in a snapshot.
type ResourceStat struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
	Nanos int64  `json:"nanos"`
	Bytes int64  `json:"bytes"`
}

// Duration returns the total time spent creating resources of this kind.
func (r ResourceStat) Duration() time.Duration { return time.Duration(r.Nanos) }

// StatsSnapshot is a point-in-time copy of the counters. Snapshots subtract,
// so a caller can measure a specific window (one frame, one phase) rather than
// only process totals.
type StatsSnapshot struct {
	Kinds  []ResourceStat   `json:"kinds"`
	Labels map[string]int64 `json:"labels,omitempty"`
}

// Snapshot copies the current counters.
func Snapshot() StatsSnapshot {
	s := StatsSnapshot{Kinds: make([]ResourceStat, 0, numResourceKinds)}
	for k := ResourceKind(0); k < numResourceKinds; k++ {
		c := &resourceStats[k]
		s.Kinds = append(s.Kinds, ResourceStat{
			Kind:  k.String(),
			Count: c.count.Load(),
			Nanos: c.nanos.Load(),
			Bytes: c.bytes.Load(),
		})
	}
	labelMu.Lock()
	if len(labelCounts) > 0 {
		s.Labels = make(map[string]int64, len(labelCounts))
		for k, v := range labelCounts {
			s.Labels[k] = v
		}
	}
	labelMu.Unlock()
	return s
}

// Sub returns the resources created between an earlier snapshot and this one.
// Label counts are differenced too, so labels that did not change drop out --
// what remains is exactly what the window created.
func (s StatsSnapshot) Sub(earlier StatsSnapshot) StatsSnapshot {
	out := StatsSnapshot{Kinds: make([]ResourceStat, len(s.Kinds))}
	prev := make(map[string]ResourceStat, len(earlier.Kinds))
	for _, r := range earlier.Kinds {
		prev[r.Kind] = r
	}
	for i, r := range s.Kinds {
		p := prev[r.Kind]
		out.Kinds[i] = ResourceStat{
			Kind:  r.Kind,
			Count: r.Count - p.Count,
			Nanos: r.Nanos - p.Nanos,
			Bytes: r.Bytes - p.Bytes,
		}
	}
	for k, v := range s.Labels {
		if d := v - earlier.Labels[k]; d != 0 {
			if out.Labels == nil {
				out.Labels = map[string]int64{}
			}
			out.Labels[k] = d
		}
	}
	return out
}

// Total returns the summed count across every kind.
func (s StatsSnapshot) Total() int64 {
	var n int64
	for _, r := range s.Kinds {
		n += r.Count
	}
	return n
}

// Get returns one kind's stat.
func (s StatsSnapshot) Get(kind ResourceKind) ResourceStat {
	name := kind.String()
	for _, r := range s.Kinds {
		if r.Kind == name {
			return r
		}
	}
	return ResourceStat{Kind: name}
}

// SortedLabels returns label counts ordered by count descending, then name, so
// output is stable across runs.
func (s StatsSnapshot) SortedLabels() []struct {
	Label string
	Count int64
} {
	out := make([]struct {
		Label string
		Count int64
	}, 0, len(s.Labels))
	for k, v := range s.Labels {
		out = append(out, struct {
			Label string
			Count int64
		}{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// JSON renders the snapshot for tooling and dashboards.
func (s StatsSnapshot) JSON() ([]byte, error) { return json.MarshalIndent(s, "", "  ") }

// ResetStats zeroes every counter and clears label tracking.
func ResetStats() {
	for k := range resourceStats {
		resourceStats[k].count.Store(0)
		resourceStats[k].nanos.Store(0)
		resourceStats[k].bytes.Store(0)
	}
	labelMu.Lock()
	labelCounts = map[string]int64{}
	labelMu.Unlock()
}
