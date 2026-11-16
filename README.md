Gotify-SMTP
===========

A plugin for piping email messages into [Gotify](https://gotify.net/) without ever hitting an email service. Inspiration for this comes from MailHog and similar implementations where there is no backing email service, it simply forwards/receives messages as needed.

There are other versions of this (specifically using the API), however this is a standalone plugin that can be loaded by Gotify.

Usage
-----

Download the plugin from the releases page, or build it from source (using the Makefile).

Point your application settings at GOTIFY_IP port 1025, with the username being the name of the account you'd like to send messages to.

Configuration
-------------

Users configure their own settings per-account in the Gotify UI.

| Option | Default | Description                                                                                                                               |
|--------|---------|-------------------------------------------------------------------------------------------------------------------------------------------|
| `password` | empty | Authentication password for SMTP. Only "PLAIN" auth supported.                                                                            |
| `priority` | 0 | The priority of the Gotify message.                                                                                                       |
| `default_title` | `(no subject)` | The default title of messages when a subject is not provided.                                                                             |
| `allowed_senders` | empty | A list of allowed senders, either full addresses (`admin@example.com`) or domain only (`@example.com`). Leave empty to allow all senders. |
| `prepend_sender` | false | Prepend sender information to the message for identification of sources                                                                   |

Limitations
-----------

HTML-only mail is converted using [bluemonday](https://github.com/microcosm-cc/bluemonday) to strip HTML content. 
If `text/plain` is supplied, it will still be prioritized above HTML.

Authentication is configurable per-user using the `password` option. Current authentication only allows for `PLAIN`, and 
is transmitted in clear text as TLS is not supported. It is advised to only run this on a local network, or via a VPN.

Examples
--------

Refer to the [examples file](EXAMPLES.md).