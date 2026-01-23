package taskset

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/genmcp/gevals/pkg/util"
	"sigs.k8s.io/yaml"
)

const (
	KindTaskSet = "TaskSet"
)

// TaskSetSpec defines a collection of tasks with shared assertions.
// This can be defined in a standalone YAML file and referenced from an Eval.
type TaskSetSpec struct {
	util.TypeMeta `json:",inline"`
	Metadata      TaskSetMetadata `json:"metadata"`
	Config        TaskSetConfig   `json:"config"`

	// basePath is the directory containing the taskset file, used for resolving relative paths
	basePath string
}

// BasePath returns the directory containing the taskset file
func (s *TaskSetSpec) BasePath() string {
	return s.basePath
}

type TaskSetMetadata struct {
	Name string `json:"name"`
}

// TaskSetConfig contains the tasks and assertions for a TaskSet
type TaskSetConfig struct {
	// Tasks defines the task references in this set
	Tasks []TaskRef `json:"tasks"`

	// Assertions to apply to all tasks in this set
	Assertions *TaskAssertions `json:"assertions,omitempty"`
}

// TaskRef specifies how to find tasks - either by glob pattern or direct path
type TaskRef struct {
	// Exactly one of Glob or Path must be set
	Glob string `json:"glob,omitempty"`
	Path string `json:"path,omitempty"`
}

// TaskAssertions defines validation rules for task execution.
// These are copied from eval package to avoid circular imports.
type TaskAssertions struct {
	// Tool assertions
	ToolsUsed    []ToolAssertion `json:"toolsUsed,omitempty"`
	RequireAny   []ToolAssertion `json:"requireAny,omitempty"`
	ToolsNotUsed []ToolAssertion `json:"toolsNotUsed,omitempty"`
	MinToolCalls *int            `json:"minToolCalls,omitempty"`
	MaxToolCalls *int            `json:"maxToolCalls,omitempty"`

	// Resource assertions
	ResourcesRead    []ResourceAssertion `json:"resourcesRead,omitempty"`
	ResourcesNotRead []ResourceAssertion `json:"resourcesNotRead,omitempty"`

	// Prompt assertions
	PromptsUsed    []PromptAssertion `json:"promptsUsed,omitempty"`
	PromptsNotUsed []PromptAssertion `json:"promptsNotUsed,omitempty"`

	// Order assertions
	CallOrder []CallOrderAssertion `json:"callOrder,omitempty"`

	// Efficiency assertions
	NoDuplicateCalls bool `json:"noDuplicateCalls,omitempty"`
}

type ToolAssertion struct {
	Server string `json:"server"`

	// Exactly one of Tool or ToolPattern should be set
	// If neither is set, matches any tool from the server
	Tool        string `json:"tool,omitempty"`
	ToolPattern string `json:"toolPattern,omitempty"` // regex pattern
}

type ResourceAssertion struct {
	Server string `json:"server"`

	// Exactly one of URI or URIPattern should be set
	// If neither is set, matches any resource from the server
	URI        string `json:"uri,omitempty"`
	URIPattern string `json:"uriPattern,omitempty"` // regex pattern
}

type PromptAssertion struct {
	Server string `json:"server"`

	// Exactly one of Prompt or PromptPattern should be set
	// If neither is set, matches any prompt from the server
	Prompt        string `json:"prompt,omitempty"`
	PromptPattern string `json:"promptPattern,omitempty"`
}

type CallOrderAssertion struct {
	Type   string `json:"type"` // "tool", "resource", "prompt"
	Server string `json:"server"`
	Name   string `json:"name"`
}

func Read(data []byte, basePath string) (*TaskSetSpec, error) {
	spec := &TaskSetSpec{}

	err := yaml.Unmarshal(data, spec)
	if err != nil {
		return nil, err
	}

	if err := spec.TypeMeta.Validate(KindTaskSet); err != nil {
		return nil, err
	}

	// Store the base path for later use
	spec.basePath = basePath

	// Resolve task paths and globs
	for i := range spec.Config.Tasks {
		if spec.Config.Tasks[i].Path != "" {
			if err := resolveFilePath(&spec.Config.Tasks[i].Path, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve task path at index %d: %w", i, err)
			}
		} else if spec.Config.Tasks[i].Glob != "" {
			if err := resolveFilePath(&spec.Config.Tasks[i].Glob, basePath); err != nil {
				return nil, fmt.Errorf("failed to resolve task glob at index %d: %w", i, err)
			}
		}
	}

	return spec, nil
}

func resolveFilePath(filePath *string, basePath string) error {
	if filePath == nil || *filePath == "" {
		return nil
	}

	// If the path is already absolute, leave it as-is
	if filepath.IsAbs(*filePath) {
		return nil
	}

	// Convert relative path to absolute path based on the YAML file's directory
	absPath := filepath.Join(basePath, *filePath)
	*filePath = absPath

	return nil
}

func FromFile(path string) (*TaskSetSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s' for tasksetspec: %w", path, err)
	}

	// Convert to absolute path to ensure basePath is absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for '%s': %w", path, err)
	}

	basePath := filepath.Dir(absPath)

	return Read(data, basePath)
}
