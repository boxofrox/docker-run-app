Description
===========

A simple program to run an arbitrary application within a container and terminate the application when any signal is received from Docker.
The following signals will be sent to the app until it terminates:

* the signal received from Docker,
* SIGTERM
* SIGKILL

Each signal is delayed two seconds.


Usage
=====

    docker-run-app [-hV] [--init-log FILE] [--] COMMAND

      COMMAND         - app and args to execute. app requires full path.
      --              - args after this flag are reserved for COMMAND.
      -h, --help      - print this help message.
      --init-log FILE - write docker-run-app output to FILE.
      -V, --version   - print version info.

Build
=====

Set up your Golang workspace.

    export GOPATH=/your/go/workspace
    go get github.com/boxofrox/docker-run-app
    cd $GOPATH/src/github.com/boxofrox/docker-run-app
    make

Make is requried to set version and build info in the binary executable.

Installation
============

Copy the binary within view of your Dockerfile.

    cp $GOPATH/bin/docker-run-app /path/to/your/dockerfile/

Modify your Dockerfile to add the binary and run it.  Here is an example diff:

```diff

+ ADD docker-run-app /run-app
+
- CMD ["/usr/bin/app", "-a", "-b", "-c"]
+ ENTRYPOINT ["/run-app"]
+ CMD ["--", "/usr/bin/app", "-a", "-b", "-c"]

```

The "--" are optional, but ensure that docker-run-app will not eat flags belonging to your app.

Author
======

Justin Charette <charetjc@gmail.com> (@boxofrox)

License
=======

This program is free software. It comes without any warranty, to
the extent permitted by applicable law. You can redistribute it
and/or modify it under the terms of the Do What the Fuck You Want
to Public License, Version 2, as published by Sam Hocevar. See
http://www.wtfpl.net/ for more details.

