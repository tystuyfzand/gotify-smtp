Gotify-SMTP
===========

A plugin for piping email messages into [Gotify](https://gotify.net/) without ever hitting an email service. Inspiration for this comes from MailHog and similar implementations where there is no backing email service, it simply forwards/receives messages as needed.

There are other versions of this (specifically using the API), however this is a standalone plugin that can be loaded by Gotify.

Usage
-----

Download the plugin from the releases page, or build it from source (using the Makefile).

Point your application settings at GOTIFY_IP port 1025, with the username being the name of the account you'd like to send messages to.

Limitations
-----------

Currently, HTML messages aren't supported. Markdown might be possible, but currently not planned as most if not all messages include a text/plain variation.

There is no authentication, besides allowing for specific names to be routed to. As such you should NOT run this as a public accessible SMTP server, and firewall it to what you need/put it behind a VPN. If Gotify supports authentication in the future, I'll add it and use the token to validate logins.

Examples
--------

Refer to the [examples file](EXAMPLES.md).