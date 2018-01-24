# Postmapweb: web-based, self-service postfix virtual map management

## Features

* Lets you delegate administration of a portion of a virtual domain map to
  the user(s) responsible for the domain.
* User-friendly interface resembling an Excel spreadsheet
* Meant to run as a standalone service owned by the Postfix user (or at least
  a user or group with write access to the Postfix config files), and thus
  does not require granting your web server any particular privileges.
* Fully self-contained executable, no dependencies
* Will allow users to create aliases for valid RFC-5322 email addresses or use
  existing local addresses, but not dangerous functionality like command
  delivery
* manages a spam map file in access(5) format when the recipient is `spam`

## Installation

### Dependencies

#### Required
* Go (tested on 1.5)
* Git

#### Optional
* Node.js (for Bower, to rebuild the Handsontable CSS/JS)

## Building postmapweb
* Run "make".

## Configuration

Postmapweb is configured using a JSON file, by default
`/etc/postfix/postmapweb.json`. It can handle multiple domains, but only one
map file per domain.

To create the config file (if it does not already exist), add a domain to it
and set up a password for it (or change the password):

    postmapweb -c <config file> -d <domain> -m <Postfix map file>

To start the server:

    postmapweb -c <config file> -p :<port>

**Warning:** postmapweb **must** be run behind a HTTPS reverse proxy server,
e.g. nginx or HAProxy. Do not run it without encryption or expose it to the
Internet by binding it elsewhere than localhost!

The config files in `/etc/postfix` are owned by `root`, not `postfix`, and it
is not good practice to run a server such as this as `root`. Fortunately the
`virtual_alias_maps` Postfix configuration directive accepts multiple maps. My
recommended configuration is to create a new user `postmapweb` in the
`postfix` group, create a subdirectory `/etc/postfix/domains` owned by
`postmapweb` and create one virtual map file per domain there. Make sure you
remove any entries from `/etc/postfix/virtual` when you migrate them to
`/etc/postfix/domains/<example.com>` as Postfix looks up aliases in the order
of the files, and thus any change to an alias in
`/etc/postfix/domains/<example.com>` would be preempted by the leftover entry
in `/etc/postfix/virtual`.

Thus:

    (backup your /etc/postfix directory)
    useradd -d /etc/postfix/domains -s /bin/sh -G postfix postmapweb
    (or whatever the equivalent is on your operating system)
    mkdir /etc/postfix/domains
    grep example.com /etc/postfix/virtual > /etc/postfix/domains/example.com
    postmap /etc/postfix/domains/example.com
    chown -R postmapweb:postfix /etc/postfix/domains
    (assuming your existing virtual_alias_maps = dbm:/etc/postfix/virtual, adjust to taste)
    postconf -e virtual_alias_maps=dbm:/etc/postfix/virtual,dbm:/etc/postfix/domains/example.com
    postfix reload
    grep -v example.com /etc/postfix/virtual > /tmp/foo;cat /tmp/foo > /etc/postfix/virtual
    postmap /etc/postfix/virtual
    postfix reload
    postmapweb -c /etc/postfix/postmapweb.json -d example.com -m /etc/postfix/domains/example.com
    postmapweb -v -c /etc/postfix/postmapweb.json

## Usage

      -c string
            config file to use (default "/etc/postfix/postmapweb.json")
      -cpuprofile string
            write cpu profile to file
      -d string
            add domain user
      -m string
            virtual domain map to use with -d (default "/etc/postfix/virtual")
      -p string
            host address and port to bind to (default "localhost:8080")
      -v    verbose logging
      -w string
            password to use with -d (insecure!)

## Credits

* Go: https://golang.org/
* HandsOnTable: http://handsontable.com/
* And of course Wietse Venema's Postfix: http://www.postfix.org/
