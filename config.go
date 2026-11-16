package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"text/template"
)

var (
	ErrInvalidConfigType = errors.New("invalid config type")
	ErrInvalidPriority   = errors.New("invalid priority, must be between 0 and 10")

	//go:embed display.md
	displayMd string
)

type Config struct {
	// Password defines an SMTP Password for this user.
	Password string `yaml:"password"`

	// Priority overrides the message priority in Gotify.
	Priority int `yaml:"priority"`

	// AllowedSenders adds an option to only allow certain senders.
	AllowedSenders []string `yaml:"allowed_senders"`

	// DefaultTitle defines the title to use when no subject is passed.
	DefaultTitle string `yaml:"default_title"`

	// PrependSender allows you to prepend the sender information to the message.
	PrependSender bool `yaml:"prepend_sender"`
}

func (c *Plugin) DefaultConfig() interface{} {
	return &Config{
		Password:       "",
		Priority:       0,
		AllowedSenders: []string{},
		DefaultTitle:   "(no subject)",
		PrependSender:  false,
	}
}

func (c *Plugin) ValidateAndSetConfig(config interface{}) error {
	cfg, ok := config.(*Config)

	if !ok {
		return ErrInvalidConfigType
	}

	if cfg.Priority < 0 || cfg.Priority > 10 {
		return ErrInvalidPriority
	}

	for i, sender := range cfg.AllowedSenders {
		if sender == "" {
			return fmt.Errorf("sender %d is empty", i)
		}

		addr, err := mail.ParseAddress(sender)

		if err != nil {
			return fmt.Errorf("invalid sender address %q: %v", sender, err)
		}

		cfg.AllowedSenders[i] = addr.Address
	}

	c.config = cfg

	return nil
}

func (c *Plugin) GetDisplay(location *url.URL) string {
	tpl := template.Must(template.New("display").Parse(displayMd))

	host := "<gotify-host>"

	if location != nil && location.Hostname() != "" {
		host = location.Hostname()
	}

	password := "any (none configured)"

	if c.config.Password != "" {
		password = "<configured password>"
	}

	var b bytes.Buffer

	type setting struct {
		Name  string
		Value string
	}

	m := map[string]any{
		"username": c.userCtx.Name,
		"priority": c.config.Priority,
		"subject":  c.config.DefaultTitle,
		"settings": []setting{
			{Name: "Host", Value: host},
			{Name: "Port", Value: "1025"},
			{Name: "Encryption", Value: "none"},
			{Name: "Authentication", Value: "plain, required"},
			{Name: "Username", Value: c.userCtx.Name},
			{Name: "Password", Value: password},
		},
	}

	if err := tpl.Execute(&b, m); err != nil {
		return fmt.Sprintf("Error rendering display template: %v", err)
	}

	return b.String()
}
