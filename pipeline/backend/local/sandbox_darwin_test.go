//go:build darwin

package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.woodpecker-ci.org/woodpecker/v3/pipeline/backend/types"
)

// Exercise the actual backend on macOS, not just the generated profile text.
func TestSandboxConcurrentWorkflows(t *testing.T) {
	require.NoError(t, checkSandbox())
	t.Setenv("WOODPECKER_AGENT_SECRET", "private-agent-canary")
	e := &local{tempDir: t.TempDir(), isolatedHome: true, sandbox: true, os: "darwin", arch: "arm64"}
	ctx := context.Background()
	const count = 4
	states := make([]*workflowState, count)
	for i := range count {
		id := fmt.Sprint(i)
		require.NoError(t, e.SetupWorkflow(ctx, &types.Config{}, id))
		var err error
		states[i], err = e.getWorkflowState(id)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(states[i].baseDir, "private"), []byte("sibling-canary"), 0600))
		t.Cleanup(func() { _ = e.DestroyWorkflow(ctx, &types.Config{}, id) })
	}
	steps := make([]*types.Step, count)
	outputs := make([]chan []byte, count)
	for i := range count {
		peer := filepath.Join(states[(i+1)%count].baseDir, "private")
		script := fmt.Sprintf(`
test -z "${WOODPECKER_AGENT_SECRET+x}"
if cat %q >/dev/null 2>&1; then exit 41; fi
if echo corrupt >%q 2>/dev/null; then exit 42; fi
printf 'int main(void){return 0;}\n' > native.c
/usr/bin/clang native.c -o native
./native
printf ok > "$HOME/result"
sleep 2
echo isolated
`, peer, peer)
		steps[i] = &types.Step{UUID: fmt.Sprint(i), Name: "native-isolation", Type: types.StepTypeCommands, Image: "sh", Commands: []string{script}}
		require.NoError(t, e.StartStep(ctx, steps[i], fmt.Sprint(i)))
		reader, err := e.TailStep(ctx, steps[i], fmt.Sprint(i))
		require.NoError(t, err)
		outputs[i] = make(chan []byte, 1)
		go func(ch chan []byte, r io.Reader) { out, _ := io.ReadAll(r); ch <- out }(outputs[i], reader)
	}
	for i := range count {
		result, err := e.WaitStep(ctx, steps[i], fmt.Sprint(i))
		require.NoError(t, err)
		out := <-outputs[i]
		require.Equal(t, 0, result.ExitCode, "workflow %d: %s", i, out)
		require.FileExists(t, filepath.Join(states[i].homeDir, "result"))
		private, err := os.ReadFile(filepath.Join(states[i].baseDir, "private"))
		require.NoError(t, err)
		require.Equal(t, "sibling-canary", string(private))
	}
}

func TestSandboxCancellationKeepsSiblingAlive(t *testing.T) {
	ctx := context.Background()
	e := &local{tempDir: t.TempDir(), isolatedHome: true, sandbox: true, os: "darwin"}
	steps := []*types.Step{}
	for i := range 2 {
		id := fmt.Sprint(i)
		require.NoError(t, e.SetupWorkflow(ctx, &types.Config{}, id))
		t.Cleanup(func() { _ = e.DestroyWorkflow(ctx, &types.Config{}, id) })
		step := &types.Step{UUID: id, Name: "sleep", Type: types.StepTypeCommands, Image: "sh", Commands: []string{"sleep 2"}}
		require.NoError(t, e.StartStep(ctx, step, id))
		steps = append(steps, step)
	}
	require.NoError(t, e.DestroyStep(ctx, steps[0], "0"))
	state, err := e.WaitStep(ctx, steps[1], "1")
	require.NoError(t, err)
	require.Equal(t, 0, state.ExitCode)
}
