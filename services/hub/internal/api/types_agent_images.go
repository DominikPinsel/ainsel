package api

import (
	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// SimpleAgentImageResponse is the simplified API representation of an AgentImage.
type SimpleAgentImageResponse struct {
	ID            string                    `json:"id"`
	DisplayName   string                    `json:"displayName"`
	Description   string                    `json:"description,omitempty"`
	ImageURL      string                    `json:"imageURL"`
	ToolCount     int                       `json:"toolCount"`
	Tools         []AgentImageToolInfo      `json:"tools"`
	Env           []AgentImageEnvVarInfo    `json:"env,omitempty"`
	MCPServers    []AgentImageMCPServerInfo `json:"mcpServers,omitempty"`
	Sidecars      []AgentImageSidecarInfo   `json:"sidecars,omitempty"`
	EnabledSkills []string                  `json:"enabledSkills,omitempty"`
	Status        AgentImageStatusInfo      `json:"status"`
}

// AgentImageToolInfo is the API representation of an AgentImageTool.
type AgentImageToolInfo struct {
	Name        string                      `json:"name"`
	Kind        string                      `json:"kind"`
	Description string                      `json:"description,omitempty"`
	McpSource   string                      `json:"mcpSource,omitempty"`
	Enabled     bool                        `json:"enabled"`
	IsNew       bool                        `json:"isNew,omitempty"`
	Examples    []AgentImageToolExampleInfo `json:"examples,omitempty"`
}

// AgentImageToolExampleInfo is the API representation of an AgentImageToolExample.
type AgentImageToolExampleInfo struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// AgentImageEnvVarInfo is the API representation of an AgentImageEnvVar.
// When Secret is true, Value is always returned as "" to prevent leaking
// sensitive data through the API.
type AgentImageEnvVarInfo struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// AgentImageMCPServerInfo is the API representation of an AgentImageMCPServer.
// TokenFromEnv is the name of the agent-pod env var whose value is forwarded as
// the bearer token; it is not a secret.
type AgentImageMCPServerInfo struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	TokenFromEnv string `json:"tokenFromEnv,omitempty"`
}

// AgentImageSidecarInfo is the API representation of an AgentImageSidecar.
type AgentImageSidecarInfo struct {
	Name    string                 `json:"name"`
	Image   string                 `json:"image"`
	Port    int32                  `json:"port,omitempty"`
	MCPPath string                 `json:"mcpPath,omitempty"`
	Env     []AgentImageEnvVarInfo `json:"env,omitempty"`
}
type AgentImageStatusInfo struct {
	Phase string `json:"phase"`
}

// SimpleAgentImageCreateRequest is the POST body for creating an AgentImage.
type SimpleAgentImageCreateRequest struct {
	DisplayName   string                  `json:"displayName"`
	GroupID       string                  `json:"groupId"`
	Description   string                  `json:"description,omitempty"`
	ImageURL      string                  `json:"imageURL"`
	Env           []AgentImageEnvVarInfo  `json:"env,omitempty"`
	MCPServers    []string                `json:"mcpServers,omitempty"`
	Sidecars      []AgentImageSidecarInfo `json:"sidecars,omitempty"`
	EnabledSkills []string                `json:"enabledSkills,omitempty"`
}

// SimpleAgentImageUpdateRequest is the PUT body for updating an AgentImage.
// All fields are optional. A nil field means "do not modify"; a non-nil empty
// slice means "clear".
type SimpleAgentImageUpdateRequest struct {
	DisplayName   *string                  `json:"displayName,omitempty"`
	Description   *string                  `json:"description,omitempty"`
	ImageURL      *string                  `json:"imageURL,omitempty"`
	Tools         *[]AgentImageToolInfo    `json:"tools,omitempty"`
	Env           *[]AgentImageEnvVarInfo  `json:"env,omitempty"`
	MCPServers    *[]string                `json:"mcpServers,omitempty"`
	Sidecars      *[]AgentImageSidecarInfo `json:"sidecars,omitempty"`
	EnabledSkills *[]string                `json:"enabledSkills,omitempty"`
}

// toSimpleAgentImageResponse maps a CRD AgentImage into the API response type.
func toSimpleAgentImageResponse(img agentv1alpha1.AgentImage) SimpleAgentImageResponse {
	tools := make([]AgentImageToolInfo, 0, len(img.Spec.Tools))
	for _, t := range img.Spec.Tools {
		info := AgentImageToolInfo{
			Name:        t.Name,
			Kind:        string(t.Kind),
			Description: t.Description,
			McpSource:   t.McpSource,
			Enabled:     !t.Disabled,
			IsNew:       t.IsNew,
		}
		if len(t.Examples) > 0 {
			info.Examples = make([]AgentImageToolExampleInfo, 0, len(t.Examples))
			for _, ex := range t.Examples {
				info.Examples = append(info.Examples, AgentImageToolExampleInfo{
					Title:   ex.Title,
					Snippet: ex.Snippet,
				})
			}
		}
		tools = append(tools, info)
	}

	status := AgentImageStatusInfo{
		Phase: string(img.Status.Phase),
	}

	env := make([]AgentImageEnvVarInfo, 0, len(img.Spec.Env))
	for _, e := range img.Spec.Env {
		evInfo := AgentImageEnvVarInfo{Name: e.Name, Secret: e.Secret}
		if e.Secret {
			evInfo.Value = "" // never echo secret values
		} else {
			evInfo.Value = e.Value
		}
		env = append(env, evInfo)
	}

	mcpServers := make([]AgentImageMCPServerInfo, 0, len(img.Spec.MCPServers))
	for _, m := range img.Spec.MCPServers {
		mcpServers = append(mcpServers, AgentImageMCPServerInfo{
			Name:         m.Name,
			URL:          m.URL,
			TokenFromEnv: m.TokenFromEnv,
		})
	}

	enabledSkills := img.Spec.EnabledSkills
	if enabledSkills == nil {
		enabledSkills = []string{}
	}

	sidecars := make([]AgentImageSidecarInfo, 0, len(img.Spec.Sidecars))
	for _, sc := range img.Spec.Sidecars {
		scInfo := AgentImageSidecarInfo{
			Name:    sc.Name,
			Image:   sc.Image,
			Port:    sc.Port,
			MCPPath: sc.MCPPath,
		}
		for _, e := range sc.Env {
			evInfo := AgentImageEnvVarInfo{Name: e.Name, Secret: e.Secret}
			if e.Secret {
				evInfo.Value = ""
			} else {
				evInfo.Value = e.Value
			}
			scInfo.Env = append(scInfo.Env, evInfo)
		}
		sidecars = append(sidecars, scInfo)
	}

	return SimpleAgentImageResponse{
		ID:            img.Name,
		DisplayName:   img.Spec.DisplayName,
		Description:   img.Spec.Description,
		ImageURL:      img.Spec.ImageURL,
		ToolCount:     len(img.Spec.Tools),
		Tools:         tools,
		Env:           env,
		MCPServers:    mcpServers,
		Sidecars:      sidecars,
		EnabledSkills: enabledSkills,
		Status:        status,
	}
}
