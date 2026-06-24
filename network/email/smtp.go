package email

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"html/template"
	"mime"
	"net"
	"net/mail"
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

	// Prepare data for templates
	data := struct {
		InvitationParams
		CurrentYear int
	}{
		InvitationParams: params,
		CurrentYear:      time.Now().Year(),
	}

	// Render templates
	htmlTmpl, err := template.New("invitationHTML").Parse(invitationHTMLTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse HTML template: %w", err)
	}
	var htmlBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return fmt.Errorf("failed to execute HTML template: %w", err)
	}

	textTmpl, err := textTemplate.New("invitationText").Parse(invitationTextTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse text template: %w", err)
	}
	var textBuf bytes.Buffer
	if err := textTmpl.Execute(&textBuf, data); err != nil {
		return fmt.Errorf("failed to execute text template: %w", err)
	}

	subject := fmt.Sprintf("Join %s Workspace", params.AppName)
	return SendRawEmailSMTP(host, port, username, password, sender, senderName, skipTLS, params.InvitedToEmail, subject, textBuf.String(), htmlBuf.String())
}

// SendRawEmailSMTP handles the actual SMTP protocol and MIME multipart generation
func SendRawEmailSMTP(host string, port int, username, password, sender, senderName string, skipTLS bool, recipient, subject, textBody, htmlBody string) error {
	// Construct email message with multipart/alternative MIME type
	randBytes := make([]byte, 16)
	_, _ = rand.Read(randBytes)
	boundary := fmt.Sprintf("livereview-smtp-boundary-%x", randBytes)

	header := make(map[string]string)
	
	fromAddr := mail.Address{Name: senderName, Address: sender}
	header["From"] = fromAddr.String()
	
	toAddr := mail.Address{Address: recipient}
	header["To"] = toAddr.String()
	
	header["Subject"] = mime.QEncoding.Encode("utf-8", subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = fmt.Sprintf("multipart/alternative; boundary=%s", boundary)

	var message strings.Builder
	for k, v := range header {
		// Prevent CRLF injection in any future arbitrary headers (defense in depth)
		safeV := strings.ReplaceAll(v, "\r", "")
		safeV = strings.ReplaceAll(safeV, "\n", "")
		message.WriteString(fmt.Sprintf("%s: %s\r\n", k, safeV))
	}
	message.WriteString("\r\n")

	// Plain text boundary section
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	message.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
	message.WriteString(textBody)
	message.WriteString("\r\n\r\n")

	// HTML boundary section
	message.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	message.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	message.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
	message.WriteString(htmlBody)
	message.WriteString("\r\n\r\n")

	message.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	addr := fmt.Sprintf("%s:%d", host, port)
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: skipTLS,
	}

	log.Info().Msgf("[SMTP] Sending email via %s to %s", addr, recipient)

	var conn net.Conn
	var err error
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

	if err = client.Rcpt(recipient); err != nil {
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

	log.Info().Msgf("[SMTP] Successfully sent email to: %s", recipient)
	return nil
}

// SendVerificationEmailSMTP sends a verification email to confirm SMTP settings from the admin dashboard
func SendVerificationEmailSMTP(host string, port int, username, password, sender, senderName string, skipTLS bool, recipient string) error {
	subject := "LiveReview SMTP Verification"
	
	htmlBody := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
</head>
<body style="font-family: sans-serif; padding: 20px;">
  <h2>SMTP Configuration Successful!</h2>
  <p>This is a test email from <strong>LiveReview Enterprise version</strong>.</p>
  <p>Your SMTP configuration has been correctly applied to your self-hosted instance.</p>
</body>
</html>`

	textBody := "SMTP Configuration Successful!\n\nThis is a test email from LiveReview Enterprise version.\nYour SMTP configuration has been correctly applied to your self-hosted instance."

	return SendRawEmailSMTP(host, port, username, password, sender, senderName, skipTLS, recipient, subject, textBody, htmlBody)
}
