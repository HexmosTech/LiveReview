package email

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strings"
	textTemplate "text/template"
	"time"
	_ "embed"
	"github.com/rs/zerolog/log"
)

//go:embed templates/invitation.html
var invitationHTMLTemplate string

//go:embed templates/invitation.txt
var invitationTextTemplate string

// SendInvitationEmailSMTP sends the invitation email using SMTP credentials
func SendInvitationEmailSMTP(host string, port int, username, password, sender, senderName string, skipTLS bool, params InvitationParams) error {
	if host == "" {
		return fmt.Errorf("SMTP host is not set")
	}

	if sender == "" {
		return fmt.Errorf("SMTP sender is not set")
	}

	// Render templates
	htmlTmpl, err := template.New("invitationHTML").Parse(invitationHTMLTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %w", err)
	}
	var htmlBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, params); err != nil {
		return fmt.Errorf("failed to execute HTML template: %w", err)
	}

	textTmpl, err := textTemplate.New("invitationText").Parse(invitationTextTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse text template: %w", err)
	}
	var textBuf bytes.Buffer
	if err := textTmpl.Execute(&textBuf, params); err != nil {
		return fmt.Errorf("failed to execute text template: %w", err)
	}

	// Construct email message with multipart/alternative MIME type
	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	boundary := fmt.Sprintf("livereview-smtp-boundary-%x", randBytes)
	subject := fmt.Sprintf("Join %s Workspace", params.AppName)

	header := make(map[string]string)
	header["From"] = fmt.Sprintf("%s <%s>", senderName, sender)
	header["To"] = params.InvitedToEmail
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = fmt.Sprintf("multipart/alternative; boundary=%s", boundary)

	var message strings.Builder
	for k, v := range header {
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	message.WriteString("\r\n")

	// Plain text boundary section
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	message.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
	message.WriteString(textBuf.String())
	message.WriteString("\r\n\r\n")

	// HTML boundary section
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	message.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
	message.WriteString(htmlBuf.String())
	message.WriteString("\r\n\r\n")

	message.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	addr := fmt.Sprintf("%s:%d", host, port)
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: skipTLS,
	}

	log.Info().Msgf("[Invitation] Sending SMTP email via %s to %s", addr, params.InvitedToEmail)

	var conn net.Conn
	if port == 465 {
		// SSL/TLS direct connection
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to dial SMTP over SSL (port 465): %w", err)
		}
	} else {
		// Plain TCP connection with potential STARTTLS
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("failed to dial SMTP server: %w", err)
		}
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	if port != 465 {
		if hasStartTLS, _ := client.Extension("STARTTLS"); hasStartTLS {
			if err = client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("failed to start TLS: %w", err)
			}
		}
	}

	if username != "" || password != "" {
		auth := smtp.PlainAuth("", username, password, host)
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	if err = client.Mail(sender); err != nil {
		return fmt.Errorf("failed to set SMTP mail sender: %w", err)
	}

	if err = client.Rcpt(params.InvitedToEmail); err != nil {
		return fmt.Errorf("failed to set SMTP mail recipient: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open SMTP data writer: %w", err)
	}
	defer w.Close()

	_, err = w.Write([]byte(message.String()))
	if err != nil {
		return fmt.Errorf("failed to write SMTP message: %w", err)
	}

	log.Info().Msgf("[Invitation] Successfully sent SMTP email to: %s", params.InvitedToEmail)
	return nil
}

// SendVerificationEmailSMTP sends a verification email to confirm SMTP settings from the admin dashboard
func SendVerificationEmailSMTP(host string, port int, username, password, sender, senderName string, skipTLS bool, recipient string) error {
	params := InvitationParams{
		AppName:        "LiveReview (Test)",
		InvitedToName:  "Admin",
		InvitedToEmail: recipient,
		InvitedByName:  "System Administrator",
		URL:            "https://livereview.io",
	}
	return SendInvitationEmailSMTP(host, port, username, password, sender, senderName, skipTLS, params)
}
