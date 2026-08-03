package mcp

import (
	"log/slog"
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/DominikPinsel/ainsel/services/mcp/internal/tools"
)

// Backends holds connection details for all backend services.
// All MCP tools route through the hub; no direct backend connections needed.
type Backends struct {
	HubURL string
}

// New creates a configured MCP server with all tools registered.
func New(log *slog.Logger, b Backends) *server.MCPServer {
	s := server.NewMCPServer(
		"ainsel-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	agents := tools.NewAgentTools(b.HubURL)
	s.AddTool(agents.ListAgentsTool(), agents.ListAgents)
	s.AddTool(agents.GetAgentTool(), agents.GetAgent)
	s.AddTool(agents.UpdateAgentTool(), agents.UpdateAgent)

	triggers := tools.NewTriggerTools(b.HubURL)
	s.AddTool(triggers.ListTriggersTool(), triggers.ListTriggers)
	s.AddTool(triggers.GetTriggerTool(), triggers.GetTrigger)
	s.AddTool(triggers.DeleteTriggerTool(), triggers.DeleteTrigger)
	s.AddTool(triggers.CreateTriggerTool(), triggers.CreateTrigger)
	s.AddTool(triggers.UpdateTriggerTool(), triggers.UpdateTrigger)

	cronTriggers := tools.NewCronTriggerTools(b.HubURL)
	s.AddTool(cronTriggers.ListCronTriggersTool(), cronTriggers.ListCronTriggers)
	s.AddTool(cronTriggers.GetCronTriggerTool(), cronTriggers.GetCronTrigger)
	s.AddTool(cronTriggers.CreateCronTriggerTool(), cronTriggers.CreateCronTrigger)
	s.AddTool(cronTriggers.UpdateCronTriggerTool(), cronTriggers.UpdateCronTrigger)
	s.AddTool(cronTriggers.DeleteCronTriggerTool(), cronTriggers.DeleteCronTrigger)

	connectors := tools.NewConnectorTools(b.HubURL)
	s.AddTool(connectors.ListConnectorsTool(), connectors.ListConnectors)
	s.AddTool(connectors.GetConnectorTool(), connectors.GetConnector)

	workflows := tools.NewWorkflowTools(b.HubURL)
	s.AddTool(workflows.SummarizeWorkflowsTool(), workflows.SummarizeWorkflows)

	invocations := tools.NewInvocationTools(b.HubURL)
	s.AddTool(invocations.ListInvocationsTool(), invocations.ListInvocations)
	s.AddTool(invocations.GetInvocationTool(), invocations.GetInvocation)

	activity := tools.NewActivityTools(b.HubURL)
	s.AddTool(activity.SummarizeAgentActivityTool(), activity.SummarizeAgentActivity)

	agentImages := tools.NewAgentImageTools(b.HubURL)
	s.AddTool(agentImages.ListAgentImagesTool(), agentImages.ListAgentImages)
	s.AddTool(agentImages.GetAgentImageTool(), agentImages.GetAgentImage)
	s.AddTool(agentImages.CreateAgentImageTool(), agentImages.CreateAgentImage)
	s.AddTool(agentImages.UpdateAgentImageTool(), agentImages.UpdateAgentImage)
	s.AddTool(agentImages.DeleteAgentImageTool(), agentImages.DeleteAgentImage)

	mcpServers := tools.NewMCPServerTools(b.HubURL)
	s.AddTool(mcpServers.ListMCPServersTool(), mcpServers.ListMCPServers)
	s.AddTool(mcpServers.GetMCPServerTool(), mcpServers.GetMCPServer)

	personas := tools.NewPersonaTools(b.HubURL)
	s.AddTool(personas.ListPersonasTool(), personas.ListPersonas)
	s.AddTool(personas.GetPersonaTool(), personas.GetPersona)
	s.AddTool(personas.ListPersonaVersionsTool(), personas.ListPersonaVersions)
	s.AddTool(personas.GetPersonaVersionTool(), personas.GetPersonaVersion)
	s.AddTool(personas.CreatePersonaTool(), personas.CreatePersona)
	s.AddTool(personas.UpdatePersonaTool(), personas.UpdatePersona)
	s.AddTool(personas.DeletePersonaTool(), personas.DeletePersona)

	skills := tools.NewSkillTools(b.HubURL)
	s.AddTool(skills.ListSkillsTool(), skills.ListSkills)
	s.AddTool(skills.GetSkillTool(), skills.GetSkill)
	s.AddTool(skills.CreateSkillTool(), skills.CreateSkill)
	s.AddTool(skills.UpdateSkillTool(), skills.UpdateSkill)
	s.AddTool(skills.DeleteSkillTool(), skills.DeleteSkill)

	errs := tools.NewErrorTools(b.HubURL)
	s.AddTool(errs.GetRecentErrorsTool(), errs.GetRecentErrors)

	usage := tools.NewUsageTools(b.HubURL)
	s.AddTool(usage.GetTokenUsageTool(), usage.GetTokenUsage)
	s.AddTool(usage.GetStatsTool(), usage.GetStats)

	events := tools.NewEventTools(b.HubURL)
	s.AddTool(events.GetStreamInfoTool(), events.GetStreamInfo)
	s.AddTool(events.ListRecentEventsTool(), events.ListRecentEvents)

	logs := tools.NewLogTools(b.HubURL)
	s.AddTool(logs.GetAgentLogsTool(), logs.GetAgentLogs)
	s.AddTool(logs.QueryLogsTool(), logs.QueryLogs)

	metrics := tools.NewMetricsTools(b.HubURL)
	s.AddTool(metrics.GetAgentMetricsTool(), metrics.GetAgentMetrics)
	s.AddTool(metrics.QueryMetricsTool(), metrics.QueryMetrics)

	health := tools.NewHealthTools(b.HubURL)
	s.AddTool(health.GetPlatformHealthTool(), health.GetPlatformHealth)

	_ = log
	return s
}

// StreamableHTTPHandler returns an http.Handler for the MCP streamable HTTP transport.
func StreamableHTTPHandler(s *server.MCPServer) http.Handler {
	return server.NewStreamableHTTPServer(s)
}