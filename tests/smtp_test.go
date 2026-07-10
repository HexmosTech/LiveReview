package livereview

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/livereview/network/email"
)

func TestSendInvitationEmailSMTP(t *testing.T) {
	// Start a local mock SMTP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start local mock SMTP server: %v", err)
	}
	defer listener.Close()

	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}



	errChan := make(chan error, 1)
	receivedMsgChan := make(chan string, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errChan <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)

		// SMTP Greeting
		writer.WriteString("220 localhost ESMTP Mock\r\n")
		writer.Flush()

		// Read HELO/EHLO
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(line, "EHLO") && !strings.HasPrefix(line, "HELO") {
			errChan <- fmt.Errorf("expected EHLO/HELO, got: %s", line)
			return
		}
		writer.WriteString("250-localhost\r\n250 AUTH PLAIN\r\n")
		writer.Flush()

		// Auth
		line, _ = reader.ReadString('\n')
		if strings.HasPrefix(line, "AUTH") {
			writer.WriteString("235 2.7.0 Authentication successful\r\n")
			writer.Flush()
			line, _ = reader.ReadString('\n')
		}

		// Mail From
		if !strings.HasPrefix(line, "MAIL FROM:") {
			errChan <- fmt.Errorf("expected MAIL FROM, got: %s", line)
			return
		}
		writer.WriteString("250 2.1.0 Ok\r\n")
		writer.Flush()

		// RCPT To
		line, _ = reader.ReadString('\n')
		if !strings.HasPrefix(line, "RCPT TO:") {
			errChan <- fmt.Errorf("expected RCPT TO, got: %s", line)
			return
		}
		writer.WriteString("250 2.1.5 Ok\r\n")
		writer.Flush()

		// Data
		line, _ = reader.ReadString('\n')
		if !strings.HasPrefix(line, "DATA") {
			errChan <- fmt.Errorf("expected DATA, got: %s", line)
			return
		}
		writer.WriteString("354 Start mail input; end with <CR><LF>.<CR><LF>\r\n")
		writer.Flush()

		// Read body
		var body strings.Builder
		for {
			l, _ := reader.ReadString('\n')
			if l == ".\r\n" {
				break
			}
			body.WriteString(l)
		}
		writer.WriteString("250 2.0.0 Ok: queued as 12345\r\n")
		writer.Flush()

		receivedMsgChan <- body.String()
		errChan <- nil
	}()

	params := email.InvitationParams{
		AppName:               "TestApp",
		InvitedToName:         "John Doe",
		InvitedToEmail:        "john@example.com",
		InvitedByName:         "Alice",
		URL:                   "http://localhost:8080/invite",
		InstallCommandLinux:   "curl install",
		InstallCommandWindows: "iwr install",
	}

	port, _ := strconv.Atoi(portStr)
	err = email.SendInvitationEmailSMTP(
		"127.0.0.1",
		port,
		"testuser",
		"testpass",
		"sender@example.com",
		"Test Sender",
		true,
		params,
	)
	if err != nil {
		t.Fatalf("failed to send SMTP email: %v", err)
	}

	serverErr := <-errChan
	if serverErr != nil {
		t.Fatalf("mock SMTP server error: %v", serverErr)
	}

	receivedMsg := <-receivedMsgChan
	if !strings.Contains(receivedMsg, "john@example.com") {
		t.Errorf("expected email to contain recipient address, got: %s", receivedMsg)
	}
	if !strings.Contains(receivedMsg, "TestApp") {
		t.Errorf("expected email to contain AppName, got: %s", receivedMsg)
	}
	if !strings.Contains(receivedMsg, "curl install") {
		t.Errorf("expected email to contain Linux command, got: %s", receivedMsg)
	}
}
