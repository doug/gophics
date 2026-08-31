//go:build !js

package desktop

import (
	"reflect"
	"testing"

	"github.com/doug/gophics/shell"
)

// Every field of a shell node must survive the copy to the windowing layer's.
//
// The two structs are field-identical by design, which is what makes this copy
// dangerous: a field added to both and forgotten in the middle reads as
// correct. Expandable and Expanded were dropped exactly that way, so a tree on
// desktop announced no open/closed state while web announced it correctly —
// the kind of gap that survives because one platform demonstrably works.
//
// The check is reflective rather than a list, so a field added later fails here
// until it is carried, instead of being silently dropped like the last two.
func TestPlatformNodeCarriesEveryField(t *testing.T) {
	// A node with every field set to a non-zero value, built reflectively so
	// adding a field to shell.A11yNode cannot leave this fixture stale.
	var in shell.A11yNode
	rv := reflect.ValueOf(&in).Elem()
	for i := range rv.NumField() {
		f := rv.Field(i)
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(int64(i + 1))
		case reflect.String:
			f.SetString(rv.Type().Field(i).Name)
		}
	}

	out := toPlatformNodes([]shell.A11yNode{in})
	if len(out) != 1 {
		t.Fatalf("converted %d nodes, want 1", len(out))
	}

	// Fields the platform node deliberately does not mirror. An entry here is a
	// decision on the record; without one, a missing field fails the test.
	exempt := map[string]string{
		"Children": "the platform node reconstructs hierarchy from ParentID, " +
			"which every backend's tree API wants anyway — carrying both would " +
			"be two sources of truth for the same edges",
	}

	ov := reflect.ValueOf(out[0])
	for i := range rv.NumField() {
		name := rv.Type().Field(i).Name
		if _, ok := exempt[name]; ok {
			continue
		}
		of := ov.FieldByName(name)
		if !of.IsValid() {
			t.Errorf("shell.A11yNode.%s has no counterpart on the platform node", name)
			continue
		}
		if !reflect.DeepEqual(rv.Field(i).Interface(), of.Interface()) {
			t.Errorf("%s dropped or altered in the copy: shell has %v, platform has %v",
				name, rv.Field(i).Interface(), of.Interface())
		}
	}
}
