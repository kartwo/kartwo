// 隔离SMTP协议收件箱 / Isolated SMTP Protocol Mailbox
// 功能：仅绑定本机、只接收虚构地址，记录协议投递并模拟DATA确认丢失；绝不转发邮件
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 20:39:31
package admin

import (
	"bufio"
	"bytes"
	"io"
	"net"
	stdmail "net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

type deliveryMessage struct{ Subject, Body string }
type deliverySMTP struct {
	listener    net.Listener
	mu          sync.Mutex
	connections map[net.Conn]bool
	messages    []deliveryMessage
	dropNext    bool
	wait        sync.WaitGroup
}

func newDeliverySMTP(t *testing.T) *deliverySMTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &deliverySMTP{listener: listener, connections: map[net.Conn]bool{}}
	server.wait.Add(1)
	go func() {
		defer server.wait.Done()
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			server.mu.Lock()
			server.connections[connection] = true
			server.mu.Unlock()
			server.wait.Add(1)
			go func() { defer server.wait.Done(); server.serve(connection) }()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		server.mu.Lock()
		for connection := range server.connections {
			_ = connection.Close()
		}
		server.mu.Unlock()
		server.wait.Wait()
	})
	return server
}
func (s *deliverySMTP) serve(connection net.Conn) {
	defer func() { _ = connection.Close(); s.mu.Lock(); delete(s.connections, connection); s.mu.Unlock() }()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	reader := textproto.NewReader(bufio.NewReader(connection))
	reply := func(text string) bool { _, err := io.WriteString(connection, text+"\r\n"); return err == nil }
	if !reply("220 isolated-fixture ESMTP") {
		return
	}
	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}
		switch {
		case strings.HasPrefix(line, "EHLO "):
			if !reply("250-isolated-fixture\r\n250 HELP") {
				return
			}
		case strings.HasPrefix(line, "HELO "), strings.HasPrefix(line, "MAIL FROM:"):
			if !reply("250 OK") {
				return
			}
		case strings.HasPrefix(line, "RCPT TO:"):
			// Even an accidental configuration change cannot forward to a real recipient.
			if line != "RCPT TO:<buyer-delivery@example.invalid>" {
				if !reply("550 fixture recipient only") {
					return
				}
			} else {
				if !reply("250 OK") {
					return
				}
			}
		case line == "DATA":
			if !reply("354 End with dot") {
				return
			}
			payload, err := reader.ReadDotBytes()
			if err != nil {
				return
			}
			message, err := stdmail.ReadMessage(bufio.NewReader(bytes.NewReader(payload)))
			if err != nil {
				return
			}
			body, err := io.ReadAll(message.Body)
			if err != nil {
				return
			}
			s.mu.Lock()
			s.messages = append(s.messages, deliveryMessage{Subject: message.Header.Get("Subject"), Body: string(body)})
			drop := s.dropNext
			s.dropNext = false
			s.mu.Unlock()
			if drop {
				return
			} // The receiver kept DATA; the sender did not get its acknowledgement.
			if !reply("250 fixture accepted") {
				return
			}
		case line == "QUIT":
			_ = reply("221 Goodbye")
			return
		default:
			if !reply("500 unsupported fixture command") {
				return
			}
		}
	}
}
func (s *deliverySMTP) snapshot() []deliveryMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]deliveryMessage(nil), s.messages...)
}
func (s *deliverySMTP) dropNextAcknowledgement() { s.mu.Lock(); s.dropNext = true; s.mu.Unlock() }
