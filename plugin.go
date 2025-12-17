// Package main implements the LinuxPkg plugin for Relicta.
package main

import (
	"context"
	"fmt"

	"github.com/relicta-tech/relicta-plugin-sdk/helpers"
	"github.com/relicta-tech/relicta-plugin-sdk/plugin"
)

// LinuxPkgPlugin implements the Build deb/rpm packages for Linux plugin.
type LinuxPkgPlugin struct{}

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
			"properties": {}
		}`,
	}
}

// Execute runs the plugin for a given hook.
func (p *LinuxPkgPlugin) Execute(ctx context.Context, req plugin.ExecuteRequest) (*plugin.ExecuteResponse, error) {
	switch req.Hook {
	case plugin.HookPostPublish:
		if req.DryRun {
			return &plugin.ExecuteResponse{
				Success: true,
				Message: "Would execute linuxpkg plugin",
			}, nil
		}
		return &plugin.ExecuteResponse{
			Success: true,
			Message: "LinuxPkg plugin executed successfully",
		}, nil
	default:
		return &plugin.ExecuteResponse{
			Success: true,
			Message: fmt.Sprintf("Hook %s not handled", req.Hook),
		}, nil
	}
}

// Validate validates the plugin configuration.
func (p *LinuxPkgPlugin) Validate(_ context.Context, config map[string]any) (*plugin.ValidateResponse, error) {
	vb := helpers.NewValidationBuilder()
	return vb.Build(), nil
}
