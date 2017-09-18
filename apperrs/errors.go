package apperrs

import "errors"

// Predefined errors.
var (
	ErrPollingConnectionClosed = errors.New("polling connection is closed")
	ErrPollingRequestNotFound  = errors.New("polling request not found")
	ErrWriteEmptyData          = errors.New("write empty data to polling request")
)
