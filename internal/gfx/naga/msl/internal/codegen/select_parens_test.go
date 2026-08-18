package codegen

import (
	"strings"
	"testing"
)

// TestSelectAsBinaryOperandIsParenthesised guards a silent miscompilation.
//
// MSL's `?:` binds more loosely than every arithmetic and comparison operator,
// so a select used as a binary operand must be parenthesised. It was not:
// needsParens — the helper binary operands go through — listed ExprBinary and
// ExprArrayLength but not ExprSelect, while its sibling needsParensInContext
// did list it.
//
// The result compiled. `a + select(x, y, c)` came out as `a + c ? x : y`, which
// C++ groups as `(a + c) ? x : y` — a different number, from a shader Metal
// accepts with at most a warning. Every WGSL shader doing arithmetic on a
// select was affected, which in this tree meant the vello flatten stage.
func TestSelectAsBinaryOperandIsParenthesised(t *testing.T) {
	src := `
@group(0) @binding(0) var<storage, read_write> out: array<f32>;

@compute @workgroup_size(1)
fn cs_main(@builtin(global_invocation_id) gid: vec3<u32>) {
    let positive = out[0] > 0.0;
    let base = out[1] * 2.0;
    out[gid.x] = base + select(-1.0, 0.0, positive);
}
`
	code := compileWGSL(t, src)

	// The generated line must not leave the ternary bare after the '+'.
	for _, line := range strings.Split(code, "\n") {
		plus := strings.Index(line, "+")
		q := strings.Index(line, "?")
		if plus < 0 || q < plus {
			continue
		}
		// Between the '+' and the '?' there must be an opening paren that
		// starts the ternary, rather than the condition sitting bare.
		between := line[plus+1 : q]
		if !strings.Contains(between, "(") {
			t.Errorf("select used as a binary operand is not parenthesised:\n  %s", strings.TrimSpace(line))
		}
	}
}

// TestSelectAtStatementLevelIsNotParenthesised pins the other half: Rust naga
// emits no outer parens when the select is the whole right-hand side, and the
// reference-fidelity snapshots compare against that output. Wrapping
// unconditionally would be semantically fine and would still diverge.
func TestSelectAtStatementLevelIsNotParenthesised(t *testing.T) {
	src := `
@group(0) @binding(0) var<storage, read_write> out: array<f32>;

@compute @workgroup_size(1)
fn cs_main() {
    let positive = out[0] > 0.0;
    out[0] = select(-1.0, 0.0, positive);
}
`
	code := compileWGSL(t, src)
	if strings.Contains(code, "= ((") {
		t.Errorf("statement-level select gained redundant outer parens, diverging from the Rust reference:\n%s", code)
	}
}
