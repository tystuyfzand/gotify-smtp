package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/gotify/plugin-api"
	"github.com/microcosm-cc/bluemonday"
)

var (
	s        *smtp.Server
	users    = make(map[string]*Plugin)
	userLock = &sync.RWMutex{}

	policy = bluemonday.StrictPolicy()
)

// GetGotifyPluginInfo returns gotify plugin info
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		Name:       "SMTP",
		ModulePath: "github.com/tystuyfzand/gotify-smtp",
		Author:     "Tyler Stuyfzand",
		Website:    "https://tystuyfzand.com",
	}
}

// startServer sets up the SMTP server.
// This is only called once, and uses usernames to authenticate to different users.
func startServer() {
	s = smtp.NewServer(&Backend{})

	s.Addr = ":1025"
	s.Domain = "0.0.0.0"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	go s.ListenAndServe()
}

// Plugin is plugin instance
type Plugin struct {
	userCtx    plugin.UserContext
	config     *Config
	msgHandler plugin.MessageHandler
}

// SetMessageHandler implements plugin.Messenger
// Invoked during initialization
func (c *Plugin) SetMessageHandler(h plugin.MessageHandler) {
	c.msgHandler = h
}

// Enable adds users to the context map which maps to a Plugin.
func (c *Plugin) Enable() error {
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
	if s == nil {
		startServer()
	}

	p := &Plugin{userCtx: ctx}

	p.config = p.DefaultConfig().(*Config)

	return p
}

// The Backend implements SMTP server methods.
type Backend struct {
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{}, nil
}

type Session struct {
	c    *Plugin
	auth bool
}

func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *Session) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		userLock.RLock()
		defer userLock.RUnlock()

		if instance, ok := users[username]; ok {

			if instance.config.Password != "" {
				if subtle.ConstantTimeCompare([]byte(password), []byte(instance.config.Password)) != 1 {
					return smtp.ErrAuthFailed
				}
			}

			s.c = instance
			return nil
		}
		return nil
	}), nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	if s.c == nil {
		return smtp.ErrAuthRequired
	}

	if len(s.c.config.AllowedSenders) > 0 {
		// Validate senders based on allow list
		for _, sender := range s.c.config.AllowedSenders {
			if sender[0] == '@' {
				if strings.HasSuffix(from, sender) {
					return nil
				}
			}

			if sender == from {
				return nil
			}
		}
	}

	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.c == nil {
		return smtp.ErrAuthRequired
	}

	return nil
}

func (s *Session) Data(r io.Reader) error {
	if s.c == nil {
		return smtp.ErrAuthRequired
	}

	m, err := mail.ReadMessage(r)

	if err != nil {
		return err
	}

	var subject string

	if subjectHeader, ok := m.Header["Subject"]; ok && len(subjectHeader) > 0 {
		subject = subjectHeader[0]
	}

	if subject == "" {
		subject = s.c.config.DefaultTitle
	}

	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))

	var message string

	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		availableTypes, err := ParsePart(m.Body, params["boundary"])

		if err != nil {
			return err
		}

		if text, ok := availableTypes["text/plain"]; ok {
			message = text
		} else if html, ok := availableTypes["text/html"]; ok {
			message = policy.Sanitize(html)
		} else {
			keys := slices.Collect(maps.Keys(availableTypes))

			message = "<unknown body type, available types: " + strings.Join(keys, ", ") + ">"
		}
	} else {
		b, err := io.ReadAll(m.Body)

		if err != nil {
			return err
		}

		if idx := strings.Index(mediaType, ";"); idx != -1 {
			mediaType = mediaType[:idx]
		}

		message = string(b)

		if mediaType == "text/html" {
			message = policy.Sanitize(message)
		}

		if s.c.config.PrependSender {
			sender := m.Header.Get("From")

			if sender != "" {
				message = fmt.Sprintf("From: %s\n\n%s", sender, message)
			}
		}

		if s.c != nil && s.c.msgHandler != nil {
			return s.c.msgHandler.SendMessage(plugin.Message{
				Title:    subject,
				Message:  message,
				Priority: s.c.config.Priority,
				Extras: map[string]interface{}{
					"smtp::sender": m.Header.Get("From"),
				},
			})
		}
	}

	return nil
}

func (s *Session) Reset() {

}

func (s *Session) Logout() error {
	return nil
}

// ParsePart will find the first text/plain part from a multipart body.
// Adapted from https://github.com/kirabou/parseMIMEemail.go
func ParsePart(body io.Reader, boundary string) (map[string]string, error) {
	reader := multipart.NewReader(body, boundary)

	ret := make(map[string]string)

	// Go through each of the MIME part of the message Body with NextPart(),
	for {
		part, err := reader.NextPart()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}

		mediaType, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))

		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			// This is a new multipart to be handled recursively
			parts, err := ParsePart(part, params["boundary"])

			if err != nil {
				return nil, err
			}

			for key, part := range parts {
				ret[key] = part
			}
		} else {
			b, err := io.ReadAll(part)

			if err != nil {
				continue
			}

			encoding := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))

			b, err = decodeMessageEncoding(encoding, b)

			if err != nil {
				// Tried to decide what to do in this situation, best to just discard it?
				continue
			}

			ret[mediaType] = string(b)
		}
	}

	return ret, nil
}

func decodeMessageEncoding(encoding string, b []byte) ([]byte, error) {
	switch encoding {
	case "base64":
		return base64.StdEncoding.DecodeString(string(b))

	case "quoted-printable":
		r := quotedprintable.NewReader(bytes.NewReader(b))

		return io.ReadAll(r)
	}

	return b, nil
}

func main() {
	panic("Program must be compiled as a Go plugin")
}
