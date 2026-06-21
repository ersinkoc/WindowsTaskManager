module github.com/ersinkoc/WindowsTaskManager

go 1.25.0

toolchain go1.26.4

require (
	github.com/ersinkoc/WindowsTaskManager/frontend v0.0.0
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	golang.org/x/sys v0.43.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect

replace github.com/ersinkoc/WindowsTaskManager/frontend => ./frontend
