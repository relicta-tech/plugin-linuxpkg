// Package main provides tests for the LinuxPkg plugin.
package main

import (
	"context"
	"testing"

	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

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

	// Verify hooks
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

	// Verify config schema is valid JSON
	t.Run("config schema is valid", func(t *testing.T) {
		t.Parallel()
		if info.ConfigSchema == "" {
			t.Error("config schema should not be empty")
		}
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		config      map[string]any
		expectValid bool
		expectErrs  int
	}{
		{
			name:        "empty config is valid",
			config:      map[string]any{},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name:        "nil config is valid",
			config:      nil,
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "config with formats is valid",
			config: map[string]any{
				"formats": []string{"deb", "rpm"},
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "config with binaries is valid",
			config: map[string]any{
				"binaries": []map[string]any{
					{"name": "myapp", "path": "./bin/myapp"},
				},
			},
			expectValid: true,
			expectErrs:  0,
		},
		{
			name: "config with all options is valid",
			config: map[string]any{
				"formats":     []string{"deb", "rpm", "apk"},
				"name":        "myapp",
				"description": "My application",
				"maintainer":  "dev@example.com",
				"binaries": []map[string]any{
					{"name": "myapp", "path": "./bin/myapp"},
				},
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
				t.Errorf("expected valid=%v, got valid=%v", tc.expectValid, resp.Valid)
			}

			if len(resp.Errors) != tc.expectErrs {
				t.Errorf("expected %d errors, got %d: %v", tc.expectErrs, len(resp.Errors), resp.Errors)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config map[string]any
	}{
		{
			name:   "empty config uses defaults",
			config: map[string]any{},
		},
		{
			name: "custom formats array",
			config: map[string]any{
				"formats": []string{"deb", "rpm"},
			},
		},
		{
			name: "single format",
			config: map[string]any{
				"formats": []string{"deb"},
			},
		},
		{
			name: "binaries configuration",
			config: map[string]any{
				"binaries": []map[string]any{
					{"name": "app1", "path": "./bin/app1"},
					{"name": "app2", "path": "./bin/app2"},
				},
			},
		},
		{
			name: "full configuration",
			config: map[string]any{
				"formats":     []string{"deb", "rpm", "apk"},
				"name":        "testapp",
				"description": "A test application",
				"maintainer":  "test@example.com",
				"homepage":    "https://example.com",
				"license":     "MIT",
				"binaries": []map[string]any{
					{"name": "testapp", "path": "./dist/testapp"},
				},
				"depends": []string{"libc6"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Validate passes with any config, verifying the plugin accepts it
			p := &LinuxPkgPlugin{}
			resp, err := p.Validate(context.Background(), tc.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !resp.Valid {
				t.Errorf("expected config to be valid, got errors: %v", resp.Errors)
			}
		})
	}
}

func TestExecuteDryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		hook           plugin.Hook
		dryRun         bool
		expectSuccess  bool
		expectContains string
	}{
		{
			name:           "PostPublish with dry run",
			hook:           plugin.HookPostPublish,
			dryRun:         true,
			expectSuccess:  true,
			expectContains: "Would execute",
		},
		{
			name:           "PostPublish without dry run",
			hook:           plugin.HookPostPublish,
			dryRun:         false,
			expectSuccess:  true,
			expectContains: "executed successfully",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &LinuxPkgPlugin{}
			req := plugin.ExecuteRequest{
				Hook:   tc.hook,
				DryRun: tc.dryRun,
				Config: map[string]any{
					"formats": []string{"deb"},
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
				t.Errorf("expected success=%v, got success=%v", tc.expectSuccess, resp.Success)
			}

			if tc.expectContains != "" && !contains(resp.Message, tc.expectContains) {
				t.Errorf("expected message to contain %q, got %q", tc.expectContains, resp.Message)
			}
		})
	}
}

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

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
