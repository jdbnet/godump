package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"text/template"
	"time"

	"godump/config"
	"godump/logger"
)

type DBResult struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Result string `json:"result"`
}

type Payload struct {
	InstanceName  string     `json:"instance_name"`
	OverallResult string     `json:"overall_result"`
	Time          time.Time  `json:"time"`
	Databases     []DBResult `json:"databases"`
}

func shouldSend(global config.NotificationEventsConfig, local *config.NotificationEventsConfig, result string) bool {
	events := global
	if local != nil {
		events = *local
	}
	if result == "success" && !events.OnSuccess {
		return false
	}
	if (result == "failed" || result == "partial") && !events.OnFailure {
		return false
	}
	return true
}

func Send(cfg config.NotificationsConfig, payload Payload) {
	if cfg.Webhook.Enabled && shouldSend(cfg.Events, cfg.Webhook.Events, payload.OverallResult) {
		go sendWebhook(cfg.Webhook, payload)
	}
	for _, w := range cfg.Webhooks {
		if w.Enabled && shouldSend(cfg.Events, w.Events, payload.OverallResult) {
			go sendWebhook(w, payload)
		}
	}
	if cfg.Email.Enabled && shouldSend(cfg.Events, cfg.Email.Events, payload.OverallResult) {
		go sendEmail(cfg.Email, payload)
	}
}

func sendWebhook(cfg config.WebhookConfig, payload Payload) {
	message := fmt.Sprintf("**GoDump Backup Result**\nInstance: %s\nResult: %s\nTime: %s\n\nDatabases:\n", 
		payload.InstanceName, payload.OverallResult, payload.Time.Format("2006-01-02 15:04:05"))
	for _, db := range payload.Databases {
		message += fmt.Sprintf("- %s: %s (%s)\n", db.Name, db.Result, formatBytes(db.Size))
	}

	webhookPayload := map[string]interface{}{
		"content": message,
		"text":    message,
		"title":   fmt.Sprintf("GoDump Backup Result: %s", payload.InstanceName),
		"message": message,
	}

	data, err := json.Marshal(webhookPayload)
	if err != nil {
		logger.Error(payload.InstanceName, "Failed to marshal webhook payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", cfg.URL, bytes.NewBuffer(data))
	if err != nil {
		logger.Error(payload.InstanceName, "Failed to create webhook request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error(payload.InstanceName, "Failed to send webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		logger.Error(payload.InstanceName, "Webhook returned error status: %d", resp.StatusCode)
	} else {
		logger.Info(payload.InstanceName, "Webhook notification sent successfully")
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

const emailTmplStr = `
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: 'Inter', sans-serif; background-color: #0a0a0a; color: #f0f0f0; margin: 0; padding: 20px; }
        .card { background-color: #141414; border: 1px solid #2a2a2a; border-radius: 8px; padding: 20px; max-width: 600px; margin: 0 auto; }
        h2 { margin-top: 0; }
        .accent { color: #21D198; }
        .error { color: #ef4444; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #2a2a2a; }
        th { color: #888888; text-transform: uppercase; font-size: 12px; }
    </style>
</head>
<body>
    <div class="card">
        <h2>GoDump - Instance: <span class="accent">{{.InstanceName}}</span></h2>
        <p>Overall Result: <strong class="{{if eq .OverallResult "success"}}accent{{else}}error{{end}}">{{.OverallResult}}</strong></p>
        <p>Time: {{.Time.Format "2006-01-02 15:04:05"}}</p>
        
        <table>
            <thead>
                <tr>
                    <th>Database</th>
                    <th>Size</th>
                    <th>Result</th>
                </tr>
            </thead>
            <tbody>
                {{range .Databases}}
                <tr>
                    <td>{{.Name}}</td>
                    <td>{{formatBytes .Size}}</td>
                    <td class="{{if eq .Result "success"}}accent{{else}}error{{end}}">{{.Result}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>
`

func sendEmail(cfg config.EmailConfig, payload Payload) {
	tmpl, err := template.New("email").Funcs(template.FuncMap{"formatBytes": formatBytes}).Parse(emailTmplStr)
	if err != nil {
		logger.Error(payload.InstanceName, "Failed to parse email template: %v", err)
		return
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, payload); err != nil {
		logger.Error(payload.InstanceName, "Failed to execute email template: %v", err)
		return
	}

	subject := fmt.Sprintf("Subject: GoDump Backup Result: %s [%s]\r\n", payload.InstanceName, payload.OverallResult)
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	
	msg := []byte(fmt.Sprintf("To: %s\r\nFrom: GoDump <%s>\r\n%s%s\r\n%s", cfg.To, cfg.From, subject, mime, body.String()))

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	err = smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, msg)
	if err != nil {
		logger.Error(payload.InstanceName, "Failed to send email: %v", err)
	} else {
		logger.Info(payload.InstanceName, "Email notification sent successfully")
	}
}
