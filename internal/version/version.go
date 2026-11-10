// Package version 版本信息。
package version

import "runtime"

// Version 构建时经 -ldflags 注入。
var Version = "dev"

// Platform 当前运行平台。
func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
