package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
	sharedpersonas "github.com/DominikPinsel/ainsel/shared/api/personas"
	sharedskills "github.com/DominikPinsel/ainsel/shared/api/skills"

	"github.com/DominikPinsel/ainsel/operators/agent/internal/controller/mcpservers"
)

const (
	// monitoringAPIVersion is the apiVersion of the prometheus-operator CRDs.
	monitoringAPIVersion = "monitoring.coreos.com/v1"
	// serviceMonitorKind is the prometheus-operator Kind that registers a scrape target.
	serviceMonitorKind = "ServiceMonitor"
	// agentTerminationGracePeriodSeconds gives a long-running agent task
	// time to finish on SIGTERM before Kubernetes SIGKILLs the pod.
	agentTerminationGracePeriodSeconds = int64(1800)
	// secretHashAnnotation is stamped on the Deployment pod template. When
	// the value changes Kubernetes performs a rolling restart automatically.
	secretHashAnnotation = "ainsel.dev/secret-hash" // #nosec G101 -- annotation key, not a credential
	// personaHashAnnotation is stamped on the Deployment pod template alongside
	// secretHashAnnotation. When the persona ConfigMap data changes, the hash
	// changes, which causes Kubernetes to perform a rolling restart automatically.
	personaHashAnnotation = "ainsel.dev/persona-hash"
	// skillHashAnnotation is stamped on the Deployment pod template alongside
	// secretHashAnnotation and personaHashAnnotation. When the shared skills
	// ConfigMap data changes, the hash changes, which causes Kubernetes to
	// perform a rolling restart automatically so agents pick up skill updates.
	skillHashAnnotation = "ainsel.dev/skill-hash"
)

// AgentReconciler reconciles a Agent object
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Recorder emits Kubernetes Events for the agent-controller. May be
	// nil — in that case Events are skipped (useful in unit tests that
	// don't need to assert on Events).
	Recorder record.EventRecorder

	// AgentGracePeriod is the pod terminationGracePeriodSeconds for agent
	// deployments. Zero uses the default (1800).
	AgentGracePeriod int64
}

//+kubebuilder:rbac:groups=ainsel.dev,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ainsel.dev,resources=agents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ainsel.dev,resources=agents/finalizers,verbs=update
//+kubebuilder:rbac:groups=ainsel.dev,resources=agentimages,verbs=get;list;watch

// gracePeriod returns the configured agent grace period or the default.
func (r *AgentReconciler) gracePeriod() int64 {
	if r.AgentGracePeriod > 0 {
		return r.AgentGracePeriod
	}
	return agentTerminationGracePeriodSeconds
}

//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile reconciles an Agent CR by ensuring the associated ConfigMap, Deployment, and Service exist.
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// 1. Fetch the Agent CR
	var agent ainselv1alpha1.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Agent resource not found, probably deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1.5. Finalizer handling — ensure the JetStream consumer is cleaned up
	// before the CR is removed.
	if !agent.DeletionTimestamp.IsZero() {
		// The CR is being deleted.
		return ctrl.Result{}, nil
	}

	agentName := fmt.Sprintf("agent-%s", agent.Name)

	// 2. Resolve the AgentImage referenced by spec.imageRef.name.
	var img ainselv1alpha1.AgentImage
	if err := r.Get(ctx, types.NamespacedName{Namespace: agent.Namespace, Name: agent.Spec.ImageRef.Name}, &img); err != nil {
		if apierrors.IsNotFound(err) {
			apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               ainselv1alpha1.AgentConditionReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ImageNotFound",
				Message:            fmt.Sprintf("AgentImage %q not found in namespace %q", agent.Spec.ImageRef.Name, agent.Namespace),
				LastTransitionTime: metav1.Now(),
			})
			if updErr := r.Status().Update(ctx, &agent); updErr != nil {
				return ctrl.Result{}, updErr
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	podImage := img.Spec.ImageURL

	// 4. Defensive guardrail: the CRD requires spec.persona.id (minLength 1),
	// but verify here so we surface a controller-side diagnostic if a request
	// slips through (e.g. via a stale client). The hub renders the
	// "persona-<id>" ConfigMap when the persona row exists.
	if agent.Spec.Persona.ID == "" {
		return ctrl.Result{}, fmt.Errorf("agent %s: spec.persona.id is required", agent.Name)
	}

	// 4.5. ConfigMap with Pi models.json for ollama-cloud agents (must exist before Deployment)
	if err := r.reconcilePiModelsConfigMap(ctx, &agent, agentName); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling Pi models ConfigMap: %w", err)
	}

	// 4.75. Secret for AgentImage env vars (owned by the Agent for GC)
	if err := r.reconcileImageEnvSecret(ctx, &agent, agentName, &img); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling image env secret: %w", err)
	}
	apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
		Type:               ainselv1alpha1.AgentConditionImageEnvSecretReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Image env secret is up to date",
		LastTransitionTime: metav1.Now(),
	})

	// 5. Deployment
	deploy, mcpMissingEnv, err := r.reconcileDeployment(ctx, &agent, agentName, &img, podImage)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 5.5. Surface MCP tokenFromEnv misconfiguration via Degraded condition
	// and Warning Events so operators can discover the problem via
	// kubectl describe agent and kubectl get events.
	r.reconcileMCPTokenEnvCondition(ctx, &agent, mcpMissingEnv)

	// 5.6. MCP discovery condition
	if len(mcpMissingEnv) == 0 {
		apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               ainselv1alpha1.AgentConditionMCPDiscoveryComplete,
			Status:             metav1.ConditionTrue,
			Reason:             "AllResolved",
			Message:            "All enabled MCP servers resolved successfully",
			LastTransitionTime: metav1.Now(),
		})
	} else {
		var names []string
		for _, m := range mcpMissingEnv {
			names = append(names, m.ServerName)
		}
		apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               ainselv1alpha1.AgentConditionMCPDiscoveryComplete,
			Status:             metav1.ConditionFalse,
			Reason:             "MissingEnvVars",
			Message:            fmt.Sprintf("MCP servers with missing env vars: %s", strings.Join(names, ", ")),
			LastTransitionTime: metav1.Now(),
		})
	}

	// 5.7. Persona ConfigMap condition.
	// A missing persona ConfigMap is non-blocking: the hub may not have
	// rendered it yet. We surface it as PersonaConfigMapReady=False so
	// operators can see the dependency, but reconciliation continues.
	personaCMName := sharedpersonas.PersonaConfigMapName(agent.Spec.Persona.ID)
	personaCM := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: agent.Namespace, Name: personaCMName}, personaCM); err != nil {
		if apierrors.IsNotFound(err) {
			apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               ainselv1alpha1.AgentConditionPersonaConfigMapReady,
				Status:             metav1.ConditionFalse,
				Reason:             "NotFound",
				Message:            fmt.Sprintf("Persona ConfigMap %q not found in namespace %q", personaCMName, agent.Namespace),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			return ctrl.Result{}, err
		}
	} else {
		apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               ainselv1alpha1.AgentConditionPersonaConfigMapReady,
			Status:             metav1.ConditionTrue,
			Reason:             "Exists",
			Message:            fmt.Sprintf("Persona ConfigMap %q exists", personaCMName),
			LastTransitionTime: metav1.Now(),
		})
	}

	// 6. Service for metrics
	if err := r.reconcileMetricsService(ctx, &agent, agentName); err != nil {
		return ctrl.Result{}, err
	}

	// 7. ServiceMonitor so Prometheus scrapes the metrics Service.
	if err := r.reconcileServiceMonitor(ctx, &agent, agentName); err != nil {
		return ctrl.Result{}, err
	}

	// 8. Update status
	if err := r.updateStatus(ctx, &agent, deploy); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled Agent", "agent", agent.Name)
	// Requeue periodically so the operator detects new image digests
	// behind mutable tags (e.g. :dev) and restarts agent pods.
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

type piProviderConfig struct {
	// Name is the provider key in Pi's models.json and the --provider flag value.
	Name string
	// BaseURL is the OpenAI-completions base URL.
	BaseURL string
	// APIKeyEnv is the environment variable name that holds the API key.
	APIKeyEnv string
	// Compat is an optional JSON object string for Pi model compat flags.
	Compat string
	// ProviderCompat is an optional JSON object string emitted at the
	// provider level in models.json (e.g. supportsDeveloperRole, maxTokensField).
	ProviderCompat string
	// ModelCompat is an optional JSON object string emitted at the model
	// level in models.json (e.g. thinkingFormat).
	ModelCompat string
	// ThinkingLevelMap is an optional JSON object string that maps pi
	// thinking levels to provider-specific values.
	ThinkingLevelMap string
}

// resolvePiProvider returns the Pi CLI provider configuration for the
// agent's LLM provider. When no hardcoded provider is configured, the controller
// checks for a custom provider (spec.customProvider) and uses it if present;
// otherwise defaults to ollama-cloud for backwards compatibility.
func resolvePiProvider(agent *ainselv1alpha1.Agent) piProviderConfig {
	switch agent.Spec.LLM.Provider {
	case ainselv1alpha1.AgentLLMProviderOpenCode:
		return piProviderConfig{
			Name:      "opencode",
			BaseURL:   "https://opencode.ai/zen/go/v1",
			APIKeyEnv: "OPENCODE_API_KEY", // #nosec G101 -- env var name, not a hardcoded value
			Compat:    `,"compat":{"requiresReasoningContentOnAssistantMessages":true}`,
		}
	case ainselv1alpha1.AgentLLMProviderAlibabaCloud:
		return piProviderConfig{
			Name:             "alibaba-cloud",
			BaseURL:          "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1",
			APIKeyEnv:        "ALIBABA_CLOUD_API_KEY", // #nosec G101 -- env var name, not a hardcoded value
			ProviderCompat:   `"compat":{"supportsDeveloperRole":false,"supportsUsageInStreaming":false,"maxTokensField":"max_tokens","supportsReasoningEffort":false}`,
			ModelCompat:      `"compat":{"thinkingFormat":"qwen"}`,
			ThinkingLevelMap: `"thinkingLevelMap":{"off":null,"minimal":null,"low":null,"medium":"medium","high":"high","xhigh":null}`,
		}
	case ainselv1alpha1.AgentLLMProviderCustom:
		// Use "openai" as the provider key so Pi CLI recognizes it.
		// The custom baseUrl is injected via models.json.
		cfg := piProviderConfig{
			Name:      "openai",
			APIKeyEnv: "CUSTOM_PROVIDER_API_KEY",
			Compat:    "",
		}
		if agent.Spec.CustomProvider != nil {
			cfg.BaseURL = agent.Spec.CustomProvider.URL
		}
		return cfg
	default:
		// ollama-cloud (default)
		return piProviderConfig{
			Name:      "ollama-api-key",
			BaseURL:   "https://ollama.com/v1",
			APIKeyEnv: "OLLAMA_API_KEY", // #nosec G101 -- env var name, not a hardcoded value
		}
	}
}


// resolveHubURL returns the hub backend REST API URL that MCP sidecars
// (e.g. the chat-mcp sidecar) use to proxy requests back to the hub. The
// operator reads HUB_URL from its own environment; the chart injects it
// on the operator Deployment.
func resolveHubURL() string {
	if v := os.Getenv("HUB_URL"); v != "" {
		return v
	}
	return ""
}

// resolveHubInternalToken returns the shared secret that MCP sidecars use
// to bypass OIDC auth on the hub backend. The operator reads
// HUB_INTERNAL_VALIDATE_SECRET from its own environment; the chart injects
// it on the operator Deployment.
func resolveHubInternalToken() string {
	return os.Getenv("HUB_INTERNAL_VALIDATE_SECRET")
}

// resolveChatMCPImage returns the container image for the chat-mcp sidecar.
// The operator reads CHAT_MCP_IMAGE from its own environment; the chart
// injects it on the operator Deployment. When unset the sidecar is not
// injected, keeping the operator safe in environments where chat is not
// configured.
func resolveChatMCPImage() string {
	return os.Getenv("CHAT_MCP_IMAGE")
}

// platformSidecarEnv returns env vars that the operator automatically
// injects into every sidecar container that declares an MCPPath (i.e. MCP
// sidecars like the chat-mcp sidecar). These are platform-level vars the
// sidecar needs to communicate with the hub; they should not have to be
// configured manually per AgentImage.
func platformSidecarEnv() []corev1.EnvVar {
	var envs []corev1.EnvVar
	if hubURL := resolveHubURL(); hubURL != "" {
		envs = append(envs, corev1.EnvVar{Name: "HUB_URL", Value: hubURL})
	}
	if token := resolveHubInternalToken(); token != "" {
		envs = append(envs, corev1.EnvVar{Name: "HUB_INTERNAL_VALIDATE_SECRET", Value: token})
	}
	return envs
}

// desiredReplicas returns the replica count for an agent's Deployment.
// Defaults to 1 when spec.scaling is nil or spec.scaling.replicas is unset.
func desiredReplicas(agent *ainselv1alpha1.Agent) int32 {
	if agent.Spec.Scaling != nil && agent.Spec.Scaling.Replicas != nil {
		return *agent.Spec.Scaling.Replicas
	}
	return 1
}

func (r *AgentReconciler) reconcileDeployment(ctx context.Context, agent *ainselv1alpha1.Agent, agentName string, img *ainselv1alpha1.AgentImage, podImage string) (*appsv1.Deployment, []mcpservers.MissingEnvEntry, error) {
	log := logf.FromContext(ctx)

	// missingEnv captures MCP servers whose tokenFromEnv references an
	// env var not defined on the AgentImage. It is populated inside the
	// CreateOrUpdate closure and returned to the caller so the Reconcile
	// loop can set a Degraded condition and emit Warning Events.
	var missingEnv []mcpservers.MissingEnvEntry

	labels := map[string]string{
		"app.kubernetes.io/name":       agentName,
		"app.kubernetes.io/managed-by": "agent-operator",
		"app.kubernetes.io/component":  "agents",
		"ainsel.dev/agent":             agent.Name,
	}

	// Resolve each enabled MCP name to a runtime URL by looking up its
	// Service in the agent's namespace. Missing Services are logged and
	// skipped — the agent still rolls out.
	mcpEntries, mcpMissing, err := mcpservers.Discover(ctx, r.Client, agent.Namespace, agent.Spec.EnabledMCPs)
	if err != nil {
		log.Error(err, "discover MCP services")
		return nil, nil, err
	}
	for _, n := range mcpMissing {
		log.Info("MCP service not found, skipping", "agent", agent.Name, "mcp", n)
	}

	imagePullPolicy := agent.Spec.Runtime.ImagePullPolicy
	if imagePullPolicy == "" {
		imagePullPolicy = corev1.PullAlways
	}

	// Determine persona volume source. Hub renders this ConfigMap in
	// the hub namespace using the shared naming helper; see
	// shared/api/personas.PersonaConfigMapName — the single source of truth
	// for the format shared by producer (hub) and consumer (this operator).
	personaCMName := sharedpersonas.PersonaConfigMapName(agent.Spec.Persona.ID)

	agentVolumeMounts := []corev1.VolumeMount{
		{Name: "persona", MountPath: "/etc/agent/", ReadOnly: true},
		{Name: "workspace", MountPath: "/workspace"},
	}
	podVolumes := []corev1.Volume{
		{
			Name: "persona",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: personaCMName},
				},
			},
		},
		{
			Name:         "workspace",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
	var initContainers []corev1.Container

	// Pi config mount. Pi expects ~/.pi/agent/ to be writable (it creates
	// sessions/ etc.), so we use an emptyDir for the directory and an init
	// container to copy models.json from the ConfigMap into it.
	piModelsCMName := agentName + "-pi-models"
	podVolumes = append(podVolumes,
		corev1.Volume{
			Name:         "pi-home",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		corev1.Volume{
			Name: "pi-models",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: piModelsCMName},
				},
			},
		},
	)
	agentVolumeMounts = append(agentVolumeMounts, corev1.VolumeMount{
		Name:      "pi-home",
		MountPath: "/home/agent/.pi/agent",
	})
	// Skills are sourced from the single shared hub-managed ConfigMap
	// (sharedskills.ConfigMapName). The operator projects only the items
	// the AgentImage enabled via spec.enabledSkills so that pods that
	// don't reference a skill don't get it. Each enabled skill key is
	// projected as <id>/SKILL.md so Pi discovers it natively under
	// /home/agent/.pi/agent/skills/<id>/SKILL.md (handled by the init
	// container copy below).
	// Determine whether security hardening is enabled. Defaults to true
	// when spec.runtime.securityHardened is nil.
	securityHardened := agent.Spec.Runtime.SecurityHardened == nil || *agent.Spec.Runtime.SecurityHardened

	// With fsGroup: 1000, emptyDir volumes are group-owned by GID 1000,
	// so the init container (running as UID 1000) can write without chown
	// when hardening is on. When hardening is off, the init container runs
	// as root and chown is needed so UID 1000 can read the copied files.
	setupPiCommand := "cp /var/pi-models/models.json /home/agent/.pi/agent/models.json"
	if !securityHardened {
		setupPiCommand += " && chown 1000:1000 /home/agent/.pi/agent/models.json"
	}
	piInitVolumeMounts := []corev1.VolumeMount{
		{Name: "pi-home", MountPath: "/home/agent/.pi/agent"},
		{Name: "pi-models", MountPath: "/var/pi-models", ReadOnly: true},
	}
	if len(img.Spec.EnabledSkills) > 0 {
		skillItems := make([]corev1.KeyToPath, 0, len(img.Spec.EnabledSkills))
		for _, id := range img.Spec.EnabledSkills {
			skillItems = append(skillItems, corev1.KeyToPath{
				Key:  id,
				Path: id + "/SKILL.md",
			})
		}
		podVolumes = append(podVolumes, corev1.Volume{
			Name: "agent-skills",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: sharedskills.ConfigMapName},
					Items:                skillItems,
				},
			},
		})
		piInitVolumeMounts = append(piInitVolumeMounts, corev1.VolumeMount{
			Name:      "agent-skills",
			MountPath: "/var/agent-skills",
			ReadOnly:  true,
		})
		setupPiCommand += " && cp -r /var/agent-skills/. /home/agent/.pi/agent/skills/"
		if !securityHardened {
			setupPiCommand += " && chown -R 1000:1000 /home/agent/.pi/agent/skills"
		}
	}
	initContainer := corev1.Container{
		Name:  "setup-pi-models",
		Image: "busybox:1.37",
		Command: []string{
			"sh", "-c",
			setupPiCommand,
		},
		VolumeMounts: piInitVolumeMounts,
	}
	if securityHardened {
		initContainer.SecurityContext = &corev1.SecurityContext{
			RunAsUser:                ptr.To(int64(1000)),
			RunAsNonRoot:             ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		}
	}
	initContainers = append(initContainers, initContainer)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName,
			Namespace: agent.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		if err := controllerutil.SetControllerReference(agent, deploy, r.Scheme); err != nil {
			return err
		}

		deploy.Labels = labels

		// Build pod securityContext based on the securityHardened flag.
		var podSecContext *corev1.PodSecurityContext
		if securityHardened {
			podSecContext = &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(1000)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			}
		} else {
			podSecContext = &corev1.PodSecurityContext{
				FSGroup: ptr.To(int64(1000)),
			}
		}

		// Build volumes, adding /tmp emptyDir when hardening is on.
		// Copy the slice to avoid mutating the shared backing array.
		deployVolumes := append([]corev1.Volume{}, podVolumes...)
		if securityHardened {
			deployVolumes = append(deployVolumes, corev1.Volume{
				Name:         "tmp",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			})
		}

		// Build agent container volume mounts, adding /tmp when hardening is on.
		// Copy the slice to avoid mutating the shared backing array.
		agentMounts := append([]corev1.VolumeMount{}, agentVolumeMounts...)
		if securityHardened {
			agentMounts = append(agentMounts, corev1.VolumeMount{
				Name:      "tmp",
				MountPath: "/tmp",
			})
		}

		// Build agent container securityContext.
		var agentSecContext *corev1.SecurityContext
		if securityHardened {
			agentSecContext = &corev1.SecurityContext{
				AllowPrivilegeEscalation: ptr.To(false),
				ReadOnlyRootFilesystem:   ptr.To(true),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
			}
		}

		deploy.Spec = appsv1.DeploymentSpec{
			Replicas: ptr.To(desiredReplicas(agent)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext:               podSecContext,
					TerminationGracePeriodSeconds: ptr.To(r.gracePeriod()),
					InitContainers:                initContainers,
					Containers: []corev1.Container{
						{
							Name:            "agent",
							Image:           podImage,
							ImagePullPolicy: imagePullPolicy,
							Resources:       agent.Spec.Runtime.Resources,
							SecurityContext: agentSecContext,
							Ports: []corev1.ContainerPort{
								{Name: "health", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
								{Name: "metrics", ContainerPort: 9090, Protocol: corev1.ProtocolTCP},
							},
							Env: func() []corev1.EnvVar {
								envs := []corev1.EnvVar{
									{Name: "AGENT_NAME", Value: agent.Name},
									{Name: "PI_PROVIDER", Value: resolvePiProvider(agent).Name},
									{Name: "OLLAMA_CLOUD_MODEL", Value: agent.Spec.LLM.Model},
									{Name: "AGENT_PERSONA_PATH", Value: "/etc/agent/persona.md"},
									{Name: "HUB_URL", Value: resolveHubURL()},
									{Name: "AGENT_TOKEN", Value: resolveHubInternalToken()},
									{Name: "HUB_ENABLED", Value: "true"},
								}
								// OLLAMA_API_KEY is required by the pi/Ollama-Cloud runtime.
								// The secret is created by the hub backend when the agent is
								// created with an ollamaCloud.apiKey. When spec.ollamaCloud
								// is unset the reference is optional so the pod still starts.
								ollamaSecretName := agent.Name + "-ollama-key"
								if agent.Spec.OllamaCloud != nil && agent.Spec.OllamaCloud.APIKeySecretRef != nil {
									ollamaSecretName = agent.Spec.OllamaCloud.APIKeySecretRef.Name
								}
								envs = append(envs, corev1.EnvVar{
									Name: "OLLAMA_API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: ollamaSecretName},
											Key:                  "api-key",
											Optional:             ptr.To(agent.Spec.OllamaCloud == nil),
										},
									},
								})
								// OPENCODE_API_KEY mirrors OLLAMA_API_KEY for agents using the
								// OpenCode provider. Optional so pods start even without a key.
								opencodeSecretName := agent.Name + "-opencode-key"
								if agent.Spec.OpenCode != nil && agent.Spec.OpenCode.APIKeySecretRef != nil {
									opencodeSecretName = agent.Spec.OpenCode.APIKeySecretRef.Name
								}
								envs = append(envs, corev1.EnvVar{
									Name: "OPENCODE_API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: opencodeSecretName},
											Key:                  "api-key",
											Optional:             ptr.To(agent.Spec.OpenCode == nil),
										},
									},
								})
								// ALIBABA_CLOUD_API_KEY mirrors OLLAMA_API_KEY for agents using
								// the Alibaba Token Plan provider. Optional so pods start even without a key.
								alibabaSecretName := agent.Name + "-alibaba-key"
								if agent.Spec.AlibabaCloud != nil && agent.Spec.AlibabaCloud.APIKeySecretRef != nil {
									alibabaSecretName = agent.Spec.AlibabaCloud.APIKeySecretRef.Name
								}
								envs = append(envs, corev1.EnvVar{
									Name: "ALIBABA_CLOUD_API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: alibabaSecretName},
											Key:                  "api-key",
											Optional:             ptr.To(agent.Spec.AlibabaCloud == nil),
										},
									},
								})
								// CUSTOM_PROVIDER_API_KEY is used by agents with a
								// custom LLM provider (non-standard endpoints). It is
								// optional so pods start even without a key.
								customProviderSecretName := agent.Name + "-custom-provider-key"
								if agent.Spec.CustomProvider != nil && agent.Spec.CustomProvider.APIKeySecretRef != nil {
									customProviderSecretName = agent.Spec.CustomProvider.APIKeySecretRef.Name
								}
								envs = append(envs, corev1.EnvVar{
									Name: "CUSTOM_PROVIDER_API_KEY",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: customProviderSecretName},
											Key:                  "api-key",
											Optional:             ptr.To(agent.Spec.CustomProvider == nil),
										},
									},
								})
								if agent.Spec.LLM.MaxTurns > 0 {
									envs = append(envs, corev1.EnvVar{
										Name:  "OLLAMA_CLOUD_MAX_TURNS",
										Value: fmt.Sprintf("%d", agent.Spec.LLM.MaxTurns),
									})
								}
								// Translate spec.enabledTools into AGENT_TOOLS.
								// claude-cli tool kinds in the AgentImage catalog are
								// ignored — the runtime no longer supports them.
								kindByName := make(map[string]ainselv1alpha1.AgentImageToolKind, len(img.Spec.Tools))
								for _, t := range img.Spec.Tools {
									kindByName[t.Name] = t.Kind
								}
								var containerTools []string
								for _, name := range agent.Spec.EnabledTools {
									if kindByName[name] == ainselv1alpha1.AgentImageToolKindContainer {
										containerTools = append(containerTools, name)
									}
									// shell and claude-cli kinds are silently dropped
								}
								envs = append(envs,
									corev1.EnvVar{Name: "AGENT_TOOLS", Value: strings.Join(containerTools, ",")},
								)
								// Inject image env vars as explicit entries (not envFrom)
								// so they are eligible for Kubernetes $(VAR) substitution
								// in MCP_SERVER_TOKENS below. Values are sourced from the
								// operator-managed <agent>-image-env Secret built by
								// reconcileImageEnvSecret.
								imageEnvNames := make(map[string]bool, len(img.Spec.Env))
								for _, e := range img.Spec.Env {
									envs = append(envs, corev1.EnvVar{
										Name: e.Name,
										ValueFrom: &corev1.EnvVarSource{
											SecretKeyRef: &corev1.SecretKeySelector{
												LocalObjectReference: corev1.LocalObjectReference{Name: agentName + "-image-env"},
												Key:                  e.Name,
											},
										},
									})
									imageEnvNames[e.Name] = true
								}
								// Inject the resolved MCP server URLs. Empty when
								// spec.enabledMCPs is empty or all referenced
								// Services are missing.
								// Also append any MCP servers configured on the AgentImage.
								for _, s := range img.Spec.MCPServers {
									mcpEntries = append(mcpEntries, fmt.Sprintf("%s=%s", s.Name, s.URL))
								}
								// Append MCP entries for sidecar containers that declare an MCP path.
								for _, sc := range img.Spec.Sidecars {
									if sc.MCPPath == "" {
										continue
									}
									port := sc.Port
									if port == 0 {
										port = 8080
									}
									mcpEntries = append(mcpEntries, fmt.Sprintf("%s=http://localhost:%d%s", sc.Name, port, sc.MCPPath))
								}
								// Inject the chat-mcp sidecar entry unless the AgentImage
								// already defines one or CHAT_MCP_IMAGE is unset.
								chatImage := resolveChatMCPImage()
								if chatImage != "" {
									chatDefined := false
									for _, sc := range img.Spec.Sidecars {
										if sc.Name == "chat" && sc.MCPPath != "" {
											chatDefined = true
											break
										}
									}
									if !chatDefined {
										mcpEntries = append(mcpEntries, "chat=http://localhost:8081/mcp")
									}
								}
								envs = append(envs, corev1.EnvVar{
									Name:  "MCP_SERVERS",
									Value: mcpservers.EnvValue(mcpEntries),
								})
								// Build MCP_SERVER_TOKENS using $(VAR) references that
								// Kubernetes substitutes from the image env entries
								// added above. Servers whose tokenFromEnv is missing
								// from the image env are skipped; the controller logs
								// them so a future Degraded condition can surface the
								// misconfiguration to the user.
								tokenValue, me := mcpservers.TokenEnvValue(img.Spec.MCPServers, imageEnvNames)
								if len(me) > 0 {
									missingEnv = append(missingEnv, me...)
									var descs []string
									for _, m := range me {
										descs = append(descs, fmt.Sprintf("%s -> %s", m.ServerName, m.EnvVarName))
									}
									logf.FromContext(ctx).Info(
										"MCP server token references env var that is not defined on the AgentImage; skipping",
										"agent", agent.Name,
										"missing", strings.Join(descs, ", "),
									)
								}
								if tokenValue != "" {
									envs = append(envs, corev1.EnvVar{
										Name:  "MCP_SERVER_TOKENS",
										Value: tokenValue,
									})
								}
								return envs
							}(),
							VolumeMounts: agentMounts,
						},
					},
					Volumes: deployVolumes,
				},
			},
		}
		// Inject sidecar containers declared on the AgentImage. Each sidecar
		// runs alongside the agent runtime in the same pod, sharing the
		// network namespace (accessible via localhost). Sidecar env vars
		// are sourced from the same <agent>-image-env Secret as the main
		// container.
		for _, sc := range img.Spec.Sidecars {
			scPort := sc.Port
			if scPort == 0 {
				scPort = 8080
			}
			sidecarEnv := []corev1.EnvVar{
				{Name: "AGENT_NAME", Value: agent.Name},
				{Name: "PORT", Value: fmt.Sprintf("%d", scPort)},
			}
			// Automatically inject platform-level env vars (HUB_URL,
			// HUB_INTERNAL_VALIDATE_SECRET) into MCP sidecars so they can
			// communicate with the hub without per-AgentImage manual config.
			// Only inject vars that are not already provided by sc.Env to
			// avoid duplicates — user-provided values take precedence.
			if sc.MCPPath != "" {
				userProvided := make(map[string]bool, len(sc.Env))
				for _, e := range sc.Env {
					userProvided[e.Name] = true
				}
				for _, pe := range platformSidecarEnv() {
					if !userProvided[pe.Name] {
						sidecarEnv = append(sidecarEnv, pe)
					}
				}
			}
			for _, e := range sc.Env {
				sidecarEnv = append(sidecarEnv, corev1.EnvVar{
					Name: e.Name,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: agentName + "-image-env"},
							Key:                  e.Name,
						},
					},
				})
			}
			deploy.Spec.Template.Spec.Containers = append(
				deploy.Spec.Template.Spec.Containers,
				corev1.Container{
					Name:  sc.Name,
					Image: sc.Image,
					Env:   sidecarEnv,
				},
			)
		}
		// Inject the chat-mcp sidecar so every agent can reply to chat
		// messages without per-AgentImage configuration, but only when
		// CHAT_MCP_IMAGE is configured on the operator Deployment. Skip
		// if the AgentImage already defines a "chat" sidecar (backward compat).
		chatImage := resolveChatMCPImage()
		chatDefined := false
		for _, sc := range img.Spec.Sidecars {
			if sc.Name == "chat" {
				chatDefined = true
				break
			}
		}
		if chatImage != "" && !chatDefined {
			chatSidecarEnv := []corev1.EnvVar{
				{Name: "AGENT_NAME", Value: agent.Name},
				{Name: "PORT", Value: "8081"},
			}
			chatSidecarEnv = append(chatSidecarEnv, platformSidecarEnv()...)
			deploy.Spec.Template.Spec.Containers = append(
				deploy.Spec.Template.Spec.Containers,
				corev1.Container{
					Name:  "chat",
					Image: chatImage,
					Env:   chatSidecarEnv,
				},
			)
		}
		// Compute a stable hash of all referenced secrets and annotate the
		// pod template. When a secret changes, the hash changes, which causes
		// Kubernetes to perform a rolling restart automatically.
		var hashRefs []corev1.SecretKeySelector
		if agent.Spec.OllamaCloud != nil && agent.Spec.OllamaCloud.APIKeySecretRef != nil {
			hashRefs = append(hashRefs, *agent.Spec.OllamaCloud.APIKeySecretRef)
		} else {
			hashRefs = append(hashRefs, corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: agent.Name + "-ollama-key"},
				Key:                  "api-key",
			})
		}
		if agent.Spec.OpenCode != nil && agent.Spec.OpenCode.APIKeySecretRef != nil {
			hashRefs = append(hashRefs, *agent.Spec.OpenCode.APIKeySecretRef)
		} else {
			hashRefs = append(hashRefs, corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: agent.Name + "-opencode-key"},
				Key:                  "api-key",
			})
		}
		if agent.Spec.AlibabaCloud != nil && agent.Spec.AlibabaCloud.APIKeySecretRef != nil {
			hashRefs = append(hashRefs, *agent.Spec.AlibabaCloud.APIKeySecretRef)
		} else {
			hashRefs = append(hashRefs, corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: agent.Name + "-alibaba-key"},
				Key:                  "api-key",
			})
		}
		if agent.Spec.CustomProvider != nil && agent.Spec.CustomProvider.APIKeySecretRef != nil {
			hashRefs = append(hashRefs, *agent.Spec.CustomProvider.APIKeySecretRef)
		} else {
			hashRefs = append(hashRefs, corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: agent.Name + "-custom-provider-key"},
				Key:                  "api-key",
				Optional:             ptr.To(true),
			})
		}
		hash, err := r.computeSecretHash(ctx, agent.Namespace, hashRefs, agentName+"-image-env")
		if err != nil {
			return fmt.Errorf("computing secret hash: %w", err)
		}
		if deploy.Spec.Template.Annotations == nil {
			deploy.Spec.Template.Annotations = map[string]string{}
		}
		deploy.Spec.Template.Annotations[secretHashAnnotation] = hash

		// Compute a stable hash of the persona ConfigMap data and annotate
		// the pod template. When the hub updates the persona ConfigMap, the
		// hash changes, which causes Kubernetes to perform a rolling restart
		// automatically so the agent picks up the new persona.
		personaHash, err := r.computePersonaHash(ctx, agent.Namespace, personaCMName)
		if err != nil {
			return fmt.Errorf("computing persona hash: %w", err)
		}
		deploy.Spec.Template.Annotations[personaHashAnnotation] = personaHash

		// Compute a stable hash of the shared skills ConfigMap data and
		// annotate the pod template. When the hub updates a skill, the hash
		// changes, which causes Kubernetes to perform a rolling restart
		// automatically so the agent picks up the new skill content.
		if len(img.Spec.EnabledSkills) > 0 {
			skillHash, err := r.computeSkillsHash(ctx, agent.Namespace)
			if err != nil {
				return fmt.Errorf("computing skill hash: %w", err)
			}
			deploy.Spec.Template.Annotations[skillHashAnnotation] = skillHash
		}

		// Resolve the image digest from the container registry and annotate
		// the pod template. When a mutable tag (e.g. :dev) is rebuilt, the
		// digest changes, which causes Kubernetes to perform a rolling
		// restart so agents pick up the new image automatically.
		if digest, err := resolveImageDigest(ctx, podImage); err != nil {
			logf.FromContext(ctx).Info("could not resolve image digest; skipping restart annotation",
				"image", podImage, "error", err)
		} else {
			deploy.Spec.Template.Annotations[imageDigestAnnotation] = digest
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return deploy, missingEnv, nil
}

// reconcileMCPTokenEnvCondition sets or clears the Degraded condition on the
// Agent status and emits Warning Events when MCP servers reference env vars
// that are not defined on the AgentImage. When missingEnv is empty the
// condition is cleared (set to False with reason AsExpected).
func (r *AgentReconciler) reconcileMCPTokenEnvCondition(ctx context.Context, agent *ainselv1alpha1.Agent, missingEnv []mcpservers.MissingEnvEntry) {
	if len(missingEnv) > 0 {
		var parts []string
		for _, m := range missingEnv {
			parts = append(parts, fmt.Sprintf("MCP server %q references env var %q which is not defined on the AgentImage", m.ServerName, m.EnvVarName))
			if r.Recorder != nil {
				r.Recorder.Eventf(agent, corev1.EventTypeWarning, "MCPTokenEnvMissing",
					"MCP server %q references env var %q which is not defined on the AgentImage; the bearer header will be omitted for this MCP",
					m.ServerName, m.EnvVarName)
			}
		}
		apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
			Type:               ainselv1alpha1.AgentConditionDegraded,
			Status:             metav1.ConditionTrue,
			Reason:             "MCPTokenEnvMissing",
			Message:            strings.Join(parts, "; "),
			LastTransitionTime: metav1.Now(),
		})
	} else {
		// Clear the Degraded condition if it was previously set.
		if cond := apimeta.FindStatusCondition(agent.Status.Conditions, ainselv1alpha1.AgentConditionDegraded); cond != nil && cond.Status == metav1.ConditionTrue {
			apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               ainselv1alpha1.AgentConditionDegraded,
				Status:             metav1.ConditionFalse,
				Reason:             "AsExpected",
				Message:            "All MCP server token env vars are defined",
				LastTransitionTime: metav1.Now(),
			})
		}
	}
}

func (r *AgentReconciler) reconcileImageEnvSecret(ctx context.Context, agent *ainselv1alpha1.Agent, agentName string, img *ainselv1alpha1.AgentImage) error {
	secretName := agentName + "-image-env"
	if len(img.Spec.Env) == 0 {
		// Clean up any previously-created secret when the image has no env vars.
		// Only delete if the secret is owned by this Agent to avoid removing
		// user-managed secrets.
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: agent.Namespace}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("getting image env secret for cleanup: %w", err)
		}
		if metav1.IsControlledBy(secret, agent) {
			if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting unused image env secret: %w", err)
			}
		}
		return nil
	}

	secretData := make(map[string][]byte, len(img.Spec.Env))
	for _, e := range img.Spec.Env {
		secretData[e.Name] = []byte(e.Value)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: agent.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(agent, secret, r.Scheme); err != nil {
			return err
		}
		secret.Data = secretData
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconciling image env secret: %w", err)
	}
	return nil
}

// reconcilePiModelsConfigMap creates a ConfigMap with a Pi CLI models.json
// that defines the provider for the agent's chosen LLM backend. The API key is
// resolved at runtime by Pi from the environment variable named in the provider
// config (injected by the operator).
func (r *AgentReconciler) reconcilePiModelsConfigMap(ctx context.Context, agent *ainselv1alpha1.Agent, agentName string) error {
	pp := resolvePiProvider(agent)
	// `ainsel` is our own sibling-of-`providers` namespace at the root of
	// models.json. Pi ignores unknown root keys; the pi-ainsel-llm-config
	// extension reads this block to inject sampling params into each
	// provider request. Keeps all LLM config in one ConfigMap and out of
	// the pod env.
	var ainselBlock string
	if agent.Spec.LLM.Temperature != nil {
		ainselBlock = fmt.Sprintf(`,
    "ainsel": {
        "temperature": %s
    }`, strconv.FormatFloat(*agent.Spec.LLM.Temperature, 'f', -1, 64))
	}
	// Pi resolves `apiKey` values as env-var references only when prefixed
	// with `$` (or `${...}`). A bare identifier is treated as the literal
	// API key and the request fails with 401. Accept either form in
	// pp.APIKeyEnv ("OLLAMA_API_KEY" or "$OLLAMA_API_KEY") and always emit
	// the $-prefixed form pi expects.
	apiKeyRef := pp.APIKeyEnv
	if !strings.HasPrefix(apiKeyRef, "$") {
		apiKeyRef = "$" + apiKeyRef
	}
	// Build optional JSON fragments for provider-level and model-level
	// compat fields. These are generic: any provider can carry them.
	providerCompatBlock := ""
	if pp.ProviderCompat != "" {
		providerCompatBlock = ",\n            " + pp.ProviderCompat
	}
	modelCompatBlock := ""
	if pp.ModelCompat != "" {
		modelCompatBlock = ",\n                    " + pp.ModelCompat
	}
	thinkingLevelMapBlock := ""
	if pp.ThinkingLevelMap != "" {
		thinkingLevelMapBlock = ",\n                    " + pp.ThinkingLevelMap
	}
	modelsJSON := fmt.Sprintf(`{
    "providers": {
        %q: {
            "api": "openai-completions",
            "apiKey": %q,
            "baseUrl": %q%s,
            "models": [
                {
                    "id": %q,
                    "contextWindow": 202752,
                    "input": ["text"],
                    "reasoning": true%s%s%s
                }
            ]
        }
    }%s
}`, pp.Name, apiKeyRef, pp.BaseURL, providerCompatBlock, agent.Spec.LLM.Model, pp.Compat, thinkingLevelMapBlock, modelCompatBlock, ainselBlock)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName + "-pi-models",
			Namespace: agent.Namespace,
		},
		Data: map[string]string{
			"models.json": modelsJSON,
		},
	}

	if err := controllerutil.SetControllerReference(agent, cm, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, cm); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		existing := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cm.Name, Namespace: cm.Namespace}}
		if err := r.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			return err
		}
		existing.Data = cm.Data
		if err := r.Update(ctx, existing); err != nil {
			return err
		}
	}
	return nil
}

func (r *AgentReconciler) reconcileMetricsService(ctx context.Context, agent *ainselv1alpha1.Agent, agentName string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName + "-metrics",
			Namespace: agent.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(agent, svc, r.Scheme); err != nil {
			return err
		}
		svc.Spec = corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name":       agentName,
				"app.kubernetes.io/managed-by": "agent-operator",
				"ainsel.dev/agent":             agent.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt32(9090),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		}
		return nil
	})
	return err
}

func (r *AgentReconciler) updateStatus(ctx context.Context, agent *ainselv1alpha1.Agent, deploy *appsv1.Deployment) error {
	// DeploymentReady condition
	if deploy != nil {
		desired := desiredReplicas(agent)
		if deploy.Status.ReadyReplicas >= desired {
			apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               ainselv1alpha1.AgentConditionDeploymentReady,
				Status:             metav1.ConditionTrue,
				Reason:             "MinimumReplicasAvailable",
				Message:            fmt.Sprintf("Deployment has %d/%d ready replicas", deploy.Status.ReadyReplicas, desired),
				LastTransitionTime: metav1.Now(),
			})
		} else {
			apimeta.SetStatusCondition(&agent.Status.Conditions, metav1.Condition{
				Type:               ainselv1alpha1.AgentConditionDeploymentReady,
				Status:             metav1.ConditionFalse,
				Reason:             "ReplicasNotReady",
				Message:            fmt.Sprintf("Deployment has %d/%d ready replicas", deploy.Status.ReadyReplicas, desired),
				LastTransitionTime: metav1.Now(),
			})
		}
		agent.Status.Replicas = deploy.Status.ReadyReplicas
	}

	// Compute aggregate Ready condition: True only when all sub-conditions are True.
	subConditions := []string{
		ainselv1alpha1.AgentConditionDeploymentReady,
		ainselv1alpha1.AgentConditionMCPDiscoveryComplete,
		ainselv1alpha1.AgentConditionPersonaConfigMapReady,
		ainselv1alpha1.AgentConditionImageEnvSecretReady,
	}
	allReady := true
	var notReady []string
	for _, ct := range subConditions {
		cond := apimeta.FindStatusCondition(agent.Status.Conditions, ct)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			allReady = false
			notReady = append(notReady, ct)
		}
	}

	readyCondition := metav1.Condition{
		Type:               ainselv1alpha1.AgentConditionReady,
		LastTransitionTime: metav1.Now(),
	}
	if allReady {
		readyCondition.Status = metav1.ConditionTrue
		readyCondition.Reason = "AllReady"
		readyCondition.Message = "All resources reconciled successfully"
	} else {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "SubConditionsNotReady"
		readyCondition.Message = fmt.Sprintf("Waiting on: %s", strings.Join(notReady, ", "))
	}
	apimeta.SetStatusCondition(&agent.Status.Conditions, readyCondition)

	agent.Status.ObservedGeneration = agent.Generation

	return r.Status().Update(ctx, agent)
}

// reconcileServiceMonitor manages a prometheus-operator ServiceMonitor for the
// agent's metrics Service, so Prometheus picks up the scrape target automatically.
//
// If the prometheus-operator CRDs are not installed (the ServiceMonitor CRD is missing)
// the call is logged and skipped — installation of prometheus-operator is a
// cluster-level prerequisite, not an agent-level failure.
func (r *AgentReconciler) reconcileServiceMonitor(ctx context.Context, agent *ainselv1alpha1.Agent, agentName string) error {
	log := logf.FromContext(ctx)

	sm := &unstructured.Unstructured{}
	sm.SetAPIVersion(monitoringAPIVersion)
	sm.SetKind(serviceMonitorKind)
	sm.SetName(agentName + "-metrics")
	sm.SetNamespace(agent.Namespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sm, func() error {
		if err := controllerutil.SetControllerReference(agent, sm, r.Scheme); err != nil {
			return err
		}
		sm.Object["spec"] = map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app.kubernetes.io/name":       agentName,
					"app.kubernetes.io/managed-by": "agent-operator",
					"ainsel.dev/agent":             agent.Name,
				},
			},
			"endpoints": []interface{}{
				map[string]interface{}{
					"port":     "metrics",
					"path":     "/metrics",
					"interval": "30s",
				},
			},
		}
		return nil
	})
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			log.Info("prometheus-operator ServiceMonitor CRD not installed; skipping scrape registration",
				"agent", agent.Name)
			return nil
		}
		return fmt.Errorf("reconcile ServiceMonitor: %w", err)
	}
	return nil
}

// computeSecretHash builds a stable hash of the data in the referenced secrets.
// If a secret does not exist, a stable sentinel value is used so that when the
// secret is later created the hash changes and triggers a restart.
func (r *AgentReconciler) computeSecretHash(ctx context.Context, namespace string, refs []corev1.SecretKeySelector, envSecretName string) (string, error) {
	h := sha256.New()

	// Sort refs by (Name, Key) so the hash is deterministic regardless of order.
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Name != refs[j].Name {
			return refs[i].Name < refs[j].Name
		}
		return refs[i].Key < refs[j].Key
	})

	for _, ref := range refs {
		secret := &corev1.Secret{}
		key := types.NamespacedName{Namespace: namespace, Name: ref.Name}
		err := r.Get(ctx, key, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Stable sentinel for missing secret: "__missing__:<name>:<key>"
				_, _ = fmt.Fprintf(h, "__missing__:%s:%s\n", ref.Name, ref.Key)
				continue
			}
			return "", err
		}

		val, ok := secret.Data[ref.Key]
		if !ok {
			// Stable sentinel for missing key inside an existing secret.
			_, _ = fmt.Fprintf(h, "__missing_key__:%s:%s\n", ref.Name, ref.Key)
			continue
		}

		_, _ = fmt.Fprintf(h, "%s:%s:%x\n", ref.Name, ref.Key, val)
	}

	// Hash the image env secret (all keys).
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: namespace, Name: envSecretName}
	err := r.Get(ctx, key, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = fmt.Fprintf(h, "__missing__:%s:*\n", envSecretName)
		} else {
			return "", err
		}
	} else {
		keys := make([]string, 0, len(secret.Data))
		for k := range secret.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			_, _ = fmt.Fprintf(h, "%s:%s:%x\n", envSecretName, k, secret.Data[k])
		}
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// computePersonaHash builds a stable hash of the Data map in the persona
// ConfigMap. If the ConfigMap does not exist, a stable sentinel value is used
// so that when it is later created the hash changes and triggers a restart.
// This mirrors the computeSecretHash behaviour for missing secrets.
func (r *AgentReconciler) computePersonaHash(ctx context.Context, namespace, personaCMName string) (string, error) {
	h := sha256.New()

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: personaCMName}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = fmt.Fprintf(h, "__missing__:%s\n", personaCMName)
			return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
		}
		return "", err
	}

	// Sort keys for a deterministic hash regardless of map iteration order.
	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s:%s\n", k, cm.Data[k])
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// computeSkillsHash builds a stable hash of the Data map in the shared skills
// ConfigMap. If the ConfigMap does not exist, a stable sentinel value is used
// so that when it is later created the hash changes and triggers a restart.
func (r *AgentReconciler) computeSkillsHash(ctx context.Context, namespace string) (string, error) {
	h := sha256.New()

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: sharedskills.ConfigMapName}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, _ = fmt.Fprintf(h, "__missing__:%s\n", sharedskills.ConfigMapName)
			return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
		}
		return "", err
	}

	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s:%s\n", k, cm.Data[k])
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}


// findAffectedAgents maps a Secret event to the Agent(s) that reference it.
func (r *AgentReconciler) findAffectedAgents(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	log := logf.FromContext(ctx)

	agentList := &ainselv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(secret.Namespace)); err != nil {
		log.Error(err, "failed to list agents for secret watcher")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		if referencesSecret(agent, secret.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: agent.Namespace,
					Name:      agent.Name,
				},
			})
		}
	}
	return requests
}

// findAffectedAgentsFromConfigMap maps a ConfigMap event to the Agent(s) whose
// spec.persona.id references it or whose AgentImage has enabled skills. Only
// ConfigMaps with the "persona-" prefix or the shared skills ConfigMap are
// considered; all other ConfigMaps are silently ignored to keep overhead low.
func (r *AgentReconciler) findAffectedAgentsFromConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm := obj.(*corev1.ConfigMap)
	log := logf.FromContext(ctx)

	// Fast path: persona ConfigMaps.
	if strings.HasPrefix(cm.Name, sharedpersonas.ConfigMapNamePrefix) {
		agentList := &ainselv1alpha1.AgentList{}
		if err := r.List(ctx, agentList, client.InNamespace(cm.Namespace)); err != nil {
			log.Error(err, "failed to list agents for persona ConfigMap watcher")
			return nil
		}
		var requests []reconcile.Request
		for _, agent := range agentList.Items {
			if sharedpersonas.PersonaConfigMapName(agent.Spec.Persona.ID) == cm.Name {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: agent.Namespace,
						Name:      agent.Name,
					},
				})
			}
		}
		return requests
	}

	// Fast path: shared skills ConfigMap.
	if cm.Name == sharedskills.ConfigMapName {
		// Find all AgentImages with enabled skills so we can match them
		// against agents.
		imgList := &ainselv1alpha1.AgentImageList{}
		if err := r.List(ctx, imgList, client.InNamespace(cm.Namespace)); err != nil {
			log.Error(err, "failed to list agent images for skills ConfigMap watcher")
			return nil
		}
		imageNamesWithSkills := make(map[string]struct{}, len(imgList.Items))
		for _, img := range imgList.Items {
			if len(img.Spec.EnabledSkills) > 0 {
				imageNamesWithSkills[img.Name] = struct{}{}
			}
		}

		agentList := &ainselv1alpha1.AgentList{}
		if err := r.List(ctx, agentList, client.InNamespace(cm.Namespace)); err != nil {
			log.Error(err, "failed to list agents for skills ConfigMap watcher")
			return nil
		}
		var requests []reconcile.Request
		for _, agent := range agentList.Items {
			if _, ok := imageNamesWithSkills[agent.Spec.ImageRef.Name]; ok {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: agent.Namespace,
						Name:      agent.Name,
					},
				})
			}
		}
		return requests
	}

	return nil
}

// agentImageToAgents maps an AgentImage change to all Agent CRs that reference it.
func (r *AgentReconciler) agentImageToAgents(ctx context.Context, obj client.Object) []reconcile.Request {
	img, ok := obj.(*ainselv1alpha1.AgentImage)
	if !ok {
		return nil
	}
	log := logf.FromContext(ctx)

	agentList := &ainselv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(img.Namespace)); err != nil {
		log.Error(err, "failed to list agents for agentimage watcher")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		if agent.Spec.ImageRef.Name == img.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: agent.Namespace,
					Name:      agent.Name,
				},
			})
		}
	}
	return requests
}

func referencesSecret(agent ainselv1alpha1.Agent, secretName string) bool {
	if agent.Spec.OllamaCloud != nil && agent.Spec.OllamaCloud.APIKeySecretRef != nil && agent.Spec.OllamaCloud.APIKeySecretRef.Name == secretName {
		return true
	}
	if agent.Spec.OpenCode != nil && agent.Spec.OpenCode.APIKeySecretRef != nil && agent.Spec.OpenCode.APIKeySecretRef.Name == secretName {
		return true
	}
	if agent.Spec.AlibabaCloud != nil && agent.Spec.AlibabaCloud.APIKeySecretRef != nil && agent.Spec.AlibabaCloud.APIKeySecretRef.Name == secretName {
		return true
	}
	if agent.Spec.CustomProvider != nil && agent.Spec.CustomProvider.APIKeySecretRef != nil && agent.Spec.CustomProvider.APIKeySecretRef.Name == secretName {
		return true
	}
	// The image-env secret is always operator-managed with a fixed name.
	if "agent-"+agent.Name+"-image-env" == secretName {
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ainselv1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAffectedAgents),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&ainselv1alpha1.AgentImage{},
			handler.EnqueueRequestsFromMapFunc(r.agentImageToAgents),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findAffectedAgentsFromConfigMap),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Named("agent").
		Complete(r)
}
