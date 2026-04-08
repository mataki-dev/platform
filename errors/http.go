package errors

// MapHTTPStatus maps an HTTP status code from an upstream service
// to a semantic error.
func MapHTTPStatus(status int, msg string) *SemanticError {
	switch {
	case status == 404:
		return NewNotFound(msg)
	case status == 403:
		return NewForbidden(msg)
	case status == 409:
		return NewConflict(msg)
	case status == 422:
		return NewInvalidInput(msg)
	case status >= 500:
		return newError("internal", status, msg, nil)
	case status >= 400:
		return newError("invalid_input", status, msg, nil)
	default:
		return NewInternal(msg)
	}
}
