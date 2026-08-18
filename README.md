Gotify-SMTP
===========

A plugin for piping email messages into [Gotify](https://gotify.net/) without ever hitting an email service. Inspiration for this comes from MailHog and similar implementations where there is no backing email service, it simply forwards/receives messages as needed.

There are other versions of this (specifically using the API), however this is a standalone plugin that can be loaded by Gotify.

Usage
-----

Download the plugin from the releases page, or build it from source (using the Makefile). Make sure to pick the build matching your Gotify server version - Go plugins only load into the exact version they were compiled against.

Point your application settings at GOTIFY_IP port 1025, with the username being the name of the account you'd like to send messages to. The subject becomes the message title, the body becomes the message text.

The plugin's detail page in Gotify shows these settings filled in with your actual host, port and username, so there is no need to guess.

Configuration
-------------

Every user configures their own instance in the Gotify UI:

| Option | Default | Description |
|---|---|---|
| `password` | empty | Password required for SMTP AUTH. Empty accepts any password, which is how the plugin behaved before this option existed. |
| `priority` | `0` | Priority of the created messages (0-10). Useful if mail should actually make your phone ring. |
| `default_title` | `(no subject)` | Title used when a mail has no Subject header. |
| `strip_html` | `true` | Convert an HTML-only mail into readable text instead of forwarding raw markup. |
| `allowed_senders` | empty | Restrict the envelope sender. Entries are either a full address (`alerts@example.com`) or a domain (`@example.com`). Empty accepts everyone. |
| `listen_addr` | `:1025` | Address the SMTP server binds to. **Instance-wide**: there is a single shared SMTP server, so the last user who saves this wins. |

Changing `listen_addr` rebinds the server immediately. If the port is already taken, saving fails with an error instead of leaving you with a server that silently never started.

Limitations
-----------

HTML-only mail is converted with a deliberately simple tag stripper - it produces readable text, not a faithful rendering. Multipart mail keeps using the `text/plain` part, which is still the better source when the sender provides one.

Authentication is per-user via the `password` option, checked against the SMTP username that selects the target account. Mail is transmitted in the clear (no TLS), so you should still NOT run this as a publicly accessible SMTP server - firewall it to what you need or put it behind a VPN.

Examples
--------

Refer to the [examples file](EXAMPLES.md).
