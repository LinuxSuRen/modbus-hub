package sink

import (
	"bufio"
	"log"
	"net"
	"sync"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/model"
)

// TCPServer 本机 TCP 服务端：把每条读数以 JSON 行推给所有已连接客户端；
// 新客户端接入时先补发一遍最新快照。慢客户端写超时即断开，绝不拖垮采集。
type TCPServer struct {
	listen string

	mu      sync.Mutex
	clients map[net.Conn]*bufio.Writer
	ln      net.Listener

	stop chan struct{}
	once sync.Once
}

// NewTCPServer 创建 TCP JSON 行流服务。
func NewTCPServer(listen string) *TCPServer {
	return &TCPServer{listen: listen, clients: make(map[net.Conn]*bufio.Writer), stop: make(chan struct{})}
}

// Start 开始监听并接受连接。
func (s *TCPServer) Start() error {
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		return err
	}
	s.ln = ln
	go s.acceptLoop()
	log.Printf("[tcp] JSON 行流已监听 %s", s.listen)
	return nil
}

func (s *TCPServer) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.stop:
				return
			default:
				log.Printf("[tcp] 接受连接失败: %v", err)
				continue
			}
		}
		go s.serveConn(conn)
	}
}

func (s *TCPServer) serveConn(conn net.Conn) {
	defer func() {
		s.remove(conn)
		_ = conn.Close()
	}()
	w := bufio.NewWriter(conn)
	s.mu.Lock()
	s.clients[conn] = w
	s.mu.Unlock()
	// 读侧只为感知对端断开
	buf := make([]byte, 64)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(time.Minute))
		if _, err := conn.Read(buf); err != nil {
			return
		}
	}
}

// Handle 实现 Manager 订阅回调：广播一条 JSON 行。
func (s *TCPServer) Handle(r model.Reading) {
	line, err := jsonMarshal(r)
	if err != nil {
		return
	}
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	for conn, w := range s.clients {
		_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if _, err := w.Write(line); err != nil || w.Flush() != nil {
			_ = conn.Close() // 触发 serveConn 退出并从列表移除
			delete(s.clients, conn)
		}
	}
}

// Snapshot 向所有客户端补发一组最新读数。
func (s *TCPServer) Snapshot(readings []model.Reading) {
	for _, r := range readings {
		s.Handle(r)
	}
}

// ClientCount 供状态展示。
func (s *TCPServer) ClientCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

func (s *TCPServer) remove(conn net.Conn) {
	s.mu.Lock()
	delete(s.clients, conn)
	s.mu.Unlock()
}

// Stop 关闭监听与全部客户端连接。
func (s *TCPServer) Stop() {
	s.once.Do(func() { close(s.stop) })
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.mu.Lock()
	for conn := range s.clients {
		_ = conn.Close()
		delete(s.clients, conn)
	}
	s.mu.Unlock()
}
