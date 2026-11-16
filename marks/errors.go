package marks

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
)

// The app's own errors. Each carries 1 Connect code, so that the private
// socket and the MCP tools report the same code and the same text, and the
// error interceptor of internal/rpc keeps it as it is.

// invalidArgument returns the app's invalid_argument error: a value of the
// request breaks 1 of the app's rules.
func invalidArgument(format string, args ...any) error {
	return newError(connect.CodeInvalidArgument, format, args...)
}

// notFound returns the app's not_found error: the bookmark or the folder does
// not exist.
func notFound(format string, args ...any) error {
	return newError(connect.CodeNotFound, format, args...)
}

// alreadyExists returns the app's already_exists error: the folder, or the URL
// in its folder, is there already.
func alreadyExists(format string, args ...any) error {
	return newError(connect.CodeAlreadyExists, format, args...)
}

// failedPrecondition returns the app's failed_precondition error: the state of
// the app refuses the call, such as a folder that is not empty.
func failedPrecondition(format string, args ...any) error {
	return newError(connect.CodeFailedPrecondition, format, args...)
}

// internal returns the app's internal error: a failure that the app cannot
// blame on the request.
func internal(format string, args ...any) error {
	return newError(connect.CodeInternal, format, args...)
}

// newError returns 1 of the app's errors: 1 Connect error of the code and the
// text that the format and the arguments give.
func newError(code connect.Code, format string, args ...any) error {
	text := fmt.Sprintf(format, args...)
	return connect.NewError(code, errors.New(text))
}
