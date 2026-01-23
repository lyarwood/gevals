package eval

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAgentSpec(t *testing.T) {
	tests := map[string]struct {
		setupEnv    func()
		cleanupEnv  func()
		spec        *EvalSpec
		expectErr   bool
		errContains string
		validate    func(t *testing.T, runner *evalRunner)
	}{
		"inline agent - builtin.claude-code": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "builtin.claude-code",
					},
				},
			},
			validate: func(t *testing.T, runner *evalRunner) {
				agentSpec, err := runner.loadAgentSpec()
				// Note: This may fail with environment validation error if claude binary is not in PATH
				// That's expected behavior - the test will skip validation if claude is not available
				if err != nil {
					if assert.Contains(t, err.Error(), "environment validation failed") {
						t.Skip("claude binary not in PATH, skipping test")
					}
					require.NoError(t, err) // Fail if it's a different error
				}
				require.NotNil(t, agentSpec)
				assert.Equal(t, "claude-code", agentSpec.Metadata.Name)
			},
		},
		"inline agent - builtin.openai-agent with valid env": {
			setupEnv: func() {
				os.Setenv("MODEL_BASE_URL", "https://api.openai.com/v1")
				os.Setenv("MODEL_KEY", "test-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("MODEL_BASE_URL")
				os.Unsetenv("MODEL_KEY")
			},
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type:  "builtin.openai-agent",
						Model: "gpt-4",
					},
				},
			},
			validate: func(t *testing.T, runner *evalRunner) {
				agentSpec, err := runner.loadAgentSpec()
				require.NoError(t, err)
				require.NotNil(t, agentSpec)
				assert.Equal(t, "openai-agent-gpt-4", agentSpec.Metadata.Name)
				require.NotNil(t, agentSpec.Builtin)
				assert.Equal(t, "openai-agent", agentSpec.Builtin.Type)
				assert.Equal(t, "gpt-4", agentSpec.Builtin.Model)
			},
		},
		"inline agent - builtin.openai-agent without model": {
			setupEnv: func() {
				os.Setenv("MODEL_BASE_URL", "https://api.openai.com/v1")
				os.Setenv("MODEL_KEY", "test-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("MODEL_BASE_URL")
				os.Unsetenv("MODEL_KEY")
			},
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "builtin.openai-agent",
					},
				},
			},
			expectErr:   true,
			errContains: "requires a model to be specified",
		},
		"inline agent - unknown type": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "builtin.unknown-agent",
					},
				},
			},
			expectErr:   true,
			errContains: "unknown builtin agent type",
		},
		"no agent configuration": {
			spec: &EvalSpec{
				Config: EvalConfig{},
			},
			expectErr:   true,
			errContains: "agent must be specified",
		},
		"file agent without path": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "file",
					},
				},
			},
			expectErr:   true,
			errContains: "path must be specified when agent type is 'file'",
		},
		"invalid agent type format": {
			spec: &EvalSpec{
				Config: EvalConfig{
					Agent: &AgentRef{
						Type: "invalid-format",
					},
				},
			},
			expectErr:   true,
			errContains: "agent type must be either 'file' or 'builtin.X' format",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.setupEnv != nil {
				tc.setupEnv()
			}
			if tc.cleanupEnv != nil {
				defer tc.cleanupEnv()
			}

			runner := &evalRunner{
				spec: tc.spec,
			}

			if tc.expectErr {
				_, err := runner.loadAgentSpec()
				require.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			if tc.validate != nil {
				tc.validate(t, runner)
			}
		})
	}
}

func TestCollectTaskConfigs(t *testing.T) {
	// Create temporary directory structure
	tmpDir, err := os.MkdirTemp("", "eval-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create task files
	tasksDir := filepath.Join(tmpDir, "tasks")
	require.NoError(t, os.MkdirAll(tasksDir, 0755))

	task1Content := `
apiVersion: gevals/v1alpha2
kind: Task
metadata:
  name: task-one
  difficulty: easy
spec:
  prompt:
    inline: "Do something"
`
	task2Content := `
apiVersion: gevals/v1alpha2
kind: Task
metadata:
  name: task-two
  difficulty: medium
spec:
  prompt:
    inline: "Do something else"
`
	require.NoError(t, os.WriteFile(filepath.Join(tasksDir, "task1.yaml"), []byte(task1Content), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tasksDir, "task2.yaml"), []byte(task2Content), 0644))

	// Create a TaskSet file
	tasksetContent := `
kind: TaskSet
metadata:
  name: test-taskset
config:
  tasks:
    - glob: tasks/*.yaml
  assertions:
    minToolCalls: 1
    maxToolCalls: 5
`
	tasksetPath := filepath.Join(tmpDir, "taskset.yaml")
	require.NoError(t, os.WriteFile(tasksetPath, []byte(tasksetContent), 0644))

	t.Run("inline TaskSets", func(t *testing.T) {
		minCalls := 2
		spec := &EvalSpec{
			Config: EvalConfig{
				TaskSets: []TaskSet{
					{
						Glob: filepath.Join(tasksDir, "*.yaml"),
						Assertions: &TaskAssertions{
							MinToolCalls: &minCalls,
						},
					},
				},
			},
		}

		runner := &evalRunner{spec: spec}
		rx := regexp.MustCompile(".")

		configs, err := runner.collectTaskConfigs(rx)
		require.NoError(t, err)
		assert.Len(t, configs, 2)

		// Verify assertions are passed through
		for _, cfg := range configs {
			require.NotNil(t, cfg.assertions)
			require.NotNil(t, cfg.assertions.MinToolCalls)
			assert.Equal(t, 2, *cfg.assertions.MinToolCalls)
		}
	})

	t.Run("TaskSetRefs", func(t *testing.T) {
		spec := &EvalSpec{
			Config: EvalConfig{
				TaskSetRefs: []TaskSetRef{
					{Path: tasksetPath},
				},
			},
		}

		runner := &evalRunner{spec: spec}
		rx := regexp.MustCompile(".")

		configs, err := runner.collectTaskConfigs(rx)
		require.NoError(t, err)
		assert.Len(t, configs, 2)

		// Verify assertions from TaskSet file are converted and passed through
		for _, cfg := range configs {
			require.NotNil(t, cfg.assertions)
			require.NotNil(t, cfg.assertions.MinToolCalls)
			assert.Equal(t, 1, *cfg.assertions.MinToolCalls)
			require.NotNil(t, cfg.assertions.MaxToolCalls)
			assert.Equal(t, 5, *cfg.assertions.MaxToolCalls)
		}
	})

	t.Run("task name filter", func(t *testing.T) {
		spec := &EvalSpec{
			Config: EvalConfig{
				TaskSets: []TaskSet{
					{Glob: filepath.Join(tasksDir, "*.yaml")},
				},
			},
		}

		runner := &evalRunner{spec: spec}
		rx := regexp.MustCompile("task-one")

		configs, err := runner.collectTaskConfigs(rx)
		require.NoError(t, err)
		assert.Len(t, configs, 1)
		assert.Equal(t, "task-one", configs[0].spec.Metadata.Name)
	})

	t.Run("mixed inline and refs", func(t *testing.T) {
		inlineMin := 10
		spec := &EvalSpec{
			Config: EvalConfig{
				TaskSetRefs: []TaskSetRef{
					{Path: tasksetPath},
				},
				TaskSets: []TaskSet{
					{
						Path: filepath.Join(tasksDir, "task1.yaml"),
						Assertions: &TaskAssertions{
							MinToolCalls: &inlineMin,
						},
					},
				},
			},
		}

		runner := &evalRunner{spec: spec}
		rx := regexp.MustCompile(".")

		configs, err := runner.collectTaskConfigs(rx)
		require.NoError(t, err)
		// 2 from TaskSetRef + 1 from inline TaskSet (task1 appears twice)
		assert.Len(t, configs, 3)
	})
}
