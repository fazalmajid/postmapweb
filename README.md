**A web-based tool to allow users to manage their postfix virtual maps.**

*Features:*

* Lets you delegate administration of a portion of a virtual domain map to
  the user(s) responsible for the domain.
* Meant to run as a service owned by the Postfix user (or at least a user or
  group with write access to the Postfix config files.


*Dependencies:*
* Go (tested on 1.5)
* Node.js (for Bower)
* Git

*To build:*

* Run "make".

*Configuration*

Configured using a JSON file.
