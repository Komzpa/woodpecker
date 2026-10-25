//go:build !darwin

package local

import (
	"context"
	"fmt"
	"os/exec"
)

func checkSandbox() error { return fmt.Errorf("local sandbox requires macOS") }
func sandboxCommand(_ context.Context, _ *exec.Cmd, _ *workflowState) (*exec.Cmd, error) {
	return nil, checkSandbox()
}
