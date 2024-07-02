package jsonrpc2

import (
	"strings"
)

var errFileClosed = "file already closed"
var errBrokenPipe = "broken pipe"

func IsRPCClosed(err error) bool {
	var errMsg = err.Error()
	return strings.HasSuffix(errMsg, errFileClosed) || strings.HasSuffix(errMsg, errBrokenPipe)
}
