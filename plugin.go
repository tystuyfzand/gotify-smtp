package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/emersion/go-smtp"
	"github.com/gotify/plugin-api"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"sync"
	"time"
)

var (
	s        *smtp.Server
	users    = make(map[string]*Plugin)
	userLock = &sync.RWMutex{}
)

// GetGotifyPluginInfo returns gotify plugin info
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		Name:       "SMTP",
		ModulePath: "github.com/tystuyfzand/gotify-smtp",
		Author:     "Tyler Stuyfzand",
		Website:    "https://meow.tf",
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

	return &Plugin{userCtx: ctx}
}

// The Backend implements SMTP server methods.
type Backend struct {
}

// Login handles a login command with username and password.
func (bkd *Backend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	userLock.RLock()
	defer userLock.RUnlock()

	if instance, ok := users[username]; ok {
		return &Session{instance}, nil
	}

	return nil, errors.New("user not found")
}

// AnonymousLogin requires clients to authenticate using SMTP AUTH before sending emails
func (bkd *Backend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return nil, smtp.ErrAuthRequired
}

type Session struct {
	c *Plugin
}

func (s *Session) Mail(from string, opts smtp.MailOptions) error {
	return nil
}

func (s *Session) Rcpt(to string) error {
	return nil
}

func (s *Session) Data(r io.Reader) error {
	if m, err := mail.ReadMessage(r); err != nil {
		return err
	} else {
		var subject string

		if subjectHeader, ok := m.Header["Subject"]; ok && len(subjectHeader) > 0 {
			subject = subjectHeader[0]
		}

		mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))

		var message string

		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			message = ParsePart(m.Body, params["boundary"])
		} else {
			b, err := ioutil.ReadAll(m.Body)

			if err != nil {
				return err
			}

			message = string(b)
		}

		if s.c != nil && s.c.msgHandler != nil {
			s.c.msgHandler.SendMessage(plugin.Message{
				Title:   subject,
				Message: message,
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

				encoding := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))

				switch {
				case strings.Compare(encoding, "base64") == 0:
					b, err = base64.StdEncoding.DecodeString(string(b))
					if err != nil {
						continue
					}

				case strings.Compare(encoding, "quoted-printable") == 0:
					b, err = ioutil.ReadAll(quotedprintable.NewReader(bytes.NewReader(b)))
					if err != nil {
						continue
					}
				}

				return string(b)
			}
		}
	}

	return ""
}

func main() {
	panic("Program must be compiled as a Go plugin")
}
