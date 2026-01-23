package taskset

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead(t *testing.T) {
	tests := map[string]struct {
		yaml        string
		expectErr   bool
		errContains string
		validate    func(t *testing.T, spec *TaskSetSpec)
	}{
		"valid taskset with glob": {
			yaml: `
kind: TaskSet
metadata:
  name: my-tasks
config:
  tasks:
    - glob: "tasks/*.yaml"
  assertions:
    minToolCalls: 1
    maxToolCalls: 10
`,
			validate: func(t *testing.T, spec *TaskSetSpec) {
				assert.Equal(t, "my-tasks", spec.Metadata.Name)
				require.Len(t, spec.Config.Tasks, 1)
				assert.Contains(t, spec.Config.Tasks[0].Glob, "tasks/*.yaml")
				require.NotNil(t, spec.Config.Assertions)
				require.NotNil(t, spec.Config.Assertions.MinToolCalls)
				assert.Equal(t, 1, *spec.Config.Assertions.MinToolCalls)
				require.NotNil(t, spec.Config.Assertions.MaxToolCalls)
				assert.Equal(t, 10, *spec.Config.Assertions.MaxToolCalls)
			},
		},
		"valid taskset with path": {
			yaml: `
kind: TaskSet
metadata:
  name: single-task
config:
  tasks:
    - path: "tasks/specific.yaml"
`,
			validate: func(t *testing.T, spec *TaskSetSpec) {
				assert.Equal(t, "single-task", spec.Metadata.Name)
				require.Len(t, spec.Config.Tasks, 1)
				assert.Contains(t, spec.Config.Tasks[0].Path, "tasks/specific.yaml")
			},
		},
		"valid taskset with multiple tasks": {
			yaml: `
kind: TaskSet
metadata:
  name: multi-tasks
config:
  tasks:
    - glob: "easy/*.yaml"
    - glob: "medium/*.yaml"
    - path: "hard/specific.yaml"
`,
			validate: func(t *testing.T, spec *TaskSetSpec) {
				assert.Equal(t, "multi-tasks", spec.Metadata.Name)
				require.Len(t, spec.Config.Tasks, 3)
			},
		},
		"valid taskset with full assertions": {
			yaml: `
kind: TaskSet
metadata:
  name: full-assertions
config:
  tasks:
    - glob: "*.yaml"
  assertions:
    toolsUsed:
      - server: kubernetes
        tool: get_pods
    toolsNotUsed:
      - server: kubernetes
        toolPattern: "delete_.*"
    requireAny:
      - server: kubernetes
        tool: list_namespaces
    resourcesRead:
      - server: filesystem
        uri: "file:///etc/config"
    promptsUsed:
      - server: prompts
        prompt: system-prompt
    callOrder:
      - type: tool
        server: kubernetes
        name: get_namespaces
    noDuplicateCalls: true
`,
			validate: func(t *testing.T, spec *TaskSetSpec) {
				a := spec.Config.Assertions
				require.NotNil(t, a)
				require.Len(t, a.ToolsUsed, 1)
				assert.Equal(t, "kubernetes", a.ToolsUsed[0].Server)
				assert.Equal(t, "get_pods", a.ToolsUsed[0].Tool)
				require.Len(t, a.ToolsNotUsed, 1)
				assert.Equal(t, "delete_.*", a.ToolsNotUsed[0].ToolPattern)
				require.Len(t, a.RequireAny, 1)
				require.Len(t, a.ResourcesRead, 1)
				require.Len(t, a.PromptsUsed, 1)
				require.Len(t, a.CallOrder, 1)
				assert.True(t, a.NoDuplicateCalls)
			},
		},
		"invalid kind": {
			yaml: `
kind: WrongKind
metadata:
  name: test
config:
  tasks:
    - glob: "*.yaml"
`,
			expectErr:   true,
			errContains: "invalid kind",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			spec, err := Read([]byte(tc.yaml), "/tmp")

			if tc.expectErr {
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			if tc.validate != nil {
				tc.validate(t, spec)
			}
		})
	}
}

func TestFromFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "taskset-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test taskset file
	tasksetContent := `
kind: TaskSet
metadata:
  name: file-test
config:
  tasks:
    - glob: "tasks/*.yaml"
    - path: "specific/task.yaml"
  assertions:
    minToolCalls: 1
`
	tasksetPath := filepath.Join(tmpDir, "taskset.yaml")
	err = os.WriteFile(tasksetPath, []byte(tasksetContent), 0644)
	require.NoError(t, err)

	// Load the taskset
	spec, err := FromFile(tasksetPath)
	require.NoError(t, err)

	assert.Equal(t, "file-test", spec.Metadata.Name)
	assert.Equal(t, tmpDir, spec.BasePath())

	// Verify paths are resolved to absolute
	require.Len(t, spec.Config.Tasks, 2)
	assert.Equal(t, filepath.Join(tmpDir, "tasks/*.yaml"), spec.Config.Tasks[0].Glob)
	assert.Equal(t, filepath.Join(tmpDir, "specific/task.yaml"), spec.Config.Tasks[1].Path)
}

func TestFromFile_NotFound(t *testing.T) {
	_, err := FromFile("/nonexistent/path/taskset.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")
}
