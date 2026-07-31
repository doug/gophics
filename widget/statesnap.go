package widget

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// State snapshot & restore — the substrate for state-preserving hot-restart.
//
// The idea (spike): treat UI state as plain, JSON-serializable data. Before a
// rebuild we walk the element tree and serialize each State, keyed by its
// location in the tree (widget type + sibling position + reconciliation key —
// the very coordinate the reconciler already uses for identity). After the
// fresh tree mounts, we walk it again and decode each saved blob back into the
// State sitting at the same location. You land exactly where you were: same
// page, same scroll, same counter, same field contents.
//
// Location keying is best-effort, like React Fast Refresh: an edit that
// restructures the tree *above* a stateful widget can move it to a new path and
// drop its state. Attach a WithKey to make identity survive reorders.
//
// Two ways a State opts in:
//
//   - Implement Snapshottable to expose an explicit, encapsulated DTO
//     (recommended for framework widgets whose fields stay unexported).
//   - Or simply give the State exported fields — plain json.Marshal then
//     captures them with zero extra code (the "state is data" path).
//
// A State that does neither (no exported fields, not Snapshottable) is skipped:
// it serializes to nothing, so it's transient by construction.

// Snapshottable is implemented by State that persists across a rebuild via an
// explicit serializable value, keeping the State's own fields encapsulated.
type Snapshottable interface {
	// SaveState returns a JSON-serializable snapshot of this State (a plain
	// struct is ideal). Return nil to persist nothing.
	SaveState() any
	// LoadState decodes data (produced by an earlier SaveState) into this
	// freshly created State. Called at most once, before the next build.
	LoadState(data json.RawMessage)
}

// StateSnapshot maps element paths to serialized State. It is itself plain JSON:
// marshal it to a file before a hot-restart, read it back in the new process,
// and pass it to RestoreState.
type StateSnapshot map[string]json.RawMessage

// SnapshotState walks the live tree and captures every State that serializes to
// something, keyed by tree location.
func (o *Owner) SnapshotState() StateSnapshot {
	out := StateSnapshot{}
	if o.root != nil {
		o.root.walkSnapshot(rootPath(o.root), out)
	}
	return out
}

// RestoreState decodes snap into the States at matching locations, then
// rebuilds so the restored values flow into the view. Call it right after
// mounting the fresh root.
func (o *Owner) RestoreState(snap StateSnapshot) {
	if o.root == nil || len(snap) == 0 {
		return
	}
	o.root.walkRestore(rootPath(o.root), snap)
	o.RebuildAll()
	o.FlushBuilds()
}

func rootPath(root *element) string { return "/" + typeName(root.widget) }

func (el *element) walkSnapshot(path string, out StateSnapshot) {
	if el.state != nil {
		if data, ok := marshalState(el.state); ok {
			out[path] = data
		}
	}
	kids := el.childElements()
	segs := siblingSegments(kids)
	for i, c := range kids {
		c.walkSnapshot(path+"/"+segs[i], out)
	}
}

func (el *element) walkRestore(path string, snap StateSnapshot) {
	if el.state != nil {
		if data, ok := snap[path]; ok {
			unmarshalState(el.state, data)
		}
	}
	kids := el.childElements()
	segs := siblingSegments(kids)
	for i, c := range kids {
		c.walkRestore(path+"/"+segs[i], snap)
	}
}

// childElements returns el's reconciliation children uniformly: a composite has
// one build child; a render widget has its positional kids.
func (el *element) childElements() []*element {
	if el.child != nil {
		return []*element{el.child}
	}
	return el.kids
}

// siblingSegments assigns each sibling a path segment that is stable across
// rebuilds: keyed siblings by "type#key", unkeyed by "type:index" (index
// counted per type, so a lone child of a type is just "type:0").
func siblingSegments(sibs []*element) []string {
	counts := map[string]int{}
	segs := make([]string, len(sibs))
	for i, s := range sibs {
		name := typeName(s.widget)
		if k := keyOf(s.widget); k != nil {
			segs[i] = name + "#" + fmt.Sprint(k)
			continue
		}
		segs[i] = name + ":" + strconv.Itoa(counts[name])
		counts[name]++
	}
	return segs
}

func typeName(w Widget) string {
	t := reflect.TypeOf(w)
	if t == nil {
		return "nil"
	}
	return t.String()
}

// marshalState serializes a State: via Snapshottable if implemented, else via
// its exported fields. Returns ok=false when there is nothing to persist.
func marshalState(s State) (json.RawMessage, bool) {
	if snap, ok := s.(Snapshottable); ok {
		v := snap.SaveState()
		if v == nil {
			return nil, false
		}
		data, err := json.Marshal(v)
		return data, err == nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, false
	}
	switch string(data) {
	case "{}", "null", "":
		return nil, false // no exported state — transient by construction
	}
	return data, true
}

func unmarshalState(s State, data json.RawMessage) {
	if snap, ok := s.(Snapshottable); ok {
		snap.LoadState(data)
		return
	}
	_ = json.Unmarshal(data, s) // into the State pointer: sets exported fields
}
