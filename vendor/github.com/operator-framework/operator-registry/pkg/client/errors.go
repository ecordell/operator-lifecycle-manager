package client


const (
	HealthErrReasonUnrecoveredTransient = "UnrecoveredTransient"
	HealthErrReasonConnection = "ConnectionError"
	HealhtErrReasonUnknown = "Unknown"

)

// HealthError is used to represent error types for health checks
type HealthError struct {
	ClientState string
	Reason string
	Message string
}

var _ error = HealthError{}

// Error implements the Error interface.
func (e HealthError) Error() string {
	return fmt.Sprintf("%s: %s", e.ClientState, e.Message)
}

// unrecoverableErrors are the set of errors that mean we can't recover an install strategy
var unrecoverableErrors = map[string]struct{}{
	HealthErrorUnrecoveredTransientFailure: {},
}

func NewHealthError(conn *grpc.ClientConn, reason string, msg string) HealthError {
	return HealthError{
		ClientState: conn.GetState(),
		Reason: reason,
		Message: msg,
	}
}

// IsErrorUnrecoverable reports if a given strategy error is one of the predefined unrecoverable types
func IsErrorUnrecoverable(err error) bool {
	if err == nil {
		return false
	}
	_, ok := unrecoverableErrors[reasonForError(err)]
	return ok
}

func reasonForError(err error) string {
	switch t := err.(type) {
	case HealthError:
		return t.Reason
	case *HealthError:
		return t.Reason
	}
	return HealhtErrReasonUnknown
}
