package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"

	_ "unsafe"
)

var (
	// ErrNotSupportAuth auth failed error
	ErrNotSupportAuth = errors.New("smtp: server doesn't support AUTH")
	// ErrNoReceiver reciver is empty error
	ErrNoReceiver = errors.New("mail: no receiver")
)

// SmtpConfig smtp config
type SmtpConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	TLS      bool   `json:"TLS"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Config smtp config
type Config struct {
	To   []string   `json:"to"`
	SMTP SmtpConfig `json:"SMTP"`
}

// Send send mail on STARTTLS/TLS port
func (cfg *Config) Send(nickName, subject, body string) error {
	if len(cfg.To) == 0 {
		return ErrNoReceiver
	}
	header := [][2]string{
		{"From", nickName + "<" + cfg.SMTP.Username + ">"},
		{"To", strings.Join(cfg.To, ";")},
		{"Subject", subject},
		{"Content-Type", "text/html; charset=UTF-8"},
	}
	message := bytes.Buffer{}
	for _, v := range header {
		message.WriteString(v[0] + ": " + v[1] + "\r\n")
	}
	message.WriteString("\r\n" + body)
	auth := smtp.PlainAuth(
		"",
		cfg.SMTP.Username,
		cfg.SMTP.Password,
		cfg.SMTP.Host,
	)
	client, err := newClient(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.TLS)
	if err != nil {
		return err
	}
	return sendMail(client,
		auth,
		cfg.SMTP.Username,
		cfg.To,
		message.Bytes(),
	)
}

// LoadConfig if Email config exists, return email config
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err = json.NewDecoder(file).Decode(cfg); err != nil {
		cfg = nil
	}
	file.Close()
	return cfg, err
}

func newClient(host string, port int, TLS bool) (client *smtp.Client, err error) {
	addr := host + ":" + strconv.FormatInt(int64(port), 10)
	var conn net.Conn
	if TLS {
		conn, err = tls.Dial("tcp",
			addr,
			&tls.Config{ServerName: host},
		)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return
	}

	client, err = smtp.NewClient(conn, host)
	if err != nil {
		return
	}
	if TLS {
		err = client.Hello("localhost")
	} else if ok, _ := client.Extension("STARTTLS"); ok {
		err = client.StartTLS(&tls.Config{ServerName: host})
	}

	if err != nil {
		client.Close()
		client = nil
	}
	return
}

func sendMail(c *smtp.Client, a smtp.Auth, from string, to []string, msg []byte) error {
	var err error
	defer c.Close()
	if err = validateLine(from); err != nil {
		return err
	}
	for _, recp := range to {
		if err = validateLine(recp); err != nil {
			return err
		}
	}

	if a != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return ErrNotSupportAuth
		}
		if err = c.Auth(a); err != nil {
			return err
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

//go:linkname validateLine net/smtp.validateLine
func validateLine(line string) error
