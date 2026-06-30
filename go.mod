module github.com/termizard/termizard

go 1.26.1

replace github.com/gogpu/gogpu => /Users/vladimirzikman/Workspace/opensource/gogpu-projects/gogpu-local

replace github.com/gogpu/gg => /Users/vladimirzikman/Workspace/opensource/gogpu-projects/gg-local

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/creack/pty v1.1.24
	github.com/gogpu/gg v0.49.2
	github.com/gogpu/gogpu v0.42.11
	github.com/gogpu/gpucontext v0.21.0
	golang.org/x/sys v0.46.0
)

require (
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.5.5 // indirect
	github.com/go-webgpu/webgpu v0.5.2 // indirect
	github.com/gogpu/gputypes v0.5.1 // indirect
	github.com/gogpu/naga v0.17.15 // indirect
	github.com/gogpu/wgpu v0.30.7 // indirect
	golang.org/x/image v0.41.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)
