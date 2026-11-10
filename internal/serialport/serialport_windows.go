//go:build windows

package serialport

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// maxEnumWalk 设备枚举树遍历的防御性上限，避免极端环境下注册表过大拖慢接口。
const maxEnumWalk = 20000

// comRe 从 FriendlyName 中提取 "(COMx)" 后缀。
var comRe = regexp.MustCompile(`\((COM\d+)\)\s*$`)

// List 枚举 Windows 串口：
//  1. HKLM\HARDWARE\DEVICEMAP\SERIALCOMM 是「当前在线」串口的权威清单（只有 COM 名）；
//  2. 设备枚举树里的 FriendlyName 形如 "USB-SERIAL CH340 (COM8)"，用来补充设备描述。
//     Enum 树会保留已拔掉设备的记录，因此只把它当命名来源，不当发现来源——
//     仅当 SERIALCOMM 完全读不到时才退化为全部收录。
func List() ([]Port, error) {
	present := map[string]bool{}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DEVICEMAP\SERIALCOMM`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if items, e := k.ReadValueNames(-1); e == nil {
			for _, item := range items {
				if v, _, e := k.GetStringValue(item); e == nil && v != "" {
					present[strings.ToUpper(v)] = true
				}
			}
		}
	}

	names := map[string]string{}
	for com := range present {
		names[com] = ""
	}
	walkEnum(func(friendly string) {
		m := comRe.FindStringSubmatch(friendly)
		if m == nil {
			return
		}
		com := strings.ToUpper(m[1])
		if len(present) > 0 && !present[com] {
			return // 已拔掉设备的历史记录，跳过
		}
		desc := strings.TrimSpace(friendly[:strings.Index(friendly, "(")])
		if desc != "" {
			names[com] = desc
		}
	})

	ports := make([]Port, 0, len(names))
	for com, desc := range names {
		ports = append(ports, Port{Path: com, Name: desc})
	}
	sort.Slice(ports, func(i, j int) bool { return comOrder(ports[i].Path) < comOrder(ports[j].Path) })
	return ports, nil
}

// comOrder 按 COM 编号自然排序（COM2 < COM10）。
func comOrder(path string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(path, "COM"))
	return n
}

// walkEnum 遍历 HKLM\SYSTEM\CurrentControlSet\Enum\<枚举器>\<设备ID>\<实例>，
// 对每条 FriendlyName 调用 fn。FriendlyName 缺失时退化为 DeviceDesc。
func walkEnum(fn func(friendly string)) {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	defer root.Close()

	walked := 0
	for _, enumName := range subKeys(root) {
		enumKey, err := registry.OpenKey(root, enumName, registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		for _, devID := range subKeys(enumKey) {
			devKey, err := registry.OpenKey(enumKey, devID, registry.ENUMERATE_SUB_KEYS)
			if err != nil {
				continue
			}
			for _, inst := range subKeys(devKey) {
				walked++
				if walked > maxEnumWalk {
					devKey.Close()
					enumKey.Close()
					return
				}
				instKey, err := registry.OpenKey(devKey, inst, registry.QUERY_VALUE)
				if err != nil {
					continue
				}
				desc, _, _ := instKey.GetStringValue("FriendlyName")
				if desc == "" {
					// DeviceDesc 形如 "@oem39.inf,%ch340%;USB-SERIAL CH340"，去掉 inf 前缀
					if d, _, _ := instKey.GetStringValue("DeviceDesc"); d != "" && strings.HasPrefix(d, "@") {
						if idx := strings.Index(d, ";"); idx >= 0 {
							d = d[idx+1:]
						}
						desc = d
					}
				}
				_ = instKey.Close()
				if desc != "" {
					fn(desc)
				}
			}
			_ = devKey.Close()
		}
		_ = enumKey.Close()
	}
}

func subKeys(k registry.Key) []string {
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	return names
}
