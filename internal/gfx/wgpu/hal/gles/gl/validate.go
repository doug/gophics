//go:build (windows || linux) && !(js && wasm)

package gl

import (
	"fmt"
	"strings"
)

// requiredEntryPoints are the GL functions the backend cannot render without.
//
// All are core in OpenGL 3.3 and ES 3.0, which is the floor the adapter already
// enforces. Listing them anyway is the point: a driver can advertise 3.3 and
// still fail to resolve a name — Windows' "GDI Generic" software path does
// exactly that, and Mesa's d3d12 gallium returns stubs for desktop names on an
// ES context.
//
// Extension and version-dependent functions are deliberately absent. Those are
// optional by nature and their call sites test for nil; this list is the set
// whose absence means "cannot draw at all".
func (c *Context) requiredEntryPoints() []struct {
	name string
	ptr  uintptr
} {
	return []struct {
		name string
		ptr  uintptr
	}{
		// State and queries.
		{"glGetError", ep(c.glGetError)},
		{"glGetString", ep(c.glGetString)},
		{"glGetIntegerv", ep(c.glGetIntegerv)},
		{"glEnable", ep(c.glEnable)},
		{"glDisable", ep(c.glDisable)},
		{"glViewport", ep(c.glViewport)},
		{"glScissor", ep(c.glScissor)},
		{"glClear", ep(c.glClear)},
		{"glClearColor", ep(c.glClearColor)},
		// Drawing.
		{"glDrawArrays", ep(c.glDrawArrays)},
		{"glDrawElements", ep(c.glDrawElements)},
		// Shaders and programs.
		{"glCreateShader", ep(c.glCreateShader)},
		{"glShaderSource", ep(c.glShaderSource)},
		{"glCompileShader", ep(c.glCompileShader)},
		{"glCreateProgram", ep(c.glCreateProgram)},
		{"glAttachShader", ep(c.glAttachShader)},
		{"glLinkProgram", ep(c.glLinkProgram)},
		{"glUseProgram", ep(c.glUseProgram)},
		// Buffers.
		{"glGenBuffers", ep(c.glGenBuffers)},
		{"glBindBuffer", ep(c.glBindBuffer)},
		{"glBufferData", ep(c.glBufferData)},
		// Vertex array objects — GL 3.0+, and the first thing the device
		// creates. This is the call that crashed at PC=0 on a 1.1 context.
		{"glGenVertexArrays", ep(c.glGenVertexArrays)},
		{"glBindVertexArray", ep(c.glBindVertexArray)},
		{"glVertexAttribPointer", ep(c.glVertexAttribPointer)},
		{"glEnableVertexAttribArray", ep(c.glEnableVertexAttribArray)},
		// Textures.
		{"glGenTextures", ep(c.glGenTextures)},
		{"glBindTexture", ep(c.glBindTexture)},
		{"glTexImage2D", ep(c.glTexImage2D)},
		{"glTexParameteri", ep(c.glTexParameteri)},
		// Framebuffers — GL 3.0+.
		{"glGenFramebuffers", ep(c.glGenFramebuffers)},
		{"glBindFramebuffer", ep(c.glBindFramebuffer)},
		{"glFramebufferTexture2D", ep(c.glFramebufferTexture2D)},
	}
}

// Validate reports which required entry points failed to resolve.
//
// Load fills 126 function pointers and, before this existed, checked none of
// them: a name the driver does not export silently became 0 and Load returned
// success. The failure then surfaced at the first call as an access violation
// with PC=0 and an empty stack, which is close to undebuggable — it was found
// by running a gophics app in a VM and reading a crash dump.
//
// Turning that into an error at load time lets the caller decline the adapter
// and fall back to CPU rendering, which is what a machine without a graphics
// driver should get.
func (c *Context) Validate() error {
	var missing []string
	for _, e := range c.requiredEntryPoints() {
		if e.ptr == 0 {
			missing = append(missing, e.name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("gl: %d required entry points missing: %s",
		len(missing), strings.Join(missing, ", "))
}
