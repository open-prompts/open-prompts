package service

import (
	"crypto/tls"
	"fmt"
	"net/smtp"

	"go.uber.org/zap"
)

type EmailService interface {
	SendVerificationCode(toEmail, code, lang string) error
}

type emailService struct {
	smtpHost     string
	smtpPort     string
	smtpUser     string
	smtpPassword string
	fromEmail    string
}

func NewEmailService(host, port, user, password, from string) EmailService {
	return &emailService{
		smtpHost:     host,
		smtpPort:     port,
		smtpUser:     user,
		smtpPassword: password,
		fromEmail:    from,
	}
}

func (s *emailService) SendVerificationCode(toEmail, code, lang string) error {
	// If configuration is missing, just log and return (for dev/test environments without SMTP)
	if s.smtpHost == "" || s.smtpPort == "" {
		zap.S().Warnf("SMTP configuration missing. Skipping email to %s. Code: %s", toEmail, code)
		return nil
	}

	var subject, body string

	if lang == "zh" {
		subject = "Awsome Prompts 验证码"
		body = fmt.Sprintf("您的验证码是: %s\n该验证码5分钟内有效。", code)
	} else {
		subject = "Awsome Prompts Verification Code"
		body = fmt.Sprintf("Your verification code is: %s\nThis code is valid for 5 minutes.", code)
	}

	// Simple text email
	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n", s.fromEmail, toEmail, subject, body))

	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPassword, s.smtpHost)
	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)

	var err error
	if s.smtpPort == "465" {
		// SMTPS (Implicit TLS)
		tlsConfig := &tls.Config{
			ServerName: s.smtpHost,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			zap.S().Errorf("Failed to connect to SMTP server %s: %v", addr, err)
			return err
		}

		c, err := smtp.NewClient(conn, s.smtpHost)
		if err != nil {
			_ = conn.Close()
			zap.S().Errorf("Failed to create SMTP client: %v", err)
			return err
		}

		if err = c.Auth(auth); err != nil {
			_ = c.Close()
			zap.S().Errorf("SMTP Auth failed: %v", err)
			return err
		}
		if err = c.Mail(s.fromEmail); err != nil {
			_ = c.Close()
			zap.S().Errorf("SMTP MAIL failed: %v", err)
			return err
		}
		if err = c.Rcpt(toEmail); err != nil {
			_ = c.Close()
			zap.S().Errorf("SMTP RCPT failed: %v", err)
			return err
		}
		w, err := c.Data()
		if err != nil {
			_ = c.Close()
			zap.S().Errorf("SMTP DATA failed: %v", err)
			return err
		}
		_, err = w.Write(msg)
		if err != nil {
			_ = c.Close()
			zap.S().Errorf("SMTP Write failed: %v", err)
			return err
		}
		err = w.Close()
		if err != nil {
			_ = c.Close()
			zap.S().Errorf("SMTP Close data failed: %v", err)
			return err
		}
		_ = c.Quit()
	} else {
		err = smtp.SendMail(addr, auth, s.fromEmail, []string{toEmail}, msg)
		if err != nil {
			zap.S().Errorf("Failed to send email to %s: %v", toEmail, err)
			return err
		}
	}

	zap.S().Infof("Sent verification code to %s", toEmail)
	return nil
}
