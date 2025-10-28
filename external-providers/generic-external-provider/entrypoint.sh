#!/bin/sh

/usr/local/bin/gopls -listen='unix;/tmp/go-pls-pipe-test' -logfile=auto &
/usr/local/bin/generic-external-provider "$@"
