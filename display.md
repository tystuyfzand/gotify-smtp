## GotifySMTP

Devices which can send email via SMTP can send notifications to this server. Every
incoming email becomes a Gotify message in the SMTP application.

Message subjects are translated into the notification title, and the body is parsed to become the text. If no subject is provided, the default "{{.subject}}" will be used.

Messages will be created with priority {{.priority}}.

| Setting | Value |
|---------|-------|
{{range .settings -}}
| {{.Name}} | {{.Value}} |
{{end}}

### Notes

- Usernames map to Gotify users, meaning that your username `{{.username}}` will map to your inbox.
- The SMTP port (1025) must be accessible when running in Docker or behind a firewall.