package api

import (
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// handlePlatformHealth serves GET /api/v1/platform/health.
// Returns a summary of all pod statuses in the hub's namespace.
func (s *Server) handlePlatformHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var pods corev1.PodList
	if err := s.client.List(r.Context(), &pods, client.InNamespace(s.ns)); err != nil {
		writeError(w, http.StatusBadGateway, "failed to list pods: "+err.Error())
		return
	}

	type containerSummary struct {
		Name     string `json:"name"`
		Ready    bool   `json:"ready"`
		State    string `json:"state"`
		Restarts int32  `json:"restarts"`
	}
	type podSummary struct {
		Name       string             `json:"name"`
		Phase      string             `json:"phase"`
		Ready      string             `json:"ready"`
		Restarts   int32              `json:"restarts"`
		Containers []containerSummary `json:"containers"`
	}

	summaries := make([]podSummary, 0, len(pods.Items))
	for _, pod := range pods.Items {
		var totalRestarts int32
		readyCount := 0
		containers := make([]containerSummary, 0, len(pod.Status.ContainerStatuses))
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
			if cs.Ready {
				readyCount++
			}
			state := "unknown"
			switch {
			case cs.State.Running != nil:
				state = "running"
			case cs.State.Waiting != nil:
				state = "waiting: " + cs.State.Waiting.Reason
			case cs.State.Terminated != nil:
				state = "terminated: " + cs.State.Terminated.Reason
			}
			containers = append(containers, containerSummary{
				Name:     cs.Name,
				Ready:    cs.Ready,
				State:    state,
				Restarts: cs.RestartCount,
			})
		}
		summaries = append(summaries, podSummary{
			Name:       pod.Name,
			Phase:      string(pod.Status.Phase),
			Ready:      fmt.Sprintf("%d/%d", readyCount, len(pod.Status.ContainerStatuses)),
			Restarts:   totalRestarts,
			Containers: containers,
		})
	}

	writeJSON(w, http.StatusOK, summaries)
}
