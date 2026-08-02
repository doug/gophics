// Separate module so the SSH server dependency (gliderlabs/ssh) stays out of
// gossamer core. It replaces gossamer and the gogpu forks with the local
// checkouts, mirroring the parent module's replace directives.
module gossamer-ssh-example

go 1.26.5

require (
	github.com/doug/gossamer v0.0.0
	github.com/gliderlabs/ssh v0.3.8
	golang.org/x/crypto v0.31.0
	golang.org/x/image v0.44.0
)

require (
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/doug/gg v0.51.0 // indirect
	github.com/doug/gogpu v0.45.0 // indirect
	github.com/doug/gpucontext v0.22.0 // indirect
	github.com/doug/gputypes v0.6.0 // indirect
	github.com/doug/naga v0.18.0 // indirect
	github.com/doug/wgpu v0.31.0 // indirect
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.6.1 // indirect
	github.com/go-webgpu/webgpu v0.5.3 // indirect
	github.com/gogpu/gputypes v0.5.1 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/doug/gossamer => ../..

replace (
	github.com/doug/gg => ../../../third_party/gg
	github.com/doug/gogpu => ../../../third_party/gogpu
	github.com/doug/gpucontext => ../../../third_party/gpucontext
	github.com/doug/gputypes => ../../../third_party/gputypes
	github.com/doug/naga => ../../../third_party/naga
	github.com/doug/wgpu => ../../../third_party/wgpu
)
