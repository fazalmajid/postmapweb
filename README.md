# Postmapweb: web-based, self-service postfix virtual map management

## Features

* Lets you delegate administration of a portion of a virtual domain map to
  the user(s) responsible for the domain.
* User-friendly interface resembling an Excel spreadsheet
* Meant to run as a standalone service owned by the Postfix user (or at least
  a user or group with write access to the Postfix config files), and thus
  does not require granting your web server any particular privileges.
* Fully self-contained executable, no dependencies

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
