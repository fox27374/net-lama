package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/fox27374/net-lama/internal/store"
	"github.com/gosnmp/gosnmp"
)

// notifyTargets dispatches alert notifications to the rule's targets.
func (s *Server) notifyTargets(rule *store.AlertRule, conn *connectedAgent, alert *store.Alert, state string, value float64) {
	if len(rule.TargetIds) == 0 {
		return
	}

	// Build the notification payload (same as webhook payload)
	payload := map[string]any{
		"state":     state,
		"rule":      rule.Name,
		"test":      rule.TestName,
		"metric":    rule.Metric,
		"threshold": rule.Threshold,
		"value":     value,
		"tenant":    conn.tenant,
		"site":      conn.agent.SiteName,
		"agent":     conn.agent.Name,
		"message":   alert.Message,
		"time":      time.Now().UTC().Format(time.RFC3339),
	}
	payloadJSON, _ := json.Marshal(payload)

	// Notify each target in a goroutine
	for _, targetID := range rule.TargetIds {
		targetID := targetID // capture for closure
		go func() {
			target, err := s.Store.GetAlertTarget(targetID)
			if err != nil {
				s.Logger.Warn("Failed to load alert target",
					slog.String("targetId", targetID),
					slog.Any("error", err))
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var notifyErr error
			switch target.Type {
			case "webhook":
				notifyErr = notifyWebhook(ctx, target, payloadJSON)
			case "email":
				notifyErr = notifyEmail(ctx, s.SMTPConfig, target, payload, state)
			case "script":
				notifyErr = notifyScript(ctx, target, payloadJSON, rule, state, conn.agent.Name)
			case "snmp":
				notifyErr = notifySNMP(ctx, target, rule, state, conn.agent.Name, value)
			default:
				notifyErr = fmt.Errorf("unknown target type: %s", target.Type)
			}

			if notifyErr != nil {
				s.Logger.Warn("Alert notification failed",
					slog.String("rule", rule.Name),
					slog.String("target", target.Name),
					slog.String("type", target.Type),
					slog.Any("error", notifyErr))
			}
		}()
	}
}

// notifyWebhook POSTs the alert to the target's webhook URL.
func notifyWebhook(ctx context.Context, target *store.AlertTarget, payloadJSON []byte) error {
	url, ok := target.Config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("webhook URL not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SMTPConfig holds SMTP configuration from env vars.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Pass     string
	From     string
	StartTLS bool
}

// notifyEmail sends an email notification.
func notifyEmail(ctx context.Context, config *SMTPConfig, target *store.AlertTarget, payload map[string]any, state string) error {
	if config.Host == "" {
		return fmt.Errorf("SMTP not configured")
	}

	toList, ok := target.Config["to"].([]interface{})
	if !ok || len(toList) == 0 {
		return fmt.Errorf("email 'to' list not configured")
	}

	var recipients []string
	for _, t := range toList {
		if email, ok := t.(string); ok {
			recipients = append(recipients, email)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no valid email recipients")
	}

	subject, _ := target.Config["subject"].(string)
	if subject == "" {
		subject = fmt.Sprintf("Alert: %s (%s)", payload["rule"], state)
	}

	// Plain-text body from payload
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Alert: %s\n", payload["rule"]))
	body.WriteString(fmt.Sprintf("State: %s\n", state))
	body.WriteString(fmt.Sprintf("Agent: %s\n", payload["agent"]))
	body.WriteString(fmt.Sprintf("Message: %s\n\n", payload["message"]))
	payloadJSON, _ := json.Marshal(payload)
	body.WriteString(fmt.Sprintf("Payload:\n%s\n", string(payloadJSON)))

	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))

	// Create SMTP client
	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dialing SMTP: %w", err)
	}
	defer conn.Close()

	if config.StartTLS {
		if err := conn.StartTLS(nil); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	if config.User != "" && config.Pass != "" {
		if err := conn.Auth(smtp.PlainAuth("", config.User, config.Pass, config.Host)); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	// Build email headers
	from := mail.Address{Address: config.From}
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n",
		from.String(), strings.Join(recipients, ", "), subject)
	msg := headers + body.String()

	if err := conn.Mail(config.From); err != nil {
		return fmt.Errorf("MAIL: %w", err)
	}
	for _, rcpt := range recipients {
		if err := conn.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT %s: %w", rcpt, err)
		}
	}
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing DATA: %w", err)
	}

	return conn.Quit()
}

// notifyScript executes a script with the alert payload.
func notifyScript(ctx context.Context, target *store.AlertTarget, payloadJSON []byte, rule *store.AlertRule, state, agentName string) error {
	scriptPath, ok := target.Config["path"].(string)
	if !ok || scriptPath == "" {
		return fmt.Errorf("script path not configured")
	}

	args, _ := target.Config["args"].([]interface{})
	var cmdArgs []string
	for _, a := range args {
		if s, ok := a.(string); ok {
			cmdArgs = append(cmdArgs, s)
		}
	}

	cmd := exec.CommandContext(ctx, scriptPath, cmdArgs...)
	cmd.Stdin = bytes.NewReader(payloadJSON)
	cmd.Env = append(os.Environ(),
		"NETLAMA_ALERT_STATE="+state,
		"NETLAMA_ALERT_RULE="+rule.Name,
		"NETLAMA_ALERT_AGENT="+agentName,
		"NETLAMA_ALERT_MESSAGE="+string(payloadJSON), // full message in case of parsing
	)

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("script failed: %s", stderr.String())
		}
		return fmt.Errorf("script failed: %w", err)
	}

	return nil
}

// notifySNMP sends an SNMPv2c trap.
func notifySNMP(ctx context.Context, target *store.AlertTarget, rule *store.AlertRule, state, agentName string, value float64) error {
	host, ok := target.Config["host"].(string)
	if !ok || host == "" {
		return fmt.Errorf("SNMP host not configured")
	}

	port := 162
	if p, ok := target.Config["port"].(float64); ok {
		port = int(p)
	}

	community := "public"
	if c, ok := target.Config["community"].(string); ok {
		community = c
	}

	conn := &gosnmp.GoSNMP{
		Target:    host,
		Port:      uint16(port),
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(5) * time.Second,
		Retries:   1,
	}

	if err := conn.Connect(); err != nil {
		return fmt.Errorf("SNMP connect failed: %w", err)
	}
	defer conn.Close()

	// Build trap with varbinds: 1.3.6.1.4.1.59777.* (netlama enterprise OID)
	trapData := gosnmp.SnmpTrap{
		Timestamp: uint(time.Now().Unix()),
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  ".1.3.6.1.4.1.59777.1", // alertRuleName
				Type:  gosnmp.OctetString,
				Value: rule.Name,
			},
			{
				Name:  ".1.3.6.1.4.1.59777.2", // alertState
				Type:  gosnmp.OctetString,
				Value: state,
			},
			{
				Name:  ".1.3.6.1.4.1.59777.3", // alertAgent
				Type:  gosnmp.OctetString,
				Value: agentName,
			},
			{
				Name:  ".1.3.6.1.4.1.59777.4", // alertValue
				Type:  gosnmp.OctetString,
				Value: fmt.Sprintf("%.2f", value),
			},
		},
	}

	_, err := conn.SendTrap(trapData)
	if err != nil {
		return fmt.Errorf("SNMP trap failed: %w", err)
	}

	return nil
}
