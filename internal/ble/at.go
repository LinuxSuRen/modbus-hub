package ble

import (
	"fmt"
	"strings"
	"time"
)

// SendAT 临时连接透传模块，发送一条 AT 指令并收集应答。
//   - cmd：AT 指令正文（不带行结束符），如 "AT+NAME"；
//   - terminator：行结束符，支持 "\r\n"、"\r"、"\n"；
//   - timeout：发送后等待应答的总时长；期间收满数据且已出现 OK/ERROR/+AT
//     等标志性回包时提前返回，否则等到超时返回全部已收数据。
//
// 返回应答的文本形式（ASCII，不可打印字符替换为 '.'）。
func SendAT(addr, service, write, notify, cmd, terminator string, timeout time.Duration) (string, error) {
	if cmd == "" {
		return "", fmt.Errorf("AT 指令不能为空")
	}
	switch terminator {
	case "":
		terminator = "\r\n"
	case "\\r\\n":
		terminator = "\r\n"
	case "\\r":
		terminator = "\r"
	case "\\n":
		terminator = "\n"
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	link, err := Dial(addr, service, write, notify, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer link.Close()

	link.Discard()
	if err := link.Write([]byte(cmd + terminator)); err != nil {
		return "", err
	}
	// AT 应答短，先读一小段再视内容决定是否继续等
	deadline := time.Now().Add(timeout)
	buf, rerr := link.ReadUntil(1, deadline) // 至少等到 1 字节，剩余按内容补读
	if len(buf) == 0 && rerr != nil {
		return "", fmt.Errorf("模块无应答: %v", rerr)
	}
	// 继续把后续到达的字节收进来（最多再等 300ms 静默窗口）
	text := ascii(buf)
	if !settled(text) {
		for time.Now().Before(deadline) {
			more, _ := link.ReadUntil(len(buf)+32, time.Now().Add(300*time.Millisecond))
			if len(more) <= len(buf) {
				break
			}
			buf = more
			text = ascii(buf)
			if settled(text) {
				break
			}
		}
	}
	return strings.TrimSpace(text), nil
}

// settled 判断 AT 应答是否已出现标志性结束。
func settled(text string) bool {
	return strings.Contains(text, "OK") || strings.Contains(text, "ERROR") ||
		strings.Contains(text, "+AT") || strings.Contains(text, "+EVENT")
}

// ascii 把字节转成可打印文本。
func ascii(b []byte) string {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 0x20 && c < 0x7f {
			out[i] = c
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}
