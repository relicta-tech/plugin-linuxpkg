// Package main provides tests for the LinuxPkg plugin.
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// MockCommandExecutor is a mock implementation of CommandExecutor for testing.
type MockCommandExecutor struct {
	// RunFunc is called when Run is invoked. If nil, returns default success.
	RunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)
	// Calls records all calls made to Run.
	Calls []MockCall
}

// MockCall records a single call to the executor.
type MockCall struct {
	Name string
	Args []string
}

// Run implements CommandExecutor.
func (m *MockCommandExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	m.Calls = append(m.Calls, MockCall{Name: name, Args: args})
	if m.RunFunc != nil {
		return m.RunFunc(ctx, name, args...)
	}
	return []byte("created package: dist/myapp-1.0.0.deb"), nil
}

// TestGetInfo verifies plugin metadata.
func TestGetInfo(t *testing.T) {
	t.Parallel()

	p := &LinuxPkgPlugin{}
	info := p.GetInfo()

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "plugin name",
			got:      info.Name,
			expected: "linuxpkg",
		},
		{
			name:     "plugin version",
			got:      info.Version,
			expected: "2.0.0",
		},
		{
			name:     "plugin description",
			got:      info.Description,
			expected: "Build deb/rpm packages for Linux",
		},
		{
			name:     "plugin author",
			got:      info.Author,
			expected: "Relicta Team",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, tc.got)
			}
		})
	}

	// Verify hooks.
	t.Run("hooks contains PostPublish", func(t *testing.T) {
		t.Parallel()
		if len(info.Hooks) != 1 {
			t.Errorf("expected 1 hook, got %d", len(info.Hooks))
			return
		}
		if info.Hooks[0] != plugin.HookPostPublish {
			t.Errorf("expected hook %q, got %q", plugin.HookPostPublish, info.Hooks[0])
		}
	})

	// Verify config schema is valid JSON.
	t.Run("config schema is valid", func(t *testing.T) {
		t.Parallel()
		if info.ConfigSchema == "" {
			t.Error("config schema should not be empty")
		}
		// Verify it contains expected properties.
		if !strings.Contains(info.ConfigSchema, "config_path") {
			t.Error("config schema should contain config_path property")
		}
		if !strings.Contains(info.ConfigSchema, "formats") {
			t.Error("config schema should contain formats property")
		}
		if !strings.Contains(info.ConfigSchema, "output_dir") {
			t.Error("config schema should contain output_dir property")
		}
	})
}

// TestValidate tests configuration validation.
func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      map[string]any
		expectValid bool
		expectErrs  int
		errContains string
	}{
		{
			name:        "empty config is valid (uses defaults)",
			config:      map[string]any{},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name:        "nil config is valid (uses defaults)",
			config:      nil,
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "valid single format",
			config: map[string]any{
				"formats": []string{"deb"},
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "valid multiple formats",
			config: map[string]any{
				"formats": []string{"deb", "rpm", "apk"},
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "invalid format",
			config: map[string]any{
				"formats": []string{"exe"},
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "unsupported format",
		},
		{
			name: "invalid format with special characters",
			config: map[string]any{
				"formats": []string{"deb; rm -rf /"},
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "invalid format name",
		},
		{
			name: "path traversal in config_path",
			config: map[string]any{
				"config_path": "../../../etc/passwd",
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "path traversal",
		},
		{
			name: "absolute path in config_path",
			config: map[string]any{
				"config_path": "/etc/nfpm.yaml",
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "absolute paths are not allowed",
		},
		{
			name: "path traversal in output_dir",
			config: map[string]any{
				"output_dir": "../../tmp",
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "path traversal",
		},
		{
			name: "absolute path in output_dir",
			config: map[string]any{
				"output_dir": "/tmp/output",
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "absolute paths are not allowed",
		},
		{
			name: "invalid architecture",
			config: map[string]any{
				"target": "invalid-arch",
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "unsupported architecture",
		},
		{
			name: "valid architecture amd64",
			config: map[string]any{
				"target": "amd64",
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "valid architecture arm64",
			config: map[string]any{
				"target": "arm64",
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "valid current architecture",
			config: map[string]any{
				"target": "current",
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "invalid packager",
			config: map[string]any{
				"packager": "invalid",
			},
			expectValid: false,
			expectErrs:  1,
			errContains: "packager must be",
		},
		{
			name: "valid packager nfpm",
			config: map[string]any{
				"packager": "nfpm",
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "valid packager native",
			config: map[string]any{
				"packager": "native",
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "full valid configuration",
			config: map[string]any{
				"config_path": "nfpm.yaml",
				"formats":     []string{"deb", "rpm"},
				"output_dir":  "dist",
				"packager":    "nfpm",
				"target":      "amd64",
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "nested path is valid",
			config: map[string]any{
				"config_path": "configs/nfpm.yaml",
				"output_dir":  "build/packages",
			},
			expectValid: true,
			expectErrs:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &LinuxPkgPlugin{}
			resp, err := p.Validate(context.Background(), tc.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Valid != tc.expectValid {
				t.Errorf("expected valid=%v, got valid=%v, errors: %v", tc.expectValid, resp.Valid, resp.Errors)
			}

			if len(resp.Errors) != tc.expectErrs {
				t.Errorf("expected %d errors, got %d: %v", tc.expectErrs, len(resp.Errors), resp.Errors)
			}

			if tc.errContains != "" && len(resp.Errors) > 0 {
				found := false
				for _, e := range resp.Errors {
					if strings.Contains(e.Message, tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, resp.Errors)
				}
			}
		})
	}
}

// TestParseConfig tests configuration parsing.
func TestParseConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         map[string]any
		expectedConfig *Config
	}{
		{
			name:   "empty config uses defaults",
			config: map[string]any{},
			expectedConfig: &Config{
				ConfigPath: "nfpm.yaml",
				Formats:    []string{"deb", "rpm"},
				OutputDir:  "dist",
				Packager:   "nfpm",
				Target:     "current",
			},
		},
		{
			name: "custom config path",
			config: map[string]any{
				"config_path": "custom-nfpm.yaml",
			},
			expectedConfig: &Config{
				ConfigPath: "custom-nfpm.yaml",
				Formats:    []string{"deb", "rpm"},
				OutputDir:  "dist",
				Packager:   "nfpm",
				Target:     "current",
			},
		},
		{
			name: "single format",
			config: map[string]any{
				"formats": []string{"deb"},
			},
			expectedConfig: &Config{
				ConfigPath: "nfpm.yaml",
				Formats:    []string{"deb"},
				OutputDir:  "dist",
				Packager:   "nfpm",
				Target:     "current",
			},
		},
		{
			name: "all formats",
			config: map[string]any{
				"formats": []string{"deb", "rpm", "apk"},
			},
			expectedConfig: &Config{
				ConfigPath: "nfpm.yaml",
				Formats:    []string{"deb", "rpm", "apk"},
				OutputDir:  "dist",
				Packager:   "nfpm",
				Target:     "current",
			},
		},
		{
			name: "custom output directory",
			config: map[string]any{
				"output_dir": "build/packages",
			},
			expectedConfig: &Config{
				ConfigPath: "nfpm.yaml",
				Formats:    []string{"deb", "rpm"},
				OutputDir:  "build/packages",
				Packager:   "nfpm",
				Target:     "current",
			},
		},
		{
			name: "native packager",
			config: map[string]any{
				"packager": "native",
			},
			expectedConfig: &Config{
				ConfigPath: "nfpm.yaml",
				Formats:    []string{"deb", "rpm"},
				OutputDir:  "dist",
				Packager:   "native",
				Target:     "current",
			},
		},
		{
			name: "specific target architecture",
			config: map[string]any{
				"target": "arm64",
			},
			expectedConfig: &Config{
				ConfigPath: "nfpm.yaml",
				Formats:    []string{"deb", "rpm"},
				OutputDir:  "dist",
				Packager:   "nfpm",
				Target:     "arm64",
			},
		},
		{
			name: "full configuration",
			config: map[string]any{
				"config_path": "configs/nfpm.yaml",
				"formats":     []string{"deb", "rpm", "apk"},
				"output_dir":  "dist/linux",
				"packager":    "nfpm",
				"target":      "amd64",
			},
			expectedConfig: &Config{
				ConfigPath: "configs/nfpm.yaml",
				Formats:    []string{"deb", "rpm", "apk"},
				OutputDir:  "dist/linux",
				Packager:   "nfpm",
				Target:     "amd64",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &LinuxPkgPlugin{}
			cfg := p.parseConfig(tc.config)

			if cfg.ConfigPath != tc.expectedConfig.ConfigPath {
				t.Errorf("ConfigPath: expected %q, got %q", tc.expectedConfig.ConfigPath, cfg.ConfigPath)
			}
			if cfg.OutputDir != tc.expectedConfig.OutputDir {
				t.Errorf("OutputDir: expected %q, got %q", tc.expectedConfig.OutputDir, cfg.OutputDir)
			}
			if cfg.Packager != tc.expectedConfig.Packager {
				t.Errorf("Packager: expected %q, got %q", tc.expectedConfig.Packager, cfg.Packager)
			}
			if cfg.Target != tc.expectedConfig.Target {
				t.Errorf("Target: expected %q, got %q", tc.expectedConfig.Target, cfg.Target)
			}
			if len(cfg.Formats) != len(tc.expectedConfig.Formats) {
				t.Errorf("Formats length: expected %d, got %d", len(tc.expectedConfig.Formats), len(cfg.Formats))
			} else {
				for i, f := range cfg.Formats {
					if f != tc.expectedConfig.Formats[i] {
						t.Errorf("Formats[%d]: expected %q, got %q", i, tc.expectedConfig.Formats[i], f)
					}
				}
			}
		})
	}
}

// TestExecuteDryRun tests dry run execution.
func TestExecuteDryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         map[string]any
		expectSuccess  bool
		expectContains string
		expectOutputs  map[string]any
	}{
		{
			name: "dry run with single format",
			config: map[string]any{
				"formats": []string{"deb"},
			},
			expectSuccess:  true,
			expectContains: "Would build 1 package(s)",
			expectOutputs: map[string]any{
				"formats": []string{"deb"},
			},
		},
		{
			name: "dry run with multiple formats",
			config: map[string]any{
				"formats": []string{"deb", "rpm", "apk"},
			},
			expectSuccess:  true,
			expectContains: "Would build 3 package(s)",
			expectOutputs: map[string]any{
				"formats": []string{"deb", "rpm", "apk"},
			},
		},
		{
			name:           "dry run with default config",
			config:         map[string]any{},
			expectSuccess:  true,
			expectContains: "Would build 2 package(s)",
			expectOutputs: map[string]any{
				"config_path": "nfpm.yaml",
				"output_dir":  "dist",
				"packager":    "nfpm",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &LinuxPkgPlugin{}
			req := plugin.ExecuteRequest{
				Hook:   plugin.HookPostPublish,
				DryRun: true,
				Config: tc.config,
				Context: plugin.ReleaseContext{
					Version:         "1.0.0",
					TagName:         "v1.0.0",
					ReleaseType:     "minor",
					RepositoryURL:   "https://github.com/example/repo",
					RepositoryOwner: "example",
					RepositoryName:  "repo",
					Branch:          "main",
					CommitSHA:       "abc123",
				},
			}

			resp, err := p.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Success != tc.expectSuccess {
				t.Errorf("expected success=%v, got success=%v, error: %s", tc.expectSuccess, resp.Success, resp.Error)
			}

			if tc.expectContains != "" && !strings.Contains(resp.Message, tc.expectContains) {
				t.Errorf("expected message to contain %q, got %q", tc.expectContains, resp.Message)
			}

			// Verify outputs.
			if resp.Outputs != nil {
				for key, expected := range tc.expectOutputs {
					got, ok := resp.Outputs[key]
					if !ok {
						t.Errorf("expected output key %q to exist", key)
						continue
					}
					// For slices, compare manually.
					switch exp := expected.(type) {
					case []string:
						gotSlice, ok := got.([]string)
						if !ok {
							t.Errorf("output %q: expected []string, got %T", key, got)
							continue
						}
						if len(gotSlice) != len(exp) {
							t.Errorf("output %q: expected length %d, got %d", key, len(exp), len(gotSlice))
						}
					case string:
						if got != exp {
							t.Errorf("output %q: expected %q, got %q", key, exp, got)
						}
					}
				}
			}
		})
	}
}

// TestExecuteWithMockExecutor tests actual execution with mock.
// Note: These tests cannot run in parallel due to chdir usage.
func TestExecuteWithMockExecutor(t *testing.T) {
	tests := []struct {
		name          string
		configPath    string
		formats       []string
		outputDir     string
		mockFunc      func(ctx context.Context, name string, args ...string) ([]byte, error)
		expectSuccess bool
		expectMessage string
		expectError   string
		verifyCall    func(t *testing.T, calls []MockCall)
	}{
		{
			name:       "successful single format build",
			configPath: "nfpm.yaml",
			formats:    []string{"deb"},
			outputDir:  "dist",
			mockFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return []byte("created package: dist/myapp-1.0.0.deb"), nil
			},
			expectSuccess: true,
			expectMessage: "Built 1 Linux package(s)",
			verifyCall: func(t *testing.T, calls []MockCall) {
				t.Helper()
				if len(calls) != 1 {
					t.Errorf("expected 1 call, got %d", len(calls))
					return
				}
				if calls[0].Name != "nfpm" {
					t.Errorf("expected command 'nfpm', got %q", calls[0].Name)
				}
				// Verify args contain expected flags.
				argsStr := strings.Join(calls[0].Args, " ")
				if !strings.Contains(argsStr, "--packager deb") {
					t.Errorf("expected --packager deb in args: %v", calls[0].Args)
				}
			},
		},
		{
			name:       "successful multiple format build",
			configPath: "nfpm.yaml",
			formats:    []string{"deb", "rpm"},
			outputDir:  "dist2",
			mockFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return []byte("created package: package.deb"), nil
			},
			expectSuccess: true,
			expectMessage: "Built 2 Linux package(s)",
			verifyCall: func(t *testing.T, calls []MockCall) {
				t.Helper()
				if len(calls) != 2 {
					t.Errorf("expected 2 calls, got %d", len(calls))
				}
			},
		},
		{
			name:       "nfpm command failure",
			configPath: "nfpm.yaml",
			formats:    []string{"deb"},
			outputDir:  "dist3",
			mockFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return []byte("error: invalid config"), errors.New("exit status 1")
			},
			expectSuccess: false,
			expectError:   "failed to build deb package",
		},
		{
			name:       "build with all formats",
			configPath: "nfpm.yaml",
			formats:    []string{"deb", "rpm", "apk"},
			outputDir:  "dist4",
			mockFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return []byte("created package: package.pkg"), nil
			},
			expectSuccess: true,
			expectMessage: "Built 3 Linux package(s)",
			verifyCall: func(t *testing.T, calls []MockCall) {
				t.Helper()
				if len(calls) != 3 {
					t.Errorf("expected 3 calls, got %d", len(calls))
				}
				// Verify each format was called.
				formats := make(map[string]bool)
				for _, call := range calls {
					for i, arg := range call.Args {
						if arg == "--packager" && i+1 < len(call.Args) {
							formats[call.Args[i+1]] = true
						}
					}
				}
				for _, f := range []string{"deb", "rpm", "apk"} {
					if !formats[f] {
						t.Errorf("expected format %q to be called", f)
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary directory and change to it.
			tmpDir := t.TempDir()
			oldWd, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get working directory: %v", err)
			}
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("failed to change to temp directory: %v", err)
			}
			t.Cleanup(func() {
				_ = os.Chdir(oldWd)
			})

			// Create the config file.
			if err := os.WriteFile(tc.configPath, []byte("name: test\nversion: 1.0.0"), 0644); err != nil {
				t.Fatalf("failed to create test config: %v", err)
			}

			mock := &MockCommandExecutor{RunFunc: tc.mockFunc}
			p := &LinuxPkgPlugin{cmdExecutor: mock}

			req := plugin.ExecuteRequest{
				Hook:   plugin.HookPostPublish,
				DryRun: false,
				Config: map[string]any{
					"config_path": tc.configPath,
					"formats":     tc.formats,
					"output_dir":  tc.outputDir,
				},
				Context: plugin.ReleaseContext{
					Version:         "1.0.0",
					TagName:         "v1.0.0",
					ReleaseType:     "minor",
					RepositoryURL:   "https://github.com/example/repo",
					RepositoryOwner: "example",
					RepositoryName:  "repo",
					Branch:          "main",
					CommitSHA:       "abc123",
				},
			}

			resp, err := p.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Success != tc.expectSuccess {
				t.Errorf("expected success=%v, got success=%v, error: %s", tc.expectSuccess, resp.Success, resp.Error)
			}

			if tc.expectMessage != "" && !strings.Contains(resp.Message, tc.expectMessage) {
				t.Errorf("expected message to contain %q, got %q", tc.expectMessage, resp.Message)
			}

			if tc.expectError != "" && !strings.Contains(resp.Error, tc.expectError) {
				t.Errorf("expected error to contain %q, got %q", tc.expectError, resp.Error)
			}

			if tc.verifyCall != nil {
				tc.verifyCall(t, mock.Calls)
			}
		})
	}
}

// TestExecuteValidationErrors tests execution with invalid configurations.
func TestExecuteValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      map[string]any
		expectError string
	}{
		{
			name: "path traversal in config_path",
			config: map[string]any{
				"config_path": "../../../etc/passwd",
			},
			expectError: "invalid config_path",
		},
		{
			name: "path traversal in output_dir",
			config: map[string]any{
				"output_dir": "../../tmp",
			},
			expectError: "invalid output_dir",
		},
		{
			name: "invalid format",
			config: map[string]any{
				"formats": []string{"exe"},
			},
			expectError: "invalid format",
		},
		{
			name: "invalid architecture",
			config: map[string]any{
				"target": "x86_64", // Should be amd64.
			},
			expectError: "invalid target",
		},
		{
			name: "absolute config path",
			config: map[string]any{
				"config_path": "/etc/nfpm.yaml",
			},
			expectError: "invalid config_path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &LinuxPkgPlugin{}
			req := plugin.ExecuteRequest{
				Hook:   plugin.HookPostPublish,
				DryRun: false, // Not dry run to trigger validation.
				Config: tc.config,
				Context: plugin.ReleaseContext{
					Version: "1.0.0",
					TagName: "v1.0.0",
				},
			}

			resp, err := p.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.Success {
				t.Error("expected execution to fail")
			}

			if !strings.Contains(resp.Error, tc.expectError) {
				t.Errorf("expected error to contain %q, got %q", tc.expectError, resp.Error)
			}
		})
	}
}

// TestExecuteConfigFileNotFound tests execution when config file doesn't exist.
func TestExecuteConfigFileNotFound(t *testing.T) {
	t.Parallel()

	p := &LinuxPkgPlugin{}
	req := plugin.ExecuteRequest{
		Hook:   plugin.HookPostPublish,
		DryRun: false,
		Config: map[string]any{
			"config_path": "nonexistent-config.yaml",
			"formats":     []string{"deb"},
		},
		Context: plugin.ReleaseContext{
			Version: "1.0.0",
			TagName: "v1.0.0",
		},
	}

	resp, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Success {
		t.Error("expected execution to fail for missing config")
	}

	if !strings.Contains(resp.Error, "config file does not exist") {
		t.Errorf("expected error about missing config, got: %s", resp.Error)
	}
}

// TestExecuteUnhandledHook tests unhandled hooks.
func TestExecuteUnhandledHook(t *testing.T) {
	t.Parallel()

	unhandledHooks := []plugin.Hook{
		plugin.HookPreInit,
		plugin.HookPostInit,
		plugin.HookPrePlan,
		plugin.HookPostPlan,
		plugin.HookPreVersion,
		plugin.HookPostVersion,
		plugin.HookPreNotes,
		plugin.HookPostNotes,
		plugin.HookPreApprove,
		plugin.HookPostApprove,
		plugin.HookPrePublish,
		plugin.HookOnSuccess,
		plugin.HookOnError,
	}

	for _, hook := range unhandledHooks {
		t.Run(string(hook), func(t *testing.T) {
			t.Parallel()

			p := &LinuxPkgPlugin{}
			req := plugin.ExecuteRequest{
				Hook:   hook,
				DryRun: false,
				Config: map[string]any{},
				Context: plugin.ReleaseContext{
					Version: "1.0.0",
					TagName: "v1.0.0",
				},
			}

			resp, err := p.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error for hook %s: %v", hook, err)
			}

			if !resp.Success {
				t.Errorf("expected success for unhandled hook %s, got failure", hook)
			}

			expectedMsg := "Hook " + string(hook) + " not handled"
			if resp.Message != expectedMsg {
				t.Errorf("expected message %q, got %q", expectedMsg, resp.Message)
			}
		})
	}
}

// TestValidatePathFunction tests the validatePath helper function.
func TestValidatePathFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{
			name:      "empty path",
			path:      "",
			expectErr: false,
		},
		{
			name:      "simple filename",
			path:      "nfpm.yaml",
			expectErr: false,
		},
		{
			name:      "nested path",
			path:      "configs/nfpm.yaml",
			expectErr: false,
		},
		{
			name:      "deeply nested path",
			path:      "configs/linux/nfpm.yaml",
			expectErr: false,
		},
		{
			name:      "path traversal at start",
			path:      "../secret.yaml",
			expectErr: true,
		},
		{
			name:      "path traversal in middle",
			path:      "configs/../../../etc/passwd",
			expectErr: true,
		},
		{
			name:      "absolute path unix",
			path:      "/etc/nfpm.yaml",
			expectErr: true,
		},
		{
			name:      "current directory",
			path:      ".",
			expectErr: false,
		},
		{
			name:      "relative current",
			path:      "./nfpm.yaml",
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validatePath(tc.path)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for path %q, got nil", tc.path)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for path %q: %v", tc.path, err)
			}
		})
	}
}

// TestValidateFormatFunction tests the validateFormat helper function.
func TestValidateFormatFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		format    string
		expectErr bool
	}{
		{
			name:      "valid deb",
			format:    "deb",
			expectErr: false,
		},
		{
			name:      "valid rpm",
			format:    "rpm",
			expectErr: false,
		},
		{
			name:      "valid apk",
			format:    "apk",
			expectErr: false,
		},
		{
			name:      "empty format",
			format:    "",
			expectErr: true,
		},
		{
			name:      "unsupported format",
			format:    "exe",
			expectErr: true,
		},
		{
			name:      "uppercase format",
			format:    "DEB",
			expectErr: true,
		},
		{
			name:      "format with special chars",
			format:    "deb; rm -rf /",
			expectErr: true,
		},
		{
			name:      "format with spaces",
			format:    "deb rpm",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateFormat(tc.format)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for format %q, got nil", tc.format)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for format %q: %v", tc.format, err)
			}
		})
	}
}

// TestValidateArchitectureFunction tests the validateArchitecture helper function.
func TestValidateArchitectureFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arch      string
		expectErr bool
	}{
		{
			name:      "empty uses current",
			arch:      "",
			expectErr: false,
		},
		{
			name:      "current keyword",
			arch:      "current",
			expectErr: false,
		},
		{
			name:      "valid amd64",
			arch:      "amd64",
			expectErr: false,
		},
		{
			name:      "valid arm64",
			arch:      "arm64",
			expectErr: false,
		},
		{
			name:      "valid 386",
			arch:      "386",
			expectErr: false,
		},
		{
			name:      "valid arm",
			arch:      "arm",
			expectErr: false,
		},
		{
			name:      "invalid x86_64",
			arch:      "x86_64",
			expectErr: true,
		},
		{
			name:      "invalid aarch64",
			arch:      "aarch64",
			expectErr: true,
		},
		{
			name:      "invalid arbitrary",
			arch:      "invalid",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateArchitecture(tc.arch)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for arch %q, got nil", tc.arch)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for arch %q: %v", tc.arch, err)
			}
		})
	}
}

// TestParsePackagePath tests the parsePackagePath helper function.
func TestParsePackagePath(t *testing.T) {
	t.Parallel()

	p := &LinuxPkgPlugin{}

	tests := []struct {
		name       string
		output     string
		outputDir  string
		format     string
		expectPath string
	}{
		{
			name:       "standard nfpm output",
			output:     "created package: dist/myapp-1.0.0.deb",
			outputDir:  "dist",
			format:     "deb",
			expectPath: "dist/myapp-1.0.0.deb",
		},
		{
			name:       "multiline output",
			output:     "building package...\ncreated package: dist/myapp-1.0.0.rpm\ndone",
			outputDir:  "dist",
			format:     "rpm",
			expectPath: "dist/myapp-1.0.0.rpm",
		},
		{
			name:       "no match returns empty",
			output:     "some other output",
			outputDir:  "dist",
			format:     "deb",
			expectPath: "",
		},
		{
			name:       "empty output",
			output:     "",
			outputDir:  "dist",
			format:     "deb",
			expectPath: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := p.parsePackagePath([]byte(tc.output), tc.outputDir, tc.format)
			if result != tc.expectPath {
				t.Errorf("expected %q, got %q", tc.expectPath, result)
			}
		})
	}
}

// TestGetExecutor tests the getExecutor method.
func TestGetExecutor(t *testing.T) {
	t.Parallel()

	t.Run("returns real executor when none set", func(t *testing.T) {
		t.Parallel()
		p := &LinuxPkgPlugin{}
		executor := p.getExecutor()
		if executor == nil {
			t.Error("expected non-nil executor")
		}
		if _, ok := executor.(*RealCommandExecutor); !ok {
			t.Errorf("expected RealCommandExecutor, got %T", executor)
		}
	})

	t.Run("returns mock executor when set", func(t *testing.T) {
		t.Parallel()
		mock := &MockCommandExecutor{}
		p := &LinuxPkgPlugin{cmdExecutor: mock}
		executor := p.getExecutor()
		if executor != mock {
			t.Error("expected mock executor to be returned")
		}
	})
}

// TestDryRunResolvesCurrentArchitecture tests that dry run correctly resolves current architecture.
func TestDryRunResolvesCurrentArchitecture(t *testing.T) {
	t.Parallel()

	p := &LinuxPkgPlugin{}
	req := plugin.ExecuteRequest{
		Hook:   plugin.HookPostPublish,
		DryRun: true,
		Config: map[string]any{
			"target": "current",
		},
		Context: plugin.ReleaseContext{
			Version: "1.0.0",
			TagName: "v1.0.0",
		},
	}

	resp, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success, got failure: %s", resp.Error)
	}

	target, ok := resp.Outputs["target"].(string)
	if !ok {
		t.Fatal("expected target output to be string")
	}

	if target != runtime.GOARCH {
		t.Errorf("expected target to be %q (current arch), got %q", runtime.GOARCH, target)
	}
}

// TestExecuteCreatesOutputDirectory tests that the plugin creates the output directory.
// Note: This test cannot run in parallel due to chdir usage.
func TestExecuteCreatesOutputDirectory(t *testing.T) {
	// Create a temporary directory and change to it.
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	configPath := "nfpm.yaml"
	if err := os.WriteFile(configPath, []byte("name: test\nversion: 1.0.0"), 0644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	outputDir := filepath.Join("nested", "output", "dir")

	mock := &MockCommandExecutor{
		RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return []byte("created package: test.deb"), nil
		},
	}
	p := &LinuxPkgPlugin{cmdExecutor: mock}

	req := plugin.ExecuteRequest{
		Hook:   plugin.HookPostPublish,
		DryRun: false,
		Config: map[string]any{
			"config_path": configPath,
			"formats":     []string{"deb"},
			"output_dir":  outputDir,
		},
		Context: plugin.ReleaseContext{
			Version: "1.0.0",
			TagName: "v1.0.0",
		},
	}

	resp, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success, got failure: %s", resp.Error)
	}

	// Verify the output directory was created.
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Error("expected output directory to be created")
	}
}

// TestValidateConfigExists tests the validateConfigExists helper function.
func TestValidateConfigExists(t *testing.T) {
	t.Parallel()

	t.Run("file exists", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "nfpm.yaml")
		if err := os.WriteFile(configPath, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		err := validateConfigExists(configPath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		t.Parallel()

		err := validateConfigExists("/nonexistent/path/nfpm.yaml")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("expected 'does not exist' in error, got: %v", err)
		}
	})

	t.Run("path is a directory", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()
		err := validateConfigExists(tmpDir)
		if err == nil {
			t.Error("expected error for directory path")
		}
		if !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("expected 'is a directory' in error, got: %v", err)
		}
	})
}

// TestCommandArgsFormat tests that the nfpm command is built correctly.
// Note: This test cannot run in parallel due to chdir usage.
func TestCommandArgsFormat(t *testing.T) {
	// Create a temporary directory and change to it.
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWd)
	})

	configPath := "nfpm.yaml"
	if err := os.WriteFile(configPath, []byte("name: test\nversion: 1.0.0"), 0644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}
	outputDir := "dist"

	var capturedArgs []string
	mock := &MockCommandExecutor{
		RunFunc: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("created package: test.deb"), nil
		},
	}
	p := &LinuxPkgPlugin{cmdExecutor: mock}

	req := plugin.ExecuteRequest{
		Hook:   plugin.HookPostPublish,
		DryRun: false,
		Config: map[string]any{
			"config_path": configPath,
			"formats":     []string{"deb"},
			"output_dir":  outputDir,
		},
		Context: plugin.ReleaseContext{
			Version: "1.0.0",
			TagName: "v1.0.0",
		},
	}

	resp, err := p.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success, got failure: %s", resp.Error)
	}

	// Verify the args structure.
	expectedArgs := []string{
		"package",
		"--config", configPath,
		"--packager", "deb",
		"--target", outputDir + "/",
	}

	if len(capturedArgs) != len(expectedArgs) {
		t.Errorf("expected %d args, got %d: %v", len(expectedArgs), len(capturedArgs), capturedArgs)
	}

	for i, expected := range expectedArgs {
		if i < len(capturedArgs) && capturedArgs[i] != expected {
			t.Errorf("arg[%d]: expected %q, got %q", i, expected, capturedArgs[i])
		}
	}
}
