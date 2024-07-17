package jsonrpc2

import (
	"fmt"
	"strings"
)

var errFileClosed = "file already closed"
var errBrokenPipe = "broken pipe"
var oomError = "java.lang.OutOfMemoryError"

func IsRPCClosed(err error) bool {
	var errMsg = err.Error()
	return strings.HasSuffix(errMsg, errFileClosed) || strings.HasSuffix(errMsg, errBrokenPipe)
}

func IsOOMError(err *Error) bool {
	return strings.Contains(fmt.Sprintf("%s", err.Data), oomError)
}
