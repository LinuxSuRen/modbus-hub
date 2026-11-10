// Package web 内嵌前端构建产物。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist 返回前端构建产物文件系统。
func Dist() fs.FS { return distFS }
