package cli

// ExitCodeError makes the process exit with Code instead of 1.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e ExitCodeError) Error() string { return e.Message }
