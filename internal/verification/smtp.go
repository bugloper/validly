package verification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// SMTPMailer sends mail through an SMTP relay. Port 465 uses implicit TLS;
// other ports use STARTTLS. Unless Opportunistic is set, a server that does
// not offer STARTTLS is refused, so an attacker on the network path cannot
// strip TLS and read the token. Credentials are only ever sent over TLS (or
// to localhost), as enforced by net/smtp.
type SMTPMailer struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	// Opportunistic allows plaintext delivery when the server does not offer
	// STARTTLS. Use it only for local relays such as Mailpit.
	Opportunistic bool
}

// ErrNoSTARTTLS is returned when TLS is required but not offered.
var ErrNoSTARTTLS = errors.New("smtp server does not offer STARTTLS")

// smtpError describes a failed step without the server's reply text, which
// often echoes the recipient address and must not reach the logs.
func smtpError(step string, err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return fmt.Errorf("smtp %s: server replied %d", step, reply.Code)
	}
	return fmt.Errorf("smtp %s: %w", step, err)
}

func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	if strings.ContainsAny(msg.To+m.From+msg.Subject, "\r\n") {
		return errors.New("header values must not contain line breaks")
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("smtp from address: %w", err)
	}
	addr := net.JoinHostPort(m.Host, m.Port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var conn net.Conn
	if m.Port == "465" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: m.Host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer c.Close()

	if m.Port != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: m.Host}); err != nil {
				return smtpError("STARTTLS", err)
			}
		} else if !m.Opportunistic {
			return ErrNoSTARTTLS
		}
	}
	if m.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
			return smtpError("AUTH", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return smtpError("MAIL FROM", err)
	}
	if err := c.Rcpt(msg.To); err != nil {
		return smtpError("RCPT TO", err)
	}
	w, err := c.Data()
	if err != nil {
		return smtpError("DATA", err)
	}
	headers := "From: " + m.From + "\r\n" +
		"To: " + msg.To + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n"
	if _, err := w.Write([]byte(headers + msg.Text)); err != nil {
		return smtpError("write", err)
	}
	if err := w.Close(); err != nil {
		return smtpError("end of DATA", err)
	}
	if err := c.Quit(); err != nil {
		return smtpError("QUIT", err)
	}
	return nil
}
