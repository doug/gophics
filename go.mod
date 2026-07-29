module github.com/doug/gossamer

go 1.26.5

require (
	github.com/go-text/typesetting v0.3.4
	github.com/gogpu/gg v0.50.7
	github.com/gogpu/gogpu v0.44.10
	github.com/gogpu/gpucontext v0.21.1
	github.com/gogpu/gputypes v0.5.1
	github.com/gogpu/wgpu v0.30.22
	golang.org/x/image v0.44.0
	golang.org/x/sys v0.47.0
)

// Local forks (github.com/doug/*) of the gogpu ecosystem — we maintain our
// own versions to unblock the GPU rasterization backend (readback + per-
// context accelerator opt-in). Paths are relative to this module.
replace (
	github.com/gogpu/gg => ../third_party/gg
	github.com/gogpu/gogpu => ../third_party/gogpu
	github.com/gogpu/gpucontext => ../third_party/gpucontext
	github.com/gogpu/gputypes => ../third_party/gputypes
	github.com/gogpu/naga => ../third_party/naga
	github.com/gogpu/wgpu => ../third_party/wgpu
)

require (
	github.com/go-webgpu/goffi v0.6.1 // indirect
	github.com/go-webgpu/webgpu v0.5.3 // indirect
	github.com/gogpu/naga v0.17.15 // indirect
	golang.org/x/mobile v0.0.0-20260709172247-6129f5bee9d5 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
)

tool golang.org/x/mobile/cmd/gobind
