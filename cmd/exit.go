package cmd

import "fmt"

// ExitError signals that the process should exit with a specific code.
// Returning this from a RunE function allows deferred cleanup to run
// before the process exits, unlike calling os.Exit directly.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}
