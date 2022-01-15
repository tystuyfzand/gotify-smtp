Examples
========

Nextcloud
---------

This can be done through either the webUI (```Settings```, ```Basic settings```, ```Email server```) or by editing ```config.php``` and adding:
```
    'mail_smtpmode' => 'smtp',
    'mail_sendmailmode' => 'smtp',
    'mail_from_address' => 'example', # example@domain.tld
    'mail_domain' => 'domain.tld', # example@domain.tld
    'mail_smtphost' => '192.168.1.2', # IP of gotify-server
    'mail_smtpport' => '1025', # make sure this port is mapped if using gotify on docker
    'mail_smtpauth' => 1,
    'mail_smtpname' => 'admin', # gotify username to send email to
    'mail_smtppassword' => '', #leave blank
    'mail_smtpauthtype' => 'PLAIN',
```

Postfix
-------

WIP
