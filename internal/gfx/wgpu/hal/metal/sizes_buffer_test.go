package metal

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/naga"
	"github.com/doug/gophics/internal/gfx/naga/msl"
)

// TestResolveSizeGlobalsMatchesNagaOrder checks that the HAL locates each
// _mslBufferSizes member at the binding naga expects, in naga's order.
//
// This is the join nobody can see at runtime. naga emits one uint per
// runtime-sized-array global, in its own order; the HAL fills that struct from
// the lengths of bound buffers. If the two disagree about which member is
// which, every bounds check silently compares an index against the wrong
// array's length — and a bounds check reading the wrong number behaves exactly
// like one reading garbage, which is the bug this whole mechanism was built to
// end.
//
// Deliberately uses two bind groups and a fixed-size array alongside the
// runtime-sized ones, so the test would catch a resolution that ignored the
// group, or that counted arrays which need no length entry.
func TestResolveSizeGlobalsMatchesNagaOrder(t *testing.T) {
	const src = `
struct Cfg { n: u32 }
@group(0) @binding(0) var<uniform> cfg: Cfg;
@group(0) @binding(1) var<storage, read> a: array<u32>;
@group(0) @binding(2) var<storage, read> fixed: array<u32, 8>;
@group(1) @binding(0) var<storage, read_write> b: array<f32>;

@compute @workgroup_size(1)
fn cs(@builtin(global_invocation_id) gid: vec3<u32>) {
    b[gid.x] = f32(a[gid.x] + fixed[0] + cfg.n);
}
`
	ast, err := naga.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	module, err := naga.LowerWithSource(ast, src)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	slot := uint8(mslSizesBufferSlot)
	opts := msl.DefaultOptions()
	opts.SizesBufferSlot = &slot
	_, info, err := msl.Compile(module, opts)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if !info.RequiresSizesBuffer {
		t.Fatal("shader has runtime-sized arrays but naga reports no sizes buffer needed")
	}

	got := resolveSizeGlobals(module, info)
	want := []sizeGlobalBinding{{group: 0, binding: 1}, {group: 1, binding: 0}}
	if len(got) != len(want) {
		t.Fatalf("resolved %d size globals, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("size global %d resolved to group %d binding %d, want group %d binding %d",
				i, got[i].group, got[i].binding, want[i].group, want[i].binding)
		}
	}
}
