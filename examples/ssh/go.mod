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
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.6.1 // indirect
	github.com/go-webgpu/webgpu v0.5.3 // indirect
	github.com/gogpu/gg v0.50.7 // indirect
	github.com/gogpu/gogpu v0.44.10 // indirect
	github.com/gogpu/gpucontext v0.21.1 // indirect
	github.com/gogpu/gputypes v0.5.1 // indirect
	github.com/gogpu/naga v0.17.15 // indirect
	github.com/gogpu/wgpu v0.30.22 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace github.com/doug/gossamer => ../..

replace (
	github.com/gogpu/gg => ../../../third_party/gg
	github.com/gogpu/gogpu => ../../../third_party/gogpu
	github.com/gogpu/gpucontext => ../../../third_party/gpucontext
	github.com/gogpu/gputypes => ../../../third_party/gputypes
	github.com/gogpu/naga => ../../../third_party/naga
	github.com/gogpu/wgpu => ../../../third_party/wgpu
)
