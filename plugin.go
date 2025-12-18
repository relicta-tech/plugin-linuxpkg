// Package main implements the LinuxPkg plugin for Relicta.
// It builds Linux packages (deb, rpm, apk) using nfpm.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// Allowed package formats for security validation.
var allowedFormats = map[string]bool{
	"deb": true,
	"rpm": true,
	"apk": true,
}

// Allowed target architectures for security validation.
var allowedArchitectures = map[string]bool{
	"amd64":   true,
	"386":     true,
	"arm64":   true,
	"arm":     true,
	"ppc64le": true,
	"s390x":   true,
	"riscv64": true,
}

// formatNamePattern validates package format names.
var formatNamePattern = regexp.MustCompile(`^[a-z]+$`)

// CommandExecutor abstracts command execution for testability.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// RealCommandExecutor executes real shell commands.
type RealCommandExecutor struct{}

// Run executes a command and returns combined output.
func (e *RealCommandExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// LinuxPkgPlugin implements the Linux package building plugin.
type LinuxPkgPlugin struct {
	// cmdExecutor is used for executing shell commands. If nil, uses RealCommandExecutor.
	cmdExecutor CommandExecutor
}

// getExecutor returns the command executor, defaulting to RealCommandExecutor.
func (p *LinuxPkgPlugin) getExecutor() CommandExecutor {
	if p.cmdExecutor != nil {
		return p.cmdExecutor
	}
	return &RealCommandExecutor{}
}

// Config represents the LinuxPkg plugin configuration.
type Config struct {
	// ConfigPath is the path to the nfpm.yaml configuration file.
	ConfigPath string
	// Formats is the list of package formats to build (deb, rpm, apk).
	Formats []string
	// OutputDir is the directory where packages will be written.
	OutputDir string
	// Packager is the tool to use for packaging (nfpm or native).
	Packager string
	// Target is the target architecture for the packages.
	Target string
}

// GetInfo returns plugin metadata.
func (p *LinuxPkgPlugin) GetInfo() plugin.Info {
	return plugin.Info{
		Name:        "linuxpkg",
		Version:     "2.0.0",
		Description: "Build deb/rpm packages for Linux",
		Author:      "Relicta Team",
		Hooks: []plugin.Hook{
			plugin.HookPostPublish,
		},
		ConfigSchema: `{
			"type": "object",
			"properties": {
				"config_path": {
					"type": "string",
					"description": "Path to nfpm.yaml config file",
					"default": "nfpm.yaml"
				},
				"formats": {
					"type": "array",
					"items": {"type": "string", "enum": ["deb", "rpm", "apk"]},
					"description": "Package formats to build",
					"default": ["deb", "rpm"]
				},
				"output_dir": {
					"type": "string",
					"description": "Output directory for packages",
					"default": "dist"
				},
				"packager": {
					"type": "string",
					"enum": ["nfpm", "native"],
					"description": "Tool to use for packaging",
					"default": "nfpm"
				},
				"target": {
					"type": "string",
					"description": "Target architecture",
					"default": "current"
				}
			}
		}`,
	}
}

// validatePath validates a file path to prevent path traversal attacks.
func validatePath(path string) error {
	if path == "" {
		return nil
	}

	// Clean the path to normalize it.
	cleaned := filepath.Clean(path)

	// Check for absolute paths (potential escape from working directory).
	if filepath.IsAbs(cleaned) {
		return fmt.Errorf("absolute paths are not allowed: %s", path)
	}

	// Check for path traversal attempts.
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+"..") {
		return fmt.Errorf("path traversal detected: cannot use '..' to escape working directory")
	}

	return nil
}

// validateFormat validates that a package format is allowed.
func validateFormat(format string) error {
	if format == "" {
		return fmt.Errorf("format cannot be empty")
	}

	if !formatNamePattern.MatchString(format) {
		return fmt.Errorf("invalid format name: must contain only lowercase letters")
	}

	if !allowedFormats[format] {
		return fmt.Errorf("unsupported format: %s (allowed: deb, rpm, apk)", format)
	}

	return nil
}

// validateArchitecture validates that a target architecture is allowed.
func validateArchitecture(arch string) error {
	if arch == "" || arch == "current" {
		return nil
	}

	if !allowedArchitectures[arch] {
		allowed := make([]string, 0, len(allowedArchitectures))
		for k := range allowedArchitectures {
			allowed = append(allowed, k)
		}
		return fmt.Errorf("unsupported architecture: %s (allowed: %s)", arch, strings.Join(allowed, ", "))
	}

	return nil
}

// validateConfigExists checks if the config file exists.
func validateConfigExists(configPath string) error {
	info, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("config path is a directory, not a file: %s", configPath)
	}
	return nil
}

// Execute runs the plugin for a given hook.
func (p *LinuxPkgPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	cfg := p.parseConfig(req.Config)

	switch req.Hook {
	case plugin.HookPostPublish:
		return p.buildPackages(ctx, cfg, req.Context, req.DryRun)
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// buildPackages builds Linux packages using nfpm.
func (p *LinuxPkgPlugin) buildPackages(ctx context.Context, cfg *Config, releaseCtx plugin.ReleaseContext, dryRun bool) (*plugin.ExecuteResponse, error) {
	// Validate configuration paths.
	if err := validatePath(cfg.ConfigPath); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid config_path: %v", err),
		}, nil
	}

	if err := validatePath(cfg.OutputDir); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid output_dir: %v", err),
		}, nil
	}

	// Validate formats.
	for _, format := range cfg.Formats {
		if err := validateFormat(format); err != nil {
			return &plugin.ExecuteResponse{
				Success: false,
				Error:   fmt.Sprintf("invalid format: %v", err),
			}, nil
		}
	}

	// Validate target architecture.
	if err := validateArchitecture(cfg.Target); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid target: %v", err),
		}, nil
	}

	// Resolve target architecture.
	targetArch := cfg.Target
	if targetArch == "" || targetArch == "current" {
		targetArch = runtime.GOARCH
	}

	// Handle dry run.
	if dryRun {
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Would build %d package(s) using %s", len(cfg.Formats), cfg.Packager),
			Outputs: map[string]any{
				"config_path": cfg.ConfigPath,
				"formats":     cfg.Formats,
				"output_dir":  cfg.OutputDir,
				"packager":    cfg.Packager,
				"target":      targetArch,
				"version":     releaseCtx.Version,
			},
		}, nil
	}

	// Validate config file exists (only for actual execution).
	if err := validateConfigExists(cfg.ConfigPath); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	// Create output directory if it doesn't exist.
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return &plugin.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create output directory: %v", err),
		}, nil
	}

	// Build packages for each format.
	builtPackages := make([]string, 0, len(cfg.Formats))
	executor := p.getExecutor()

	for _, format := range cfg.Formats {
		output, err := p.buildPackage(ctx, executor, cfg, format, targetArch)
		if err != nil {
			return &plugin.ExecuteResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to build %s package: %v\nOutput: %s", format, err, string(output)),
			}, nil
		}

		// Parse the output to get the package filename.
		packagePath := p.parsePackagePath(output, cfg.OutputDir, format)
		if packagePath != "" {
			builtPackages = append(builtPackages, packagePath)
		} else {
			// Fallback: construct expected package name.
			builtPackages = append(builtPackages, filepath.Join(cfg.OutputDir, fmt.Sprintf("package.%s", format)))
		}
	}

	return &plugin.ExecuteResponse{
		Success: true,
		Message: fmt.Sprintf("Built %d Linux package(s)", len(builtPackages)),
		Outputs: map[string]any{
			"packages":   builtPackages,
			"formats":    cfg.Formats,
			"output_dir": cfg.OutputDir,
			"target":     targetArch,
			"version":    releaseCtx.Version,
		},
	}, nil
}

// buildPackage builds a single package using nfpm.
func (p *LinuxPkgPlugin) buildPackage(ctx context.Context, executor CommandExecutor, cfg *Config, format, targetArch string) ([]byte, error) {
	args := []string{
		"package",
		"--config", cfg.ConfigPath,
		"--packager", format,
		"--target", cfg.OutputDir + "/",
	}

	return executor.Run(ctx, "nfpm", args...)
}

// parsePackagePath attempts to parse the package path from nfpm output.
func (p *LinuxPkgPlugin) parsePackagePath(output []byte, outputDir, format string) string {
	// nfpm typically outputs: "created package: <path>"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "created package:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
		// Also check for "using" pattern from some nfpm versions.
		if strings.Contains(line, "."+format) && strings.Contains(line, outputDir) {
			return line
		}
	}
	return ""
}

// parseConfig parses the raw configuration into a Config struct.
func (p *LinuxPkgPlugin) parseConfig(raw map[string]any) *Config {
	parser := helpers.NewConfigParser(raw)

	// Parse formats with default.
	formats := parser.GetStringSlice("formats", []string{"deb", "rpm"})
	if len(formats) == 0 {
		formats = []string{"deb", "rpm"}
	}

	return &Config{
		ConfigPath: parser.GetString("config_path", "", "nfpm.yaml"),
		Formats:    formats,
		OutputDir:  parser.GetString("output_dir", "", "dist"),
		Packager:   parser.GetString("packager", "", "nfpm"),
		Target:     parser.GetString("target", "", "current"),
	}
}

// Validate validates the plugin configuration.
func (p *LinuxPkgPlugin) Validate(_ context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	parser := helpers.NewConfigParser(config)

	// Validate config_path.
	configPath := parser.GetString("config_path", "", "nfpm.yaml")
	if err := validatePath(configPath); err != nil {
		vb.AddError("config_path", err.Error())
	}

	// Validate output_dir.
	outputDir := parser.GetString("output_dir", "", "dist")
	if err := validatePath(outputDir); err != nil {
		vb.AddError("output_dir", err.Error())
	}

	// Validate formats.
	formats := parser.GetStringSlice("formats", []string{"deb", "rpm"})
	for _, format := range formats {
		if err := validateFormat(format); err != nil {
			vb.AddError("formats", err.Error())
		}
	}

	// Validate target architecture.
	target := parser.GetString("target", "", "current")
	if err := validateArchitecture(target); err != nil {
		vb.AddError("target", err.Error())
	}

	// Validate packager.
	packager := parser.GetString("packager", "", "nfpm")
	if packager != "nfpm" && packager != "native" {
		vb.AddError("packager", "packager must be 'nfpm' or 'native'")
	}

	return vb.Build(), nil
}
