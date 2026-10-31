// SMTP 投递边界测试 / SMTP Delivery Boundary Tests
// 功能：用本机 SMTP 协议桩验证接收确认、断线未知、明确拒绝、绝对超时与头部安全
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 14:16:42
package mail

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// The local fixture never forwards mail, authenticates to a provider or uses real recipients.
func localSMTP(t *testing.T, mode string) (Config, func() []string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mutex sync.Mutex
	commands := []string{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.WriteString(conn, "220 fixture ESMTP\r\n")
		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			mutex.Lock()
			commands = append(commands, line)
			mutex.Unlock()
			switch {
			case strings.HasPrefix(line, "EHLO "):
				_, _ = io.WriteString(conn, "250-fixture\r\n250 HELP\r\n")
			case strings.HasPrefix(line, "MAIL FROM:"):
				_, _ = io.WriteString(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "RCPT TO:"):
				if mode == "reject_rcpt" {
					_, _ = io.WriteString(conn, "550 fixture recipient rejected\r\n")
				} else {
					_, _ = io.WriteString(conn, "250 OK\r\n")
				}
			case line == "DATA":
				_, _ = io.WriteString(conn, "354 End with dot\r\n")
				for {
					body, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					if body == ".\r\n" {
						break
					}
				}
				if mode == "drop_data_ack" {
					return
				}
				reply := "250 fixture accepted\r\n"
				switch mode {
				case "reject_data":
					reply = "550 fixture content rejected\r\n"
				case "unexpected_data_reply":
					reply = "354 fixture unexpected continuation\r\n"
				case "alternate_accept_reply":
					reply = "251 fixture accepted\r\n"
				}
				_, _ = io.WriteString(conn, reply)
			case line == "QUIT":
				if mode != "drop_quit_ack" {
					_, _ = io.WriteString(conn, "221 Goodbye\r\n")
				}
				return
			default:
				_, _ = io.WriteString(conn, "500 fixture unknown command\r\n")
			}
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			t.Error("fixture SMTP did not stop")
		}
	})
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return Config{Host: host, Port: port, FromAddress: "shop@example.invalid", FromName: "Fixture Store", Encryption: "none"}, func() []string {
		mutex.Lock()
		defer mutex.Unlock()
		return append([]string{}, commands...)
	}
}

func TestSMTPDistinguishesAcceptanceRejectionAndUnknownDelivery(t *testing.T) {
	for _, test := range []struct {
		mode               string
		wantError, unknown bool
	}{
		{"accept", false, false}, {"drop_quit_ack", false, false}, {"alternate_accept_reply", false, false},
		{"drop_data_ack", true, true}, {"reject_data", true, false}, {"reject_rcpt", true, false}, {"unexpected_data_reply", true, true},
	} {
		t.Run(test.mode, func(t *testing.T) {
			cfg, transcript := localSMTP(t, test.mode)
			err := Send(cfg, "buyer@example.invalid", "Fixture subject", "Only local fixture text.\n")
			if (err != nil) != test.wantError || errors.Is(err, ErrDeliveryUnknown) != test.unknown {
				t.Fatalf("result=%v unknown=%v wantError=%v unknown=%v", err, errors.Is(err, ErrDeliveryUnknown), test.wantError, test.unknown)
			}
			if !test.wantError && !strings.Contains(strings.Join(transcript(), "\n"), "QUIT") {
				t.Fatal("accepted message did not finish the SMTP session")
			}
		})
	}
}

func TestSMTPDoesNotSilentlyDowngradeTLSOrAuthentication(t *testing.T) {
	for _, mode := range []string{"starttls", "auth"} {
		t.Run(mode, func(t *testing.T) {
			cfg, transcript := localSMTP(t, "accept")
			if mode == "starttls" {
				cfg.Encryption = "starttls"
			} else {
				cfg.Username, cfg.Password = "fixture-user", "fixture-password"
			}
			if err := Send(cfg, "buyer@example.invalid", "Fixture", "Fixture"); err == nil || errors.Is(err, ErrDeliveryUnknown) {
				t.Fatalf("missing security capability must reject before DATA: %v", err)
			}
			commands := strings.Join(transcript(), "\n")
			if strings.Contains(commands, "MAIL FROM:") || strings.Contains(commands, "RCPT TO:") || strings.Contains(commands, "DATA") {
				t.Fatal("security downgrade reached envelope or DATA")
			}
		})
	}
}

func TestSMTPRejectsHeaderInjectionBeforeAnyConnection(t *testing.T) {
	base := Config{Host: "127.0.0.1", Port: "0", FromAddress: "shop@example.invalid", Encryption: "none"}
	for _, test := range []struct{ to, from, name, subject string }{
		{"buyer@example.invalid\r\nBcc: other@example.invalid", base.FromAddress, "", "Fixture"},
		{"buyer@example.invalid", "shop@example.invalid\nBcc: other@example.invalid", "", "Fixture"},
		{"buyer@example.invalid", base.FromAddress, "Store\r\nBcc: other@example.invalid", "Fixture"},
		{"buyer@example.invalid", base.FromAddress, "", "Fixture\nBcc: other@example.invalid"},
		{"Buyer <buyer@example.invalid>", base.FromAddress, "", "Fixture"},
	} {
		cfg := base
		cfg.FromAddress, cfg.FromName = test.from, test.name
		if err := Send(cfg, test.to, test.subject, "Body"); !errors.Is(err, ErrInvalidMessage) {
			t.Fatalf("header injection or malformed mailbox reached dial: %v", err)
		}
	}
	message := string(buildMessage(base, "buyer@example.invalid", "Fixture", "one\r\ntwo\rthree\nfour"))
	if !strings.HasSuffix(message, "one\r\ntwo\r\nthree\r\nfour") || strings.Contains(message, "\r\r\n") {
		t.Fatal("body line endings are not canonical CRLF")
	}
}

func TestSMTPAbsoluteDeadlineCoversGreetingAndLaterCommands(t *testing.T) {
	for _, greeting := range []bool{false, true} {
		t.Run(fmt.Sprintf("greeting=%v", greeting), func(t *testing.T) {
			client, server := net.Pipe()
			defer func() { _ = server.Close() }()
			done := make(chan struct{})
			go func() {
				defer close(done)
				if greeting {
					_, _ = io.WriteString(server, "220 fixture ESMTP\r\n")
				}
				_, _ = io.Copy(io.Discard, server)
			}()
			start := time.Now()
			c, err := connectedSMTP(client, "localhost", time.Now().Add(50*time.Millisecond))
			if err == nil {
				err = c.Hello("localhost")
				_ = c.Close()
			}
			var timeout net.Error
			if !errors.As(err, &timeout) || !timeout.Timeout() || time.Since(start) > 2*time.Second {
				t.Fatalf("SMTP deadline not enforced: %v", err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("SMTP error did not close connection")
			}
		})
	}
}

func TestWorkerHoldsUnknownDeliveryAndRedactsTransportErrors(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown=%v", unknown), func(t *testing.T) {
			db := newDB(t)
			id := enqueueRaw(t, db, 1, "buyer@example.invalid")
			var logs bytes.Buffer
			w := NewWorker(db, dbConfiguredCache(t, db), slog.New(slog.NewTextHandler(&logs, nil)), time.Second)
			calls := 0
			w.send = func(Config, string, string, string) error {
				calls++
				cause := errors.New("buyer@example.invalid fixture-password private-message-body")
				if unknown {
					return fmt.Errorf("%w: %w", ErrDeliveryUnknown, cause)
				}
				return cause
			}
			w.tick(context.Background())
			status, attempts := rowState(t, db, id)
			if attempts != 1 || unknown && status != "delivery_unknown" || !unknown && status != "pending" {
				t.Fatalf("status=%s attempts=%d", status, attempts)
			}
			w.tick(context.Background())
			if calls != 1 {
				t.Fatal("held or deferred delivery was sent again")
			}
			var saved string
			if err := db.QueryRow(`SELECT last_error FROM email_outbox WHERE id=?`, id).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"buyer@example.invalid", "fixture-password", "private-message-body"} {
				if strings.Contains(saved, secret) || strings.Contains(logs.String(), secret) {
					t.Fatal("transport error leaked private data")
				}
			}
		})
	}
}

func TestWorkerRestartDoesNotBlindlyResendInterruptedMessage(t *testing.T) {
	db := newDB(t)
	id := enqueueRaw(t, db, 1, "buyer@example.invalid")
	if _, err := db.Exec(`UPDATE email_outbox SET status='sending' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Millisecond)
	w.send = func(Config, string, string, string) error { t.Error("interrupted send was repeated"); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(time.Second)
	for {
		status, _ := rowState(t, db, id)
		if status == "delivery_unknown" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("restart did not preserve unknown result")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWorkerSMTPAcceptanceSurvivesLostQuitReply(t *testing.T) {
	db := newDB(t)
	id := enqueueRaw(t, db, 1, "buyer@example.invalid")
	cfg, _ := localSMTP(t, "drop_quit_ack")
	w := NewWorker(db, dbConfiguredCache(t, db), quietLogger(), time.Second)
	w.send = func(_ Config, to, subject, body string) error { return Send(cfg, to, subject, body) }
	w.tick(context.Background())
	if status, _ := rowState(t, db, id); status != "sent" {
		t.Fatalf("acknowledged DATA treated as retryable: %s", status)
	}
}
