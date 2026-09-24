package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// i32 avoids a ptr import: another test file in this package declares a local
// symbol of that name.
func i32(v int32) *int32 { return &v }

func agentKey(name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: "test-ns"}
}

func TestScalingProblem(t *testing.T) {
	tests := []struct {
		name    string
		scaling *agentv1alpha1.AgentScaling
		want    string
	}{
		{"nil scaling is nothing to say", nil, ""},
		{"no floor means static", &agentv1alpha1.AgentScaling{Replicas: i32(3)}, ""},
		{"floor at zero is the whole point", &agentv1alpha1.AgentScaling{Replicas: i32(3), MinReplicas: i32(0)}, ""},
		{"floor below ceiling", &agentv1alpha1.AgentScaling{Replicas: i32(4), MinReplicas: i32(2)}, ""},
		{"floor equal to ceiling pins the count", &agentv1alpha1.AgentScaling{Replicas: i32(2), MinReplicas: i32(2)}, ""},
		{"zero ceiling with zero floor is just disabled", &agentv1alpha1.AgentScaling{Replicas: i32(0), MinReplicas: i32(0)}, ""},
		// An unset ceiling means one pod, and a caller asking for two warm pods
		// on it would otherwise be silently clamped.
		{"floor above an unset ceiling", &agentv1alpha1.AgentScaling{MinReplicas: i32(2)}, "minReplicas 2 cannot exceed replicas 1"},
		{"floor above ceiling", &agentv1alpha1.AgentScaling{Replicas: i32(3), MinReplicas: i32(4)}, "minReplicas 4 cannot exceed replicas 3"},
		{"negative floor", &agentv1alpha1.AgentScaling{Replicas: i32(3), MinReplicas: i32(-1)}, "minReplicas must not be negative"},
		{"negative ceiling", &agentv1alpha1.AgentScaling{Replicas: i32(-2)}, "replicas must not be negative"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scalingProblem(tc.scaling); got != tc.want {
				t.Errorf("scalingProblem() = %q, want %q", got, tc.want)
			}
		})
	}
}

func postAgentReq(t *testing.T, s *Server, req any) (int, string) {
	t.Helper()
	body, _ := json.Marshal(req)
	hreq := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, hreq)
	return rec.Code, rec.Body.String()
}

func putAgentReq(t *testing.T, s *Server, id string, req any) (int, string) {
	t.Helper()
	body, _ := json.Marshal(req)
	hreq := httptest.NewRequest(http.MethodPut, "/api/v1/agents/"+id, bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, hreq)
	return rec.Code, rec.Body.String()
}

func newAgentServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t, testAgentImage("img-1", "git"))
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgent)
	return s
}

func createAgent(t *testing.T, s *Server, req SimpleAgentCreateRequest) SimpleAgentResponse {
	t.Helper()
	code, body := postAgentReq(t, s, req)
	if code != http.StatusCreated {
		t.Fatalf("create agent: got %d, want 201: %s", code, body)
	}
	var out SimpleAgentResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode create response: %v (%s)", err, body)
	}
	return out
}

func TestCreateAgent_QueueScalingRoundTrip(t *testing.T) {
	s := newAgentServer(t)

	created := createAgent(t, s, SimpleAgentCreateRequest{
		Name:        "Dormant Agent",
		ImageRef:    AgentImageRefInfo{Name: "img-1"},
		LLM:         AgentLLMInfo{Model: "glm-5.1:cloud"},
		Replicas:    i32(5),
		MinReplicas: i32(0),
	})
	if created.MinReplicas == nil || *created.MinReplicas != 0 {
		t.Errorf("response minReplicas = %v, want 0", created.MinReplicas)
	}

	var stored agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), agentKey(created.ID), &stored); err != nil {
		t.Fatalf("read back agent: %v", err)
	}
	if stored.Spec.Scaling == nil || stored.Spec.Scaling.MinReplicas == nil || *stored.Spec.Scaling.MinReplicas != 0 {
		t.Fatalf("stored scaling = %+v, want minReplicas 0", stored.Spec.Scaling)
	}
	if stored.Spec.Scaling.Replicas == nil || *stored.Spec.Scaling.Replicas != 5 {
		t.Errorf("stored replicas = %v, want 5 as the ceiling", stored.Spec.Scaling.Replicas)
	}
}

// An impossible floor is refused rather than clamped: clamping turns "keep two
// warm" into "keep one warm" with no signal to whoever wrote it.
func TestCreateAgent_RejectsFloorAboveCeiling(t *testing.T) {
	s := newAgentServer(t)

	code, body := postAgentReq(t, s, SimpleAgentCreateRequest{
		Name:        "Bad Agent",
		ImageRef:    AgentImageRefInfo{Name: "img-1"},
		LLM:         AgentLLMInfo{Model: "glm-5.1:cloud"},
		Replicas:    i32(2),
		MinReplicas: i32(3),
	})
	if code != http.StatusBadRequest {
		t.Fatalf("create with floor above ceiling: got %d, want 400 (%s)", code, body)
	}
	if !strings.Contains(body, "minReplicas 3 cannot exceed replicas 2") {
		t.Errorf("error body = %s, want it to name the offending pair", body)
	}
}

func TestUpdateAgent_MinReplicas(t *testing.T) {
	s := newAgentServer(t)

	created := createAgent(t, s, SimpleAgentCreateRequest{
		Name:     "Static Agent",
		ImageRef: AgentImageRefInfo{Name: "img-1"},
		LLM:      AgentLLMInfo{Model: "glm-5.1:cloud"},
		Replicas: i32(4),
	})
	if created.MinReplicas != nil {
		t.Fatalf("an agent created without a floor should have none, got %v", created.MinReplicas)
	}

	// Opt in.
	if code, body := putAgentReq(t, s, created.ID, SimpleAgentUpdateRequest{MinReplicas: i32(0)}); code != http.StatusOK {
		t.Fatalf("opting into scale-to-zero: got %d (%s)", code, body)
	}
	var opted agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), agentKey(created.ID), &opted); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if opted.Spec.Scaling.MinReplicas == nil || *opted.Spec.Scaling.MinReplicas != 0 {
		t.Errorf("stored minReplicas = %v, want 0", opted.Spec.Scaling.MinReplicas)
	}
	// The ceiling must survive a floor-only update: overwriting the whole scaling
	// struct here would silently drop replicas back to the one-pod default.
	if opted.Spec.Scaling.Replicas == nil || *opted.Spec.Scaling.Replicas != 4 {
		t.Errorf("stored replicas = %v, want the existing ceiling 4 to survive", opted.Spec.Scaling.Replicas)
	}

	// A floor the ceiling cannot carry is refused, and the refused request must
	// not have touched the stored agent.
	if code, _ := putAgentReq(t, s, created.ID, SimpleAgentUpdateRequest{MinReplicas: i32(9)}); code != http.StatusBadRequest {
		t.Fatalf("floor above ceiling: got %d, want 400", code)
	}
	var after agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), agentKey(created.ID), &after); err != nil {
		t.Fatalf("read back after rejection: %v", err)
	}
	if after.Spec.Scaling.MinReplicas == nil || *after.Spec.Scaling.MinReplicas != 0 {
		t.Errorf("rejected update changed the agent: minReplicas = %v, want the previous 0", after.Spec.Scaling.MinReplicas)
	}
}

// The UI needs the operator's intent, not just the current count: with zero pods,
// "asleep" and "cannot schedule" look identical otherwise.
func TestAgentResponse_ExposesOperatorScalingIntent(t *testing.T) {
	s := newAgentServer(t)

	created := createAgent(t, s, SimpleAgentCreateRequest{
		Name:        "Sleepy Agent",
		ImageRef:    AgentImageRefInfo{Name: "img-1"},
		LLM:         AgentLLMInfo{Model: "glm-5.1:cloud"},
		Replicas:    i32(3),
		MinReplicas: i32(0),
	})

	var stored agentv1alpha1.Agent
	if err := s.client.Get(context.Background(), agentKey(created.ID), &stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	stored.Status.Replicas = 0
	stored.Status.Scaling = &agentv1alpha1.AgentScalingStatus{
		Mode:    "queue",
		Desired: 0,
		Reason:  "ScaledToZero",
		Message: "quiet for 4m0s",
	}
	if err := s.client.Status().Update(context.Background(), &stored); err != nil {
		t.Fatalf("set status: %v", err)
	}

	code, body := putAgentReq(t, s, created.ID, SimpleAgentUpdateRequest{})
	if code != http.StatusOK {
		t.Fatalf("read via update: got %d (%s)", code, body)
	}
	var resp SimpleAgentResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status == nil {
		t.Fatal("response has no status")
	}
	if resp.Status.Desired != 0 || resp.Status.Mode != "queue" || resp.Status.Reason != "ScaledToZero" {
		t.Errorf("status = %+v, want the operator's queue/asleep/ScaledToZero account", resp.Status)
	}
	if !strings.Contains(resp.Status.Message, "quiet for") {
		t.Errorf("status message = %q, want the operator's explanation", resp.Status.Message)
	}
}
