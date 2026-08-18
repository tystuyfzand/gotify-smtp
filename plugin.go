package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/emersion/go-smtp"
	"github.com/gotify/plugin-api"
	"html"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	s          *smtp.Server
	serverAddr string
	serverLock = &sync.Mutex{}
	users      = make(map[string]*Plugin)
	userLock   = &sync.RWMutex{}
)

const defaultListenAddr = ":1025"

// GetGotifyPluginInfo returns gotify plugin info
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		Name:        "SMTP",
		ModulePath:  "github.com/tystuyfzand/gotify-smtp",
		Author:      "Tyler Stuyfzand",
		Website:     "https://meow.tf",
		Description: "Turns incoming mail into Gotify messages. Point any device that can only send e-mail at it.",
	}
}

// Config is the per-user plugin configuration edited in the Gotify UI.
//
// Everything except ListenAddr is per user: the SMTP username selects the
// Gotify user, so each user gets their own password, priority and so on.
// ListenAddr configures the single shared SMTP server and therefore applies
// instance-wide - see the note in GetDisplay.
type Config struct {
	// Password required for SMTP AUTH. Empty means any password is accepted,
	// which is how the plugin behaved before this option existed.
	Password string `yaml:"password"`
	// Priority assigned to messages created from mail (0-10).
	Priority int `yaml:"priority"`
	// DefaultTitle is used when a mail carries no Subject header.
	DefaultTitle string `yaml:"default_title"`
	// StripHTML converts an HTML-only mail into readable text instead of
	// forwarding raw markup.
	StripHTML bool `yaml:"strip_html"`
	// AllowedSenders optionally restricts the envelope sender. Entries may be
	// a full address (alerts@example.com) or a domain (@example.com).
	// Empty list means every sender is accepted.
	AllowedSenders []string `yaml:"allowed_senders"`
	// ListenAddr is the address the shared SMTP server binds to.
	// INSTANCE-WIDE: the last user who saves it wins.
	ListenAddr string `yaml:"listen_addr"`
}

// DefaultConfig implements plugin.Configurer
func (c *Plugin) DefaultConfig() interface{} {
	return &Config{
		Password:       "",
		Priority:       0,
		DefaultTitle:   "(no subject)",
		StripHTML:      true,
		AllowedSenders: []string{},
		ListenAddr:     defaultListenAddr,
	}
}

// ValidateAndSetConfig implements plugin.Configurer
func (c *Plugin) ValidateAndSetConfig(cfg interface{}) error {
	config, ok := cfg.(*Config)
	if !ok {
		return errors.New("invalid config type")
	}

	if config.Priority < 0 || config.Priority > 10 {
		return fmt.Errorf("priority must be between 0 and 10, got %d", config.Priority)
	}

	for _, sender := range config.AllowedSenders {
		if strings.TrimSpace(sender) == "" {
			return errors.New("allowed_senders must not contain empty entries")
		}
	}

	addr := strings.TrimSpace(config.ListenAddr)
	if addr == "" {
		addr = defaultListenAddr
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("listen_addr %q is not a valid host:port (example: :1025): %w", addr, err)
	}
	config.ListenAddr = addr

	// Apply the address before storing the config: if the port is taken we want
	// the user to see the error in the UI instead of silently keeping the old one.
	if err := ensureServer(addr); err != nil {
		return err
	}

	c.configLock.Lock()
	c.config = config
	c.configLock.Unlock()

	return nil
}

// currentConfig returns the active config, falling back to the defaults when the
// plugin has not been configured yet.
func (c *Plugin) currentConfig() *Config {
	c.configLock.RLock()
	defer c.configLock.RUnlock()

	if c.config == nil {
		return c.DefaultConfig().(*Config)
	}
	return c.config
}

// ensureServer starts the shared SMTP server, restarting it when the address changed.
//
// The listener is created up front so a busy port surfaces as a config error
// rather than disappearing into a goroutine.
func ensureServer(addr string) error {
	serverLock.Lock()
	defer serverLock.Unlock()

	if s != nil && serverAddr == addr {
		return nil
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}

	if s != nil {
		s.Close()
	}

	s = smtp.NewServer(&Backend{})
	s.Addr = addr
	s.Domain = "0.0.0.0"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	serverAddr = addr

	go s.Serve(ln)

	return nil
}

// Plugin is plugin instance
type Plugin struct {
	userCtx    plugin.UserContext
	msgHandler plugin.MessageHandler
	config     *Config
	configLock sync.RWMutex
}

// SetMessageHandler implements plugin.Messenger
// Invoked during initialization
func (c *Plugin) SetMessageHandler(h plugin.MessageHandler) {
	c.msgHandler = h
}

// GetDisplay implements plugin.Displayer and renders usage instructions,
// filled in with the values actually configured for this user.
func (c *Plugin) GetDisplay(location *url.URL) string {
	config := c.currentConfig()

	host := "<gotify-host>"
	if location != nil && location.Hostname() != "" {
		host = location.Hostname()
	}

	_, port, err := net.SplitHostPort(config.ListenAddr)
	if err != nil || port == "" {
		port = "1025"
	}

	password := "any (no password configured)"
	if config.Password != "" {
		password = "the password from this plugin's configuration"
	}

	senders := "any sender"
	if len(config.AllowedSenders) > 0 {
		senders = "only " + strings.Join(config.AllowedSenders, ", ")
	}

	// Assembled line by line: the markdown is full of backticks, which cannot
	// appear inside a Go raw string literal.
	lines := []string{
		"## Sending mail to Gotify",
		"",
		"Point any device that can only send e-mail (NAS, printer, router, UPS, ...)",
		"at this server. Every mail becomes a Gotify message in the **SMTP** application.",
		"",
		"| Setting | Value |",
		"|---|---|",
		fmt.Sprintf("| Server | `%s` |", host),
		fmt.Sprintf("| Port | `%s` |", port),
		"| Encryption | none (plain, STARTTLS is not offered) |",
		"| Authentication | required |",
		fmt.Sprintf("| Username | `%s` |", c.userCtx.Name),
		fmt.Sprintf("| Password | %s |", password),
		fmt.Sprintf("| From / To | ignored, %s accepted |", senders),
		"",
		"The **subject becomes the title**, the **body becomes the message text**.",
		fmt.Sprintf("Messages are created with priority **%d**.", config.Priority),
		"",
		"### Notes",
		"",
		fmt.Sprintf("- The username selects the Gotify account, so send as `%s` to reach this inbox.", c.userCtx.Name),
		"- Your Gotify server has to publish the SMTP port. With Docker that means mapping",
		fmt.Sprintf("  it, for example `- \"%s:%s\"` in the ports section. Prefer binding it to a LAN", port, port),
		"  address instead of 0.0.0.0 unless you really want it reachable from everywhere.",
		"- Never expose this port to the internet: mail is transmitted in the clear.",
		"- `listen_addr` configures the shared SMTP server and therefore affects **all",
		"  users** of this Gotify instance - whoever saves it last wins.",
	}

	return strings.Join(lines, "\n") + "\n"
}

// Enable adds users to the context map which maps to a Plugin.
func (c *Plugin) Enable() error {
	if err := ensureServer(c.currentConfig().ListenAddr); err != nil {
		return err
	}

	userLock.Lock()
	users[c.userCtx.Name] = c
	userLock.Unlock()
	return nil
}

// Disable removes users from the context map.
func (c *Plugin) Disable() error {
	userLock.Lock()
	delete(users, c.userCtx.Name)
	userLock.Unlock()
	return nil
}

// NewGotifyPluginInstance creates a plugin instance for a user context.
func NewGotifyPluginInstance(ctx plugin.UserContext) plugin.Plugin {
	return &Plugin{userCtx: ctx}
}

// The Backend implements SMTP server methods.
type Backend struct {
}

// Login handles a login command with username and password.
func (bkd *Backend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	userLock.RLock()
	instance, ok := users[username]
	userLock.RUnlock()

	if !ok {
		return nil, errors.New("user not found")
	}

	// An empty password keeps the previous behaviour: the username alone is
	// enough. Once a password is configured it is enforced.
	if configured := instance.currentConfig().Password; configured != "" {
		if subtle.ConstantTimeCompare([]byte(configured), []byte(password)) != 1 {
			return nil, errors.New("invalid password")
		}
	}

	return &Session{c: instance}, nil
}

// AnonymousLogin requires clients to authenticate using SMTP AUTH before sending emails
func (bkd *Backend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return nil, smtp.ErrAuthRequired
}

type Session struct {
	c *Plugin
}

func (s *Session) Mail(from string, opts smtp.MailOptions) error {
	if s.c == nil {
		return nil
	}

	allowed := s.c.currentConfig().AllowedSenders
	if len(allowed) == 0 {
		return nil
	}

	from = strings.ToLower(strings.TrimSpace(from))

	for _, entry := range allowed {
		entry = strings.ToLower(strings.TrimSpace(entry))

		// A leading @ matches a whole domain, anything else an exact address.
		if strings.HasPrefix(entry, "@") {
			if strings.HasSuffix(from, entry) {
				return nil
			}
			continue
		}

		if from == entry {
			return nil
		}
	}

	return fmt.Errorf("sender %s is not allowed", from)
}

func (s *Session) Rcpt(to string) error {
	return nil
}

func (s *Session) Data(r io.Reader) error {
	m, err := mail.ReadMessage(r)
	if err != nil {
		return err
	}

	config := s.c.currentConfig()

	var subject string
	if subjectHeader, ok := m.Header["Subject"]; ok && len(subjectHeader) > 0 {
		subject = subjectHeader[0]
	}
	if strings.TrimSpace(subject) == "" {
		subject = config.DefaultTitle
	}

	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))

	var message string
	var isHTML bool

	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		message = ParsePart(m.Body, params["boundary"])
	} else {
		b, err := ioutil.ReadAll(m.Body)
		if err != nil {
			return err
		}
		// Without this, a quoted-printable body keeps its soft line breaks and
		// umlauts arrive as =C3=A4.
		message = decodeTransferEncoding(b, m.Header.Get("Content-Transfer-Encoding"))
		isHTML = strings.HasPrefix(mediaType, "text/html")
	}

	// An HTML-only mail has no text/plain part, so without this the raw markup
	// would end up in the notification.
	if config.StripHTML && (isHTML || looksLikeHTML(message)) {
		message = stripHTML(message)
	}

	if s.c != nil && s.c.msgHandler != nil {
		s.c.msgHandler.SendMessage(plugin.Message{
			Title:    subject,
			Message:  message,
			Priority: config.Priority,
		})
	}

	return nil
}

func (s *Session) Reset() {

}

func (s *Session) Logout() error {
	return nil
}

var (
	htmlTagRe = regexp.MustCompile(`(?is)<[^>]+>`)
	// No backreference here: Go's RE2 engine does not support them, so the
	// closing tag is matched by name instead of by group.
	htmlDropRe   = regexp.MustCompile(`(?is)<\s*(?:script|style|head)\b[^>]*>.*?<\s*/\s*(?:script|style|head)\s*>`)
	htmlBreakRe  = regexp.MustCompile(`(?i)<\s*(br\s*/?|/p|/div|/tr|/h[1-6])\s*>`)
	blankLinesRe = regexp.MustCompile(`\n{3,}`)
	looksHTMLRe  = regexp.MustCompile(`(?is)<\s*(html|body|div|p|br|table)[\s>/]`)
)

func looksLikeHTML(s string) bool {
	return looksHTMLRe.MatchString(s)
}

// stripHTML turns HTML into something readable in a notification. It is
// deliberately simple - no dependency, no attempt at layout.
func stripHTML(s string) string {
	s = htmlDropRe.ReplaceAllString(s, "")
	s = htmlBreakRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	// html.UnescapeString covers every named entity, not just a hand-picked list.
	s = html.UnescapeString(s)

	s = strings.ReplaceAll(s, "\u00a0", " ")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	s = strings.Join(lines, "\n")
	s = blankLinesRe.ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}

// decodeTransferEncoding undoes base64 / quoted-printable encoding so that
// umlauts and soft line breaks do not end up in the notification verbatim.
func decodeTransferEncoding(b []byte, encoding string) string {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		if decoded, err := base64.StdEncoding.DecodeString(string(b)); err == nil {
			return string(decoded)
		}
	case "quoted-printable":
		if decoded, err := ioutil.ReadAll(quotedprintable.NewReader(bytes.NewReader(b))); err == nil {
			return string(decoded)
		}
	}
	return string(b)
}

// ParsePart will find the first text/plain part from a multipart body.
// Adapted from https://github.com/kirabou/parseMIMEemail.go
func ParsePart(body io.Reader, boundary string) string {
	reader := multipart.NewReader(body, boundary)

	if reader == nil {
		return ""
	}

	// Go through each of the MIME part of the message Body with NextPart(),
	for {
		part, err := reader.NextPart()

		if err == io.EOF {
			break
		}

		if err != nil {
			fmt.Println("Error going through the MIME parts -", err)
			break
		}

		mediaType, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))

		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			// This is a new multipart to be handled recursively
			str := ParsePart(part, params["boundary"])

			if str != "" {
				return str
			}
		} else {
			if strings.HasPrefix(mediaType, "text/plain") {
				b, err := ioutil.ReadAll(part)

				if err != nil {
					continue
				}

				return decodeTransferEncoding(b, part.Header.Get("Content-Transfer-Encoding"))
			}
		}
	}

	return ""
}

func main() {
	panic("Program must be compiled as a Go plugin")
}
