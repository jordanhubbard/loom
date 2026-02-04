package actions

import (
	"context"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/testing"
)

// TestRunnerAdapter adapts internal/testing.TestRunner to the actions.TestRunner interface
type TestRunnerAdapter struct {
	runner     *testing.TestRunner
	projectDir string
}

// NewTestRunnerAdapter creates a new adapter for the test runner
func NewTestRunnerAdapter(projectDir string) *TestRunnerAdapter {
	return &TestRunnerAdapter{
		runner:     testing.NewTestRunner(projectDir),
		projectDir: projectDir,
	}
}

// Run executes tests and returns structured results
func (a *TestRunnerAdapter) Run(ctx context.Context, projectPath string, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error) {
	// Use provided project path or fall back to adapter's project dir
	if projectPath == "" || projectPath == "." {
		projectPath = a.projectDir
	}

	// Build test request
	req := testing.TestRequest{
		ProjectPath: projectPath,
		TestPattern: testPattern,
		Framework:   framework,
		Timeout:     testing.DefaultTestTimeout,
	}

	// Apply custom timeout if specified
	if timeoutSeconds > 0 {
		req.Timeout = time.Duration(timeoutSeconds) * time.Second
	}

	// Execute tests
	result, err := a.runner.Run(ctx, req)
	if err != nil {
		return nil, err
	}

	// Convert TestResult to map for JSON serialization
	metadata := map[string]interface{}{
		"framework":  result.Framework,
		"success":    result.Success,
		"exit_code":  result.ExitCode,
		"timed_out":  result.TimedOut,
		"duration":   result.Duration.String(),
		"raw_output": result.RawOutput,
		"summary": map[string]interface{}{
			"total":   result.Summary.Total,
			"passed":  result.Summary.Passed,
			"failed":  result.Summary.Failed,
			"skipped": result.Summary.Skipped,
		},
	}

	// Add error if present
	if result.Error != "" {
		metadata["error"] = result.Error
	}

	// Add individual test cases if present
	if len(result.Tests) > 0 {
		tests := make([]map[string]interface{}, 0, len(result.Tests))
		for _, test := range result.Tests {
			testMap := map[string]interface{}{
				"name":     test.Name,
				"package":  test.Package,
				"status":   string(test.Status),
				"duration": test.Duration.String(),
			}
			if test.Output != "" {
				testMap["output"] = test.Output
			}
			if test.Error != "" {
				testMap["error"] = test.Error
			}
			if test.StackTrace != "" {
				testMap["stack_trace"] = test.StackTrace
			}
			tests = append(tests, testMap)
		}
		metadata["tests"] = tests
	}

	return metadata, nil
}
