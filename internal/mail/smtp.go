// SMTP 发送 / SMTP Sender
// 功能：有界 SMTP 投递与明确/未知结果分类；支持 none / STARTTLS / 隐式 TLS，拒绝邮件头注入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrDeliveryUnknown means DATA might have been accepted. Retrying can duplicate mail.
	ErrDeliveryUnknown = errors.New("mail: 邮件投递结果未知")
	ErrInvalidMessage  = errors.New("mail: 邮件地址或邮件头非法")
)

const smtpDialTimeout = 10 * time.Second
const smtpOperationTimeout = 30 * time.Second

// Send 发送一封纯文本邮件（UTF-8）。凭证仅在内存、不落日志。
// 加密模式：tls=隐式 TLS(465)；starttls=先明文再升级(587)；none=明文(25，不安全)。
func Send(cfg Config, to, subject, body string) error {
	if !cfg.Configured() {
		return errors.New("mail: SMTP 未配置")
	}
	if !plainMailbox(cfg.FromAddress) || !plainMailbox(to) || strings.ContainsFunc(cfg.FromName, unicode.IsControl) || strings.ContainsFunc(subject, unicode.IsControl) {
		return ErrInvalidMessage
	}
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	msg := buildMessage(cfg, to, subject, body)

	var auth smtp.Auth
	if strings.TrimSpace(cfg.Username) != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Encryption)) {
	case "tls":
		return sendImplicitTLS(addr, cfg.Host, auth, cfg.FromAddress, to, msg)
	case "none":
		return sendPlain(addr, cfg.Host, auth, cfg.FromAddress, to, msg, false)
	default: // starttls（默认）
		return sendPlain(addr, cfg.Host, auth, cfg.FromAddress, to, msg, true)
	}
}

// sendPlain 用明文连接；requireTLS=true 时强制 STARTTLS 升级（服务器不支持则报错，不静默降级）。
func sendPlain(addr, host string, auth smtp.Auth, from, to string, msg []byte, requireTLS bool) error {
	c, err := openSMTP(addr, host, false)
	if err != nil {
		return fmt.Errorf("mail: 连接 SMTP 失败: %w", err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Hello("localhost"); err != nil {
		return fmt.Errorf("mail: EHLO 失败: %w", err)
	}
	if requireTLS {
		ok, _ := c.Extension("STARTTLS")
		if !ok {
			return errors.New("mail: 服务器不支持 STARTTLS（如确为明文服务器请选加密方式 none）")
		}
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: STARTTLS 升级失败: %w", err)
		}
	}
	return finish(c, auth, from, to, msg)
}

// sendImplicitTLS 用隐式 TLS 连接（端口 465）。
func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	c, err := openSMTP(addr, host, true)
	if err != nil {
		return fmt.Errorf("mail: TLS 连接失败: %w", err)
	}
	defer func() { _ = c.Close() }()
	return finish(c, auth, from, to, msg)
}

func plainMailbox(value string) bool {
	if strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	parsed, err := stdmail.ParseAddress(value)
	return err == nil && parsed.Name == "" && parsed.Address == value
}

func openSMTP(addr, host string, implicitTLS bool) (*smtp.Client, error) {
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	var conn net.Conn
	var err error
	if implicitTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	return connectedSMTP(conn, host, time.Now().Add(smtpOperationTimeout))
}

// The absolute connection deadline covers greeting, TLS upgrade, AUTH, DATA and QUIT together.
func connectedSMTP(conn net.Conn, host string, deadline time.Time) (*smtp.Client, error) {
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

// finish 完成 AUTH（若有）+ MAIL/RCPT/DATA 投递。
func finish(c *smtp.Client, auth smtp.Auth, from, to string, msg []byte) error {
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("mail: 已配置登录凭证，但服务器不支持 AUTH")
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("mail: SMTP 鉴权失败: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail: MAIL FROM 失败: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO 失败: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA 失败: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mail: 写正文中断: %w: %w", ErrDeliveryUnknown, err)
	}
	if err := w.Close(); err != nil {
		var reply *textproto.Error
		if errors.As(err, &reply) && reply.Code >= 200 && reply.Code < 300 {
			_ = c.Quit()
			return nil
		}
		if errors.As(err, &reply) && reply.Code >= 400 && reply.Code < 600 {
			return fmt.Errorf("mail: SMTP 拒绝邮件: %w", err)
		}
		return fmt.Errorf("mail: 未收到正文接收确认: %w: %w", ErrDeliveryUnknown, err)
	}
	// DATA's positive reply confirms acceptance; a lost QUIT response cannot undo it.
	_ = c.Quit()
	return nil
}

// buildMessage 组装 RFC 5322 报文（UTF-8 纯文本；Subject 用 RFC 2047 编码支持中文）。
func buildMessage(cfg Config, to, subject, body string) []byte {
	from := cfg.FromAddress
	if strings.TrimSpace(cfg.FromName) != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", cfg.FromName), cfg.FromAddress)
	}
	h := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n"
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	return []byte(h + strings.ReplaceAll(body, "\n", "\r\n"))
}
