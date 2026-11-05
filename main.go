package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/linuxsuren/modbus-hub/internal/httpserver"
	"github.com/linuxsuren/modbus-hub/internal/modbus"
	"github.com/linuxsuren/modbus-hub/internal/store"
	"github.com/linuxsuren/modbus-hub/internal/version"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP 监听地址")
	dataDir := flag.String("data", "./data", "数据目录（SQLite）")
	showVersion := flag.Bool("version", false, "打印版本")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version, version.Platform())
		return
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	db, err := store.Open(*dataDir)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}

	mgr := modbus.NewManager(db)
	mgr.Reload()

	srv, err := httpserver.NewServer(db, mgr)
	if err != nil && srv == nil {
		log.Fatalf("初始化服务失败: %v", err)
	}
	log.Printf("modbus-hub %s 启动，监听 %s，数据目录 %s",
		version.Version, *addr, filepath.Clean(*dataDir))
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("HTTP 服务退出: %v", err)
	}
}
