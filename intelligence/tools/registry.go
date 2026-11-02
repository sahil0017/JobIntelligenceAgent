package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type Risk string

const (
	RiskReadOnly       Risk = "read_only"
	RiskExternalAction Risk = "external_action"
)

type Handler func(context.Context, json.RawMessage) (any, error)

type Tool struct {
	Name             string
	Description      string
	Risk             Risk
	RequiresApproval bool
	Handler          Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (registry *Registry) Register(tool Tool) error {
	if tool.Name == "" || tool.Handler == nil {
		return fmt.Errorf("tool name and handler are required")
	}
	if tool.Risk != RiskReadOnly && tool.Risk != RiskExternalAction {
		return fmt.Errorf("tool %q has an unsupported risk classification", tool.Name)
	}
	if tool.Risk == RiskExternalAction {
		tool.RequiresApproval = true
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.tools[tool.Name]; exists {
		return fmt.Errorf("tool %q is already registered", tool.Name)
	}
	registry.tools[tool.Name] = tool
	return nil
}

func (registry *Registry) Execute(ctx context.Context, name string, input json.RawMessage, approved bool) (any, error) {
	registry.mu.RLock()
	tool, exists := registry.tools[name]
	registry.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("tool %q is not registered", name)
	}
	if tool.RequiresApproval && !approved {
		return nil, fmt.Errorf("tool %q requires explicit user approval", name)
	}
	return tool.Handler(ctx, input)
}
