package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strings"
	textTemplate "text/template"
	"time"
)

const invitationHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Join {{.AppName}}</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background-color: #f3f4f6;
      margin: 0;
      padding: 0;
      -webkit-font-smoothing: antialiased;
    }
    .wrapper {
      width: 100%;
      background-color: #f3f4f6;
      padding: 40px 20px;
      box-sizing: border-box;
    }
    .container {
      max-width: 600px;
      margin: 0 auto;
      background: #ffffff;
      border-radius: 16px;
      overflow: hidden;
      box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
    }
    .header {
      background: linear-gradient(135deg, #013E7D 0%, #002244 100%);
      padding: 40px;
      text-align: center;
      color: #ffffff;
    }
    .header h1 {
      margin: 0;
      font-size: 28px;
      font-weight: 700;
      letter-spacing: -0.5px;
    }
    .content {
      padding: 40px;
      color: #374151;
      line-height: 1.6;
    }
    .greeting {
      font-size: 18px;
      font-weight: 600;
      margin-bottom: 16px;
    }
    .message {
      font-size: 16px;
      margin-bottom: 24px;
    }
    .cta-container {
      text-align: center;
      margin: 32px 0;
    }
    .btn {
      display: inline-block;
      padding: 14px 30px;
      background-color: #013E7D;
      color: #ffffff !important;
      text-decoration: none;
      border-radius: 8px;
      font-weight: 600;
      font-size: 16px;
      box-shadow: 0 4px 12px rgba(1, 62, 125, 0.2);
    }
    .btn:hover {
      background-color: #002c5c;
    }
    .cli-section {
      background-color: #0f172a;
      border-radius: 12px;
      padding: 24px;
      color: #f8fafc;
      margin-top: 32px;
    }
    .cli-title {
      font-size: 14px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 1px;
      color: #94a3b8;
      margin-bottom: 16px;
    }
    .cli-header {
      font-size: 13px;
      color: #38bdf8;
      margin-bottom: 6px;
      font-weight: 600;
    }
    .code-block {
      background-color: #1e293b;
      padding: 12px;
      border-radius: 6px;
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, Courier, monospace;
      font-size: 13px;
      margin: 0 0 16px 0;
      white-space: pre-wrap;
      word-break: break-all;
      color: #e2e8f0;
      border-left: 4px solid #38bdf8;
    }
    .code-block:last-child {
      margin-bottom: 0;
    }
    .footer {
      background-color: #f9fafb;
      padding: 24px 40px;
      text-align: center;
      font-size: 12px;
      color: #9ca3af;
      border-top: 1px solid #f3f4f6;
    }
  </style>
</head>
<body>
  <div class="wrapper">
    <div class="container">
      <div class="header">
        <h1>LiveReview</h1>
      </div>
      <div class="content">
        <div class="greeting">Hi {{.InvitedToName}},</div>
        <div class="message">
          <strong>{{.InvitedByName}}</strong> has invited you to join the <strong>{{.AppName}}</strong> workspace. Collaborative, AI-powered code reviews are just one step away.
        </div>
        <div class="cta-container">
          <a href="{{.URL}}" class="btn">Join Workspace</a>
        </div>
        
        {{if or .InstallCommandLinux .InstallCommandWindows}}
        <div class="cli-section">
          <div class="cli-title">Get Started with the CLI</div>
          
          {{if .InstallCommandLinux}}
          <div class="cli-header">Linux / macOS Install Command:</div>
          <pre class="code-block">{{.InstallCommandLinux}}</pre>
          {{end}}
          
          {{if .InstallCommandWindows}}
          <div class="cli-header">Windows PowerShell Install Command:</div>
          <pre class="code-block">{{.InstallCommandWindows}}</pre>
          {{end}}
        </div>
        {{end}}
      </div>
      <div class="footer">
        This is an automated invitation email. If you did not expect this, you can safely ignore it.<br>
        &copy; 2026 LiveReview. All rights reserved.
      </div>
    </div>
  </div>
</body>
</html>
`

const invitationTextTemplate = `Hi {{.InvitedToName}},

{{.InvitedByName}} has invited you to join the {{.AppName}} workspace!

Join Workspace:
{{.URL}}
{{if or .InstallCommandLinux .InstallCommandWindows}}
Get Started with the CLI:
{{if .InstallCommandLinux}}
Linux / macOS:
{{.InstallCommandLinux}}
{{end}}
{{if .InstallCommandWindows}}
Windows PowerShell:
{{.InstallCommandWindows}}
{{end}}
{{end}}
This is an automated invitation email. If you did not expect this, you can safely ignore it.
`

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
	boundary := "livereview-smtp-boundary-12345"
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

	fmt.Printf("[Invitation] Sending SMTP email via %s to %s\n", addr, params.InvitedToEmail)

	var conn net.Conn
	if port == 465 {
		// SSL/TLS direct connection
		conn, err = tls.Dial("tcp", addr, tlsConfig)
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

	fmt.Printf("[Invitation] Successfully sent SMTP email to: %s\n", params.InvitedToEmail)
	return nil
}

func SendTestEmailSMTP(host string, port int, username, password, sender, senderName string, skipTLS bool, recipient string) error {
	params := InvitationParams{
		AppName:        "LiveReview (Test)",
		InvitedToName:  "Admin",
		InvitedToEmail: recipient,
		InvitedByName:  "System Administrator",
		URL:            "https://livereview.io",
	}
	return SendInvitationEmailSMTP(host, port, username, password, sender, senderName, skipTLS, params)
}
