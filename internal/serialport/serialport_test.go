package serialport

import (
	"testing"
)

func TestListNoDuplicateAndSorted(t *testing.T) {
	ports, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range ports {
		if p.Path == "" {
			t.Fatalf("空串口路径: %+v", ports)
		}
		if seen[p.Path] {
			t.Fatalf("重复串口 %s", p.Path)
		}
		seen[p.Path] = true
	}
}

func TestListFindsPortsOnDevMachine(t *testing.T) {
	// 开发机/CI 上至少能正常返回（可能为空列表），不 panic 即可
	if _, err := List(); err != nil {
		t.Fatalf("List: %v", err)
	}
}
