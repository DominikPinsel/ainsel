package controller

import (
	"context"
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ainselv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"

)

func findEnvVar(envs []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}

var _ = Describe("Agent Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-agent"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		const testImageName = "test-image"
		const testImageURL = "dpinsel/agent:latest"

		BeforeEach(func() {
			By("creating the AgentImage fixture")
			img := &ainselv1alpha1.AgentImage{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)
			if err != nil && errors.IsNotFound(err) {
				img = &ainselv1alpha1.AgentImage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testImageName,
						Namespace: "default",
					},
					Spec: ainselv1alpha1.AgentImageSpec{
						DisplayName: "Test Image",
						ImageURL:    testImageURL,
						Tools: []ainselv1alpha1.AgentImageTool{
							{Name: "git", Kind: ainselv1alpha1.AgentImageToolKindContainer},
							{Name: "bash", Kind: ainselv1alpha1.AgentImageToolKindShell},
						},
					},
				}
				Expect(k8sClient.Create(ctx, img)).To(Succeed())
				// Set status.phase = Ready via status subresource
				img.Status.Phase = ainselv1alpha1.AgentImagePhaseReady
				Expect(k8sClient.Status().Update(ctx, img)).To(Succeed())
			}

			By("creating the custom resource for the Kind Agent")
			agent := &ainselv1alpha1.Agent{}
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			if err != nil && errors.IsNotFound(err) {
				resource := &ainselv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: ainselv1alpha1.AgentSpec{
						DisplayName: "Test Agent",
						ImageRef: ainselv1alpha1.AgentImageRef{
							Name: testImageName,
						},
						Runtime: ainselv1alpha1.AgentRuntime{},
						LLM: ainselv1alpha1.AgentLLM{
							Model: "glm-5.1:cloud",
						},
						Persona: ainselv1alpha1.AgentPersona{
							ID: "01hxtestpersona00000000000",
						},
						EnabledTools: []string{"git", "bash"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &ainselv1alpha1.Agent{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Agent")
	Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			By("Cleanup the AgentImage fixture")
			img := &ainselv1alpha1.AgentImage{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)
			if err == nil {
				Expect(k8sClient.Delete(ctx, img)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			agentName := "agent-" + resourceName

			By("Verifying the operator does NOT create the legacy per-agent persona ConfigMap")
			// The hub now owns persona ConfigMaps (rendered as "persona-<id>").
			// The operator no longer renders <agent>-persona.
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName + "-persona",
				Namespace: "default",
			}, cm)
			Expect(errors.IsNotFound(err)).To(BeTrue(),
				"operator must not render the legacy <agent>-persona ConfigMap; the hub owns persona-<id>")

			By("Verifying the Deployment was created")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Name).To(Equal("agent"))

			By("Verifying the container image is sourced from the AgentImage CR")
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal(testImageURL))

			By("Verifying AGENT_TOOLS is set to the container-kind EnabledTools only")
			envs := deploy.Spec.Template.Spec.Containers[0].Env
			agentToolsEnv := findEnvVar(envs, "AGENT_TOOLS")
			Expect(agentToolsEnv).NotTo(BeNil())
			// Only container-kind tools are surfaced; shell kinds (e.g. "bash")
			// are dropped — they ran inside the runtime via Bash patterns in the
			// claude-cli days, which is no longer relevant now the agent is pi-only.
			Expect(agentToolsEnv.Value).To(Equal("git"))
			By("Verifying CLAUDE_CLI_ALLOWED_TOOLS is no longer emitted")
			Expect(findEnvVar(envs, "CLAUDE_CLI_ALLOWED_TOOLS")).To(BeNil())
			By("Verifying AGENT_PROVIDER is no longer emitted")
			Expect(findEnvVar(envs, "AGENT_PROVIDER")).To(BeNil())

			By("Verifying the metrics Service was created")
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName + "-metrics",
				Namespace: "default",
			}, svc)).To(Succeed())

			By("Verifying the Agent status has sub-conditions set")
			updatedAgent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedAgent)).To(Succeed())
			Expect(updatedAgent.Status.Conditions).NotTo(BeEmpty())

			findCondition := func(conditions []metav1.Condition, condType string) *metav1.Condition {
				for i := range conditions {
					if conditions[i].Type == condType {
						return &conditions[i]
					}
				}
				return nil
			}

			By("Verifying DeploymentReady is False (no ready replicas in envtest)")
			deployCond := findCondition(updatedAgent.Status.Conditions, ainselv1alpha1.AgentConditionDeploymentReady)
			Expect(deployCond).NotTo(BeNil())
			Expect(deployCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(deployCond.Reason).To(Equal("ReplicasNotReady"))

			By("Verifying MCPDiscoveryComplete is True")
			mcpCond := findCondition(updatedAgent.Status.Conditions, ainselv1alpha1.AgentConditionMCPDiscoveryComplete)
			Expect(mcpCond).NotTo(BeNil())
			Expect(mcpCond.Status).To(Equal(metav1.ConditionTrue))

			By("Verifying PersonaConfigMapReady is False (no persona CM in this test)")
			personaCond := findCondition(updatedAgent.Status.Conditions, ainselv1alpha1.AgentConditionPersonaConfigMapReady)
			Expect(personaCond).NotTo(BeNil())
			Expect(personaCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(personaCond.Reason).To(Equal("NotFound"))

			By("Verifying ImageEnvSecretReady is True")
			secretCond := findCondition(updatedAgent.Status.Conditions, ainselv1alpha1.AgentConditionImageEnvSecretReady)
			Expect(secretCond).NotTo(BeNil())
			Expect(secretCond.Status).To(Equal(metav1.ConditionTrue))

			By("Verifying aggregate Ready is False (DeploymentReady and PersonaConfigMapReady not ready)")
			readyCondition := findCondition(updatedAgent.Status.Conditions, ainselv1alpha1.AgentConditionReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("SubConditionsNotReady"))
			Expect(readyCondition.Message).To(ContainSubstring("DeploymentReady"))
			Expect(readyCondition.Message).To(ContainSubstring("PersonaConfigMapReady"))

			By("Creating the persona ConfigMap and simulating Deployment readiness")
			pcm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persona-01hxtestpersona00000000000",
					Namespace: "default",
				},
				Data: map[string]string{"CLAUDE.md": "test persona"},
			}
			Expect(k8sClient.Create(ctx, pcm)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, pcm) })

			deploy = &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentName, Namespace: "default"}, deploy)).To(Succeed())
			deploy.Status.ReadyReplicas = 1
			deploy.Status.Replicas = 1
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			By("Reconciling again to pick up ready replicas")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Agent status has Ready=True condition")
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedAgent)).To(Succeed())
			readyCondition = findCondition(updatedAgent.Status.Conditions, ainselv1alpha1.AgentConditionReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("AllReady"))
		})

		It("should inject sidecar containers from AgentImage spec", func() {
			agentName := "agent-" + resourceName
			By("Updating the AgentImage to include a chat sidecar")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Sidecars = []ainselv1alpha1.AgentImageSidecar{
				{
					Name:    "chat",
					Image:   "dpinsel/ainsel-chat-mcp:main",
					Port:    8081,
					MCPPath: "/mcp",
				},
			}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			By("Triggering reconciliation")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment has two containers")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deploy.Spec.Template.Spec.Containers[0].Name).To(Equal("agent"))
			Expect(deploy.Spec.Template.Spec.Containers[1].Name).To(Equal("chat"))
			Expect(deploy.Spec.Template.Spec.Containers[1].Image).To(Equal("dpinsel/ainsel-chat-mcp:main"))

			By("Verifying AGENT_NAME is propagated to the sidecar")
			sidecarEnv := deploy.Spec.Template.Spec.Containers[1].Env
			sidecarAgentName := findEnvVar(sidecarEnv, "AGENT_NAME")
			Expect(sidecarAgentName).NotTo(BeNil())
			Expect(sidecarAgentName.Value).To(Equal(resourceName))

			By("Verifying PORT is set on the sidecar")
			sidecarPort := findEnvVar(sidecarEnv, "PORT")
			Expect(sidecarPort).NotTo(BeNil())
			Expect(sidecarPort.Value).To(Equal("8081"))

			By("Verifying MCP_SERVERS includes the sidecar localhost entry")
			mainEnv := deploy.Spec.Template.Spec.Containers[0].Env
			mcpServers := findEnvVar(mainEnv, "MCP_SERVERS")
			Expect(mcpServers).NotTo(BeNil())
			Expect(mcpServers.Value).To(ContainSubstring("chat=http://localhost:8081/mcp"))

			By("Verifying platform env vars (HUB_URL, HUB_INTERNAL_VALIDATE_SECRET) are injected into MCP sidecars")
			// These are injected by the operator from its own environment so
			// the chat-mcp sidecar can reach the hub without per-AgentImage
			// manual configuration. When the operator's env is unset the
			// vars are simply absent (not injected with empty values).
			if hubURL := findEnvVar(sidecarEnv, "HUB_URL"); hubURL != nil {
				Expect(hubURL.Value).To(Equal(resolveHubURL()))
			}
			if hubToken := findEnvVar(sidecarEnv, "HUB_INTERNAL_VALIDATE_SECRET"); hubToken != nil {
				Expect(hubToken.Value).To(Equal(resolveHubInternalToken()))
			}
			// Verify no HUB_URL is injected when the operator env is unset.
			if resolveHubURL() == "" {
				Expect(findEnvVar(sidecarEnv, "HUB_URL")).To(BeNil())
			}
			if resolveHubInternalToken() == "" {
				Expect(findEnvVar(sidecarEnv, "HUB_INTERNAL_VALIDATE_SECRET")).To(BeNil())
			}

			By("Cleaning up — removing sidecars from the image")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Sidecars = nil
			Expect(k8sClient.Update(ctx, img)).To(Succeed())
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should auto-inject the chat-mcp sidecar when CHAT_MCP_IMAGE is set", func() {
			agentName := "agent-" + resourceName
			By("Setting CHAT_MCP_IMAGE in the operator environment")
			originalChatImage := os.Getenv("CHAT_MCP_IMAGE")
			Expect(os.Setenv("CHAT_MCP_IMAGE", "dpinsel/ainsel-chat-mcp:auto")).To(Succeed())
			DeferCleanup(func() {
				if originalChatImage == "" {
					Expect(os.Unsetenv("CHAT_MCP_IMAGE")).To(Succeed())
				} else {
					Expect(os.Setenv("CHAT_MCP_IMAGE", originalChatImage)).To(Succeed())
				}
			})

			By("Triggering reconciliation")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment has two containers")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(2))
			Expect(deploy.Spec.Template.Spec.Containers[0].Name).To(Equal("agent"))
			Expect(deploy.Spec.Template.Spec.Containers[1].Name).To(Equal("chat"))
			Expect(deploy.Spec.Template.Spec.Containers[1].Image).To(Equal("dpinsel/ainsel-chat-mcp:auto"))

			By("Verifying MCP_SERVERS includes the chat sidecar entry")
			mainEnv := deploy.Spec.Template.Spec.Containers[0].Env
			mcpServers := findEnvVar(mainEnv, "MCP_SERVERS")
			Expect(mcpServers).NotTo(BeNil())
			Expect(mcpServers.Value).To(ContainSubstring("chat=http://localhost:8081/mcp"))
		})

		It("should reflect the Deployment's ReadyReplicas in Agent status.replicas", func() {
			By("Initial reconciliation creates the Deployment")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Status.replicas defaults to 0 while no pods are ready")
			// Reset deployment status in case a previous test left ReadyReplicas > 0.
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			deploy.Status.ReadyReplicas = 0
			deploy.Status.Replicas = 0
			deploy.Status.AvailableReplicas = 0
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			// Re-reconcile to pick up the zeroed status.
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updatedAgent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedAgent)).To(Succeed())
			Expect(updatedAgent.Status.Replicas).To(BeEquivalentTo(0))

			By("Simulating the Deployment becoming ready")
			deploy = &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			deploy.Status.Replicas = 1
			deploy.Status.ReadyReplicas = 1
			deploy.Status.AvailableReplicas = 1
			Expect(k8sClient.Status().Update(ctx, deploy)).To(Succeed())

			By("Reconciling again propagates ReadyReplicas to Agent status")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedAgent)).To(Succeed())
			Expect(updatedAgent.Status.Replicas).To(BeEquivalentTo(1))
		})

		It("should set Deployment replicas from spec.scaling.replicas", func() {
			By("Setting spec.scaling.replicas = 3 on the Agent")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.Scaling = &ainselv1alpha1.AgentScaling{
				Replicas: ptr.To[int32](3),
			}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())

			Expect(deploy.Spec.Replicas).NotTo(BeNil())
			Expect(*deploy.Spec.Replicas).To(BeEquivalentTo(3),
				"Deployment replicas must match spec.scaling.replicas")
		})

		It("should default to 1 replica when scaling is nil", func() {
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())

			Expect(deploy.Spec.Replicas).NotTo(BeNil())
			Expect(*deploy.Spec.Replicas).To(BeEquivalentTo(1),
				"Deployment replicas must default to 1 when spec.scaling is nil")
		})

		It("should inject MCP_SERVERS from spec.enabledMCPs when the Service exists", func() {
			By("Pre-creating an mcp-example-mcp Service in the agent's namespace")
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mcp-example-mcp",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Name: "http", Port: 8080}},
				},
			}
			Expect(k8sClient.Create(ctx, svc)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, svc)
			}()

			By("Adding example-mcp to spec.enabledMCPs")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.EnabledMCPs = []string{"example-mcp"}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying MCP_SERVERS has the resolved URL")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			mcpEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "MCP_SERVERS")
			Expect(mcpEnv).NotTo(BeNil())
			Expect(mcpEnv.Value).To(Equal("example-mcp=http://mcp-example-mcp.default.svc.cluster.local:8080/mcp"))
		})

		It("should not fail when an enabled MCP Service is missing and should set MCP_SERVERS to empty", func() {
			By("Adding a non-existent MCP name to spec.enabledMCPs")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.EnabledMCPs = []string{"ghost"}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying MCP_SERVERS is present but empty")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			mcpEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "MCP_SERVERS")
			Expect(mcpEnv).NotTo(BeNil())
			Expect(mcpEnv.Value).To(Equal(""))
		})

		It("should build MCP_SERVER_TOKENS from AgentImage tokenFromEnv entries", func() {
			By("Updating the AgentImage with an MCP server that has tokenFromEnv")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Env = []ainselv1alpha1.AgentImageEnvVar{
				{Name: "FORGEJO_PAT", Value: "secret-token-123", Secret: true},
			}
			img.Spec.MCPServers = []ainselv1alpha1.AgentImageMCPServer{
				{Name: "forgejo-mcp-server", URL: "http://forgejo-mcp.workloads.svc.cluster.local:8080/mcp", TokenFromEnv: "FORGEJO_PAT"},
			}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the image env Secret contains FORGEJO_PAT")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName + "-image-env",
				Namespace: "default",
			}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKeyWithValue("FORGEJO_PAT", []byte("secret-token-123")))

			By("Verifying the Deployment exposes FORGEJO_PAT via secretKeyRef")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			container := deploy.Spec.Template.Spec.Containers[0]
			forgejoPat := findEnvVar(container.Env, "FORGEJO_PAT")
			Expect(forgejoPat).NotTo(BeNil())
			Expect(forgejoPat.ValueFrom).NotTo(BeNil())
			Expect(forgejoPat.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(forgejoPat.ValueFrom.SecretKeyRef.Key).To(Equal("FORGEJO_PAT"))

			By("Verifying MCP_SERVER_TOKENS includes the forgejo-mcp-server entry with $(FORGEJO_PAT)")
			tokensEnv := findEnvVar(container.Env, "MCP_SERVER_TOKENS")
			Expect(tokensEnv).NotTo(BeNil())
			Expect(tokensEnv.Value).To(ContainSubstring("forgejo-mcp-server=$(FORGEJO_PAT)"))
		})

		It("should skip MCP servers whose tokenFromEnv references a missing env var", func() {
			By("Updating the AgentImage with an MCP server whose tokenFromEnv is not defined in env")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Env = nil
			img.Spec.MCPServers = []ainselv1alpha1.AgentImageMCPServer{
				{Name: "forgejo-mcp-server", URL: "http://x", TokenFromEnv: "MISSING_TOKEN"},
			}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying MCP_SERVER_TOKENS is not set (all entries skipped)")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			tokensEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "MCP_SERVER_TOKENS")
			Expect(tokensEnv).To(BeNil())
		})

		It("should embed an ainsel.temperature block in models.json when spec.llm.temperature is set", func() {
			By("Setting spec.llm.temperature to 0.3")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.LLM.Temperature = ptr.To[float64](0.3)
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the pi-models ConfigMap embeds ainsel.temperature")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName + "-pi-models",
				Namespace: "default",
			}, cm)).To(Succeed())
			modelsJSON := cm.Data["models.json"]
			Expect(modelsJSON).To(ContainSubstring(`"ainsel"`))
			Expect(modelsJSON).To(ContainSubstring(`"temperature": 0.3`))

			By("Verifying models.json is still valid JSON after the injection")
			var parsed struct {
				Providers map[string]any `json:"providers"`
				Ainsel    struct {
					Temperature float64 `json:"temperature"`
				} `json:"ainsel"`
			}
			Expect(json.Unmarshal([]byte(modelsJSON), &parsed)).To(Succeed())
			Expect(parsed.Ainsel.Temperature).To(Equal(0.3))
			Expect(parsed.Providers).NotTo(BeEmpty())

			By("Verifying no PI_TEMPERATURE env var leaks into the pod spec")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "PI_TEMPERATURE")).To(BeNil())
		})

		It("should omit the ainsel block from models.json when spec.llm.temperature is unset", func() {
			By("Reconciling without touching spec.llm.temperature")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the pi-models ConfigMap has no ainsel block")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName + "-pi-models",
				Namespace: "default",
			}, cm)).To(Succeed())
			Expect(cm.Data["models.json"]).NotTo(ContainSubstring(`"ainsel"`))

			By("Verifying models.json is valid JSON")
			var parsed map[string]any
			Expect(json.Unmarshal([]byte(cm.Data["models.json"]), &parsed)).To(Succeed())
		})

		It("should embed provider and model compat blocks in models.json for alibaba-cloud", func() {
			By("Setting the provider to alibaba-cloud")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.LLM.Provider = ainselv1alpha1.AgentLLMProviderAlibabaCloud
			agent.Spec.LLM.Model = "qwen3.8-max-preview"
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the pi-models ConfigMap contains compat fields")
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName + "-pi-models",
				Namespace: "default",
			}, cm)).To(Succeed())
			modelsJSON := cm.Data["models.json"]

			By("Verifying models.json is valid JSON")
			var parsed struct {
				Providers map[string]struct {
					API     string `json:"api"`
					BaseURL string `json:"baseUrl"`
					Compat  struct {
						SupportsDeveloperRole    bool   `json:"supportsDeveloperRole"`
						SupportsUsageInStreaming bool   `json:"supportsUsageInStreaming"`
						MaxTokensField           string `json:"maxTokensField"`
						SupportsReasoningEffort  bool   `json:"supportsReasoningEffort"`
					} `json:"compat"`
					Models []struct {
						ID               string             `json:"id"`
						Reasoning        bool               `json:"reasoning"`
						ThinkingLevelMap map[string]*string `json:"thinkingLevelMap"`
						Compat           struct {
							ThinkingFormat string `json:"thinkingFormat"`
						} `json:"compat"`
					} `json:"models"`
				} `json:"providers"`
			}
			Expect(json.Unmarshal([]byte(modelsJSON), &parsed)).To(Succeed())

			provider, ok := parsed.Providers["alibaba-cloud"]
			Expect(ok).To(BeTrue(), "expected alibaba-cloud provider in models.json")
			Expect(provider.BaseURL).To(ContainSubstring("aliyuncs.com"))

			By("Verifying provider-level compat")
			Expect(provider.Compat.SupportsDeveloperRole).To(BeFalse())
			Expect(provider.Compat.SupportsUsageInStreaming).To(BeFalse())
			Expect(provider.Compat.MaxTokensField).To(Equal("max_tokens"))
			Expect(provider.Compat.SupportsReasoningEffort).To(BeFalse())

			By("Verifying model-level compat and thinkingLevelMap")
			Expect(provider.Models).To(HaveLen(1))
			model := provider.Models[0]
			Expect(model.ID).To(Equal("qwen3.8-max-preview"))
			Expect(model.Reasoning).To(BeTrue())
			Expect(model.Compat.ThinkingFormat).To(Equal("qwen"))
			Expect(model.ThinkingLevelMap).To(HaveKey("medium"))
			Expect(*model.ThinkingLevelMap["medium"]).To(Equal("medium"))
			Expect(model.ThinkingLevelMap["off"]).To(BeNil())
		})

		It("should mount the hub-rendered persona ConfigMap named persona-<id>", func() {
			// The hub renders persona ConfigMaps as "persona-<id>" (see
			// services/hub/internal/personas/reconciler.go). The operator's
			// Deployment must mount that ConfigMap, not the legacy
			// <agent>-persona name the operator used to render itself.
			const personaID = "01hxtestpersona00000000000"

			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.Persona = ainselv1alpha1.AgentPersona{ID: personaID}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())

			var personaVol *corev1.Volume
			for i := range deploy.Spec.Template.Spec.Volumes {
				if deploy.Spec.Template.Spec.Volumes[i].Name == "persona" {
					personaVol = &deploy.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(personaVol).NotTo(BeNil(), "persona volume must be present on the agent Deployment")
			Expect(personaVol.ConfigMap).NotTo(BeNil(), "persona volume must be backed by a ConfigMap")
			Expect(personaVol.ConfigMap.Name).To(Equal("persona-"+personaID),
				"persona volume must reference the hub-rendered ConfigMap by id")
		})

		It("should reconcile the agent deployment", func() {
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify deployment was created
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
		})

		It("should create a Secret with image env vars and inject them as explicit env entries", func() {
			By("Updating the AgentImage to include env vars")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Env = []ainselv1alpha1.AgentImageEnvVar{
				{Name: "CUSTOM_VAR", Value: "custom-value"},
				{Name: "ANOTHER_VAR", Value: "another-value"},
			}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			agentName := "agent-" + resourceName

			By("Verifying the image env Secret was created")
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName + "-image-env",
				Namespace: "default",
			}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKeyWithValue("CUSTOM_VAR", []byte("custom-value")))
			Expect(secret.Data).To(HaveKeyWithValue("ANOTHER_VAR", []byte("another-value")))

			By("Verifying the Deployment exposes each image env via explicit env entries")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())

			container := deploy.Spec.Template.Spec.Containers[0]
			// envFrom is no longer used; explicit env entries enable $(VAR)
			// substitution in MCP_SERVER_TOKENS.
			Expect(container.EnvFrom).To(BeEmpty())

			custom := findEnvVar(container.Env, "CUSTOM_VAR")
			Expect(custom).NotTo(BeNil())
			Expect(custom.ValueFrom).NotTo(BeNil())
			Expect(custom.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(custom.ValueFrom.SecretKeyRef.Name).To(Equal(agentName + "-image-env"))
			Expect(custom.ValueFrom.SecretKeyRef.Key).To(Equal("CUSTOM_VAR"))

			another := findEnvVar(container.Env, "ANOTHER_VAR")
			Expect(another).NotTo(BeNil())
			Expect(another.ValueFrom.SecretKeyRef.Key).To(Equal("ANOTHER_VAR"))
		})

		It("should delete the image env Secret when the AgentImage has no env vars", func() {
			agentName := "agent-" + resourceName

			By("Cleaning up any pre-existing image env Secret")
			_ = k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName + "-image-env",
					Namespace: "default",
				},
			})

			By("Pre-creating a leftover image env Secret owned by the Agent")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName + "-image-env",
					Namespace: "default",
				},
				StringData: map[string]string{"OLD_VAR": "old-value"},
			}
			Expect(controllerutil.SetControllerReference(agent, secret, k8sClient.Scheme())).To(Succeed())
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the image env Secret was deleted")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName + "-image-env",
				Namespace: "default",
			}, &corev1.Secret{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should not delete an image env Secret not owned by the Agent", func() {
			agentName := "agent-" + resourceName

			By("Cleaning up any pre-existing image env Secret")
			_ = k8sClient.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName + "-image-env",
					Namespace: "default",
				},
			})

			By("Pre-creating an image env Secret without owner references")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName + "-image-env",
					Namespace: "default",
				},
				StringData: map[string]string{"USER_VAR": "user-value"},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the image env Secret was NOT deleted")
			remaining := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName + "-image-env",
				Namespace: "default",
			}, remaining)).To(Succeed())
			Expect(remaining.Data).To(HaveKeyWithValue("USER_VAR", []byte("user-value")))
		})

		It("should reference the secret named in APIKeySecretRef for Ollama Cloud", func() {
			By("Updating the Agent to set a custom OllamaCloud APIKeySecretRef")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.OllamaCloud = &ainselv1alpha1.AgentOllamaCloud{
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "custom-ollama-secret"},
					Key:                  "api-key",
				},
			}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment references the custom secret")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			ollamaEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "OLLAMA_API_KEY")
			Expect(ollamaEnv).NotTo(BeNil())
			Expect(ollamaEnv.ValueFrom).NotTo(BeNil())
			Expect(ollamaEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(ollamaEnv.ValueFrom.SecretKeyRef.Name).To(Equal("custom-ollama-secret"))
		})

		It("should reference the secret named in APIKeySecretRef for OpenCode", func() {
			By("Updating the Agent to set a custom OpenCode APIKeySecretRef")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.OpenCode = &ainselv1alpha1.AgentOpenCode{
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "custom-opencode-secret"},
					Key:                  "api-key",
				},
			}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment references the custom secret")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			opencodeEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "OPENCODE_API_KEY")
			Expect(opencodeEnv).NotTo(BeNil())
			Expect(opencodeEnv.ValueFrom).NotTo(BeNil())
			Expect(opencodeEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(opencodeEnv.ValueFrom.SecretKeyRef.Name).To(Equal("custom-opencode-secret"))
		})

		It("should reference the secret named in APIKeySecretRef for Alibaba Cloud", func() {
			By("Updating the Agent to set a custom AlibabaCloud APIKeySecretRef")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.AlibabaCloud = &ainselv1alpha1.AgentAlibabaCloud{
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "custom-alibaba-secret"},
					Key:                  "api-key",
				},
			}
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment references the custom secret")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			alibabaEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "ALIBABA_CLOUD_API_KEY")
			Expect(alibabaEnv).NotTo(BeNil())
			Expect(alibabaEnv.ValueFrom).NotTo(BeNil())
			Expect(alibabaEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(alibabaEnv.ValueFrom.SecretKeyRef.Name).To(Equal("custom-alibaba-secret"))
		})

		It("should stamp a secret-hash annotation on the Deployment pod template", func() {
			By("Creating the referenced secrets")
			agentName := "agent-" + resourceName
			ollamaSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-ollama-key",
					Namespace: "default",
				},
				Data: map[string][]byte{"api-key": []byte("ollama-key-value")},
			}
			Expect(k8sClient.Create(ctx, ollamaSecret)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ollamaSecret) })

			opencodeSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-opencode-key",
					Namespace: "default",
				},
				Data: map[string][]byte{"api-key": []byte("opencode-key-value")},
			}
			Expect(k8sClient.Create(ctx, opencodeSecret)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, opencodeSecret) })

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment has the secret-hash annotation")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Annotations).To(HaveKey(secretHashAnnotation))
		})

		It("should change the secret-hash annotation when a referenced secret is updated", func() {
			By("Creating the referenced secrets")
			agentName := "agent-" + resourceName
			ollamaSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-ollama-key",
					Namespace: "default",
				},
				Data: map[string][]byte{"api-key": []byte("initial-value")},
			}
			Expect(k8sClient.Create(ctx, ollamaSecret)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ollamaSecret) })

			By("Reconciling to get the initial hash")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			initialHash := deploy.Spec.Template.Annotations[secretHashAnnotation]
			Expect(initialHash).NotTo(BeEmpty())

			By("Updating the secret data")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: resourceName + "-ollama-key", Namespace: "default"}, ollamaSecret)).To(Succeed())
			ollamaSecret.Data["api-key"] = []byte("updated-value")
			Expect(k8sClient.Update(ctx, ollamaSecret)).To(Succeed())

			By("Reconciling again and verifying the hash changed")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			newHash := deploy.Spec.Template.Annotations[secretHashAnnotation]
			Expect(newHash).NotTo(BeEmpty())
			Expect(newHash).NotTo(Equal(initialHash))
		})

		It("should find affected agents for a referenced secret", func() {
			agent := ainselv1alpha1.Agent{
				Spec: ainselv1alpha1.AgentSpec{
					OllamaCloud: &ainselv1alpha1.AgentOllamaCloud{
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "my-ollama-secret"},
							Key:                  "api-key",
						},
					},
					OpenCode: &ainselv1alpha1.AgentOpenCode{
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "my-opencode-secret"},
							Key:                  "api-key",
						},
					},
					AlibabaCloud: &ainselv1alpha1.AgentAlibabaCloud{
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "my-alibaba-secret"},
							Key:                  "api-key",
						},
					},
				},
			}
			agent.Name = "test-agent"

			Expect(referencesSecret(agent, "my-ollama-secret")).To(BeTrue())
			Expect(referencesSecret(agent, "my-opencode-secret")).To(BeTrue())
			Expect(referencesSecret(agent, "my-alibaba-secret")).To(BeTrue())
			Expect(referencesSecret(agent, "agent-test-agent-image-env")).To(BeTrue())
			Expect(referencesSecret(agent, "unrelated-secret")).To(BeFalse())
		})
		It("should stamp a persona-hash annotation on the Deployment pod template", func() {
			By("Creating the persona ConfigMap")
			const personaID = "01hxtestpersona00000000000"
			personaCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persona-" + personaID,
					Namespace: "default",
				},
				Data: map[string]string{
					"persona.md": "You are a helpful agent.",
				},
			}
			Expect(k8sClient.Create(ctx, personaCM)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, personaCM) })

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment has the persona-hash annotation")
			agentName := "agent-" + resourceName
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Annotations).To(HaveKey(personaHashAnnotation))
			initialHash := deploy.Spec.Template.Annotations[personaHashAnnotation]
			Expect(initialHash).NotTo(BeEmpty())
		})

		It("should change the persona-hash annotation when the persona ConfigMap data is updated", func() {
			By("Creating the persona ConfigMap with initial data")
			const personaID = "01hxtestpersona00000000000"
			personaCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persona-" + personaID,
					Namespace: "default",
				},
				Data: map[string]string{
					"persona.md": "You are a helpful agent.",
				},
			}
			Expect(k8sClient.Create(ctx, personaCM)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, personaCM) })

			By("Reconciling to get the initial hash")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			agentName := "agent-" + resourceName
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			initialHash := deploy.Spec.Template.Annotations[personaHashAnnotation]
			Expect(initialHash).NotTo(BeEmpty())

			By("Updating the persona ConfigMap data")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "persona-" + personaID,
				Namespace: "default",
			}, personaCM)).To(Succeed())
			personaCM.Data["persona.md"] = "You are an even more helpful agent."
			Expect(k8sClient.Update(ctx, personaCM)).To(Succeed())

			By("Reconciling again and verifying the hash changed")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			newHash := deploy.Spec.Template.Annotations[personaHashAnnotation]
			Expect(newHash).NotTo(BeEmpty())
			Expect(newHash).NotTo(Equal(initialHash))
		})

		It("should find affected agents for a persona ConfigMap change", func() {
			By("Creating agents with different persona IDs")
			agent1 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-persona-test-1",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: testImageName},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona000000000AA"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent1)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent1) })

			agent2 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-persona-test-2",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: testImageName},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona000000000BB"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent2)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent2) })

			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Mapping a persona ConfigMap to the matching agent only")
			personaCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "persona-01hxtestpersona000000000AA",
					Namespace: "default",
				},
			}
			requests := controllerReconciler.findAffectedAgentsFromConfigMap(ctx, personaCM)
			Expect(requests).To(ConsistOf(
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "agent-persona-test-1"}},
			))

			By("Ignoring a non-persona ConfigMap")
			otherCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-other-configmap",
					Namespace: "default",
				},
			}
			requests = controllerReconciler.findAffectedAgentsFromConfigMap(ctx, otherCM)
			Expect(requests).To(BeEmpty())
		})

		It("should reconcile successfully on first create", func() {
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the deployment was created")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
		})

		It("should find affected agents for an AgentImage change", func() {
			By("Creating a shared AgentImage")
			sharedImg := &ainselv1alpha1.AgentImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-image",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentImageSpec{
					ImageURL: "shared:latest",
				},
			}
			Expect(k8sClient.Create(ctx, sharedImg)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, sharedImg) })

			By("Creating agents referencing the shared and another image")
			agent1 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-with-shared-image",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: "shared-image"},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona00000000000"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent1)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent1) })

			agent2 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-with-shared-image-2",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: "shared-image"},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona00000000001"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent2)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent2) })

			agent3 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-with-other-image",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: "other-image"},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona00000000002"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent3)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent3) })

			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Mapping the shared AgentImage to referencing agents")
			requests := controllerReconciler.agentImageToAgents(ctx, sharedImg)
			Expect(requests).To(ConsistOf(
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "agent-with-shared-image"}},
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "agent-with-shared-image-2"}},
			))
		})

		It("should mount the skills ConfigMap with projected items when the AgentImage has enabledSkills", func() {
			By("Updating the AgentImage to include enabled skills")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.EnabledSkills = []string{"git-review", "bash-advanced"}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment mounts the skills volume with projected items")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())

			var skillsVol *corev1.Volume
			for i := range deploy.Spec.Template.Spec.Volumes {
				if deploy.Spec.Template.Spec.Volumes[i].Name == "agent-skills" {
					skillsVol = &deploy.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(skillsVol).NotTo(BeNil(), "skills volume must be present when EnabledSkills is set")
			Expect(skillsVol.ConfigMap).NotTo(BeNil())
			Expect(skillsVol.ConfigMap.Name).To(Equal("skills"))
			Expect(skillsVol.ConfigMap.Items).To(HaveLen(2))
			Expect(skillsVol.ConfigMap.Items).To(ContainElements(
				corev1.KeyToPath{Key: "git-review", Path: "git-review/SKILL.md"},
				corev1.KeyToPath{Key: "bash-advanced", Path: "bash-advanced/SKILL.md"},
			))

			By("Verifying the init container copies skills into the EmptyDir")
			initContainers := deploy.Spec.Template.Spec.InitContainers
			Expect(len(initContainers)).To(BeNumerically(">=", 1))
			setupCmd := initContainers[0].Command[2]
			Expect(setupCmd).To(ContainSubstring("cp -r /var/agent-skills/. /home/agent/.pi/agent/skills/"))
		})

		It("should stamp a skill-hash annotation on the Deployment pod template", func() {
			By("Creating the shared skills ConfigMap")
			skillsCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "skills",
					Namespace: "default",
				},
				Data: map[string]string{
					"git-review": "SKILL.md content",
				},
			}
			Expect(k8sClient.Create(ctx, skillsCM)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, skillsCM) })

			By("Updating the AgentImage to include enabled skills")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.EnabledSkills = []string{"git-review"}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Deployment has the skill-hash annotation")
			agentName := "agent-" + resourceName
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Annotations).To(HaveKey(skillHashAnnotation))
			initialHash := deploy.Spec.Template.Annotations[skillHashAnnotation]
			Expect(initialHash).NotTo(BeEmpty())
		})

		It("should change the skill-hash annotation when the skills ConfigMap data is updated", func() {
			By("Creating the skills ConfigMap with initial data")
			skillsCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "skills",
					Namespace: "default",
				},
				Data: map[string]string{
					"git-review": "initial SKILL.md content",
				},
			}
			Expect(k8sClient.Create(ctx, skillsCM)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, skillsCM) })

			By("Updating the AgentImage to include enabled skills")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.EnabledSkills = []string{"git-review"}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			agentName := "agent-" + resourceName
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			initialHash := deploy.Spec.Template.Annotations[skillHashAnnotation]
			Expect(initialHash).NotTo(BeEmpty())

			By("Updating the skills ConfigMap data")
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "skills",
				Namespace: "default",
			}, skillsCM)).To(Succeed())
			skillsCM.Data["git-review"] = "updated SKILL.md content"
			Expect(k8sClient.Update(ctx, skillsCM)).To(Succeed())

			By("Reconciling again and verifying the hash changed")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      agentName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			newHash := deploy.Spec.Template.Annotations[skillHashAnnotation]
			Expect(newHash).NotTo(BeEmpty())
			Expect(newHash).NotTo(Equal(initialHash))
		})

		It("should set a Degraded condition and emit a Warning Event when tokenFromEnv references a missing env var", func() {
			By("Updating the AgentImage with an MCP server whose tokenFromEnv is not defined in env")
			img := &ainselv1alpha1.AgentImage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Env = nil
			img.Spec.MCPServers = []ainselv1alpha1.AgentImageMCPServer{
				{Name: "forgejo-mcp-server", URL: "http://x", TokenFromEnv: "MISSING_TOKEN"},
			}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			By("Creating a fake recorder to capture Events")
			fakeRecorder := record.NewFakeRecorder(10)

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				Recorder:        fakeRecorder,
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Agent status has a Degraded condition with reason MCPTokenEnvMissing")
			updatedAgent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedAgent)).To(Succeed())
			degradedCondition := func() *metav1.Condition {
				for i := range updatedAgent.Status.Conditions {
					if updatedAgent.Status.Conditions[i].Type == ainselv1alpha1.AgentConditionDegraded {
						return &updatedAgent.Status.Conditions[i]
					}
				}
				return nil
			}()
			Expect(degradedCondition).NotTo(BeNil(), "Degraded condition must be set when tokenFromEnv references a missing env var")
			Expect(degradedCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(degradedCondition.Reason).To(Equal("MCPTokenEnvMissing"))
			Expect(degradedCondition.Message).To(ContainSubstring("forgejo-mcp-server"))
			Expect(degradedCondition.Message).To(ContainSubstring("MISSING_TOKEN"))

			By("Verifying a Warning Event was recorded")
			Expect(fakeRecorder.Events).To(Receive(And(
				ContainSubstring("MCPTokenEnvMissing"),
				ContainSubstring("forgejo-mcp-server"),
			)))

			By("Verifying MCP_SERVER_TOKENS is not set (all entries skipped)")
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "agent-" + resourceName,
				Namespace: "default",
			}, deploy)).To(Succeed())
			tokensEnv := findEnvVar(deploy.Spec.Template.Spec.Containers[0].Env, "MCP_SERVER_TOKENS")
			Expect(tokensEnv).To(BeNil())

			By("Fixing the AgentImage env to include the missing var")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.Env = []ainselv1alpha1.AgentImageEnvVar{
				{Name: "MISSING_TOKEN", Value: "now-present", Secret: true},
			}
			Expect(k8sClient.Update(ctx, img)).To(Succeed())

			By("Reconciling again to clear the Degraded condition")
			fakeRecorder2 := record.NewFakeRecorder(10)
			controllerReconciler2 := &AgentReconciler{
				Client:          k8sClient,
				Scheme:          k8sClient.Scheme(),
				Recorder:        fakeRecorder2,
			}
			_, err = controllerReconciler2.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Degraded condition is cleared")
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedAgent)).To(Succeed())
			degradedCondition = func() *metav1.Condition {
				for i := range updatedAgent.Status.Conditions {
					if updatedAgent.Status.Conditions[i].Type == ainselv1alpha1.AgentConditionDegraded {
						return &updatedAgent.Status.Conditions[i]
					}
				}
				return nil
			}()
			Expect(degradedCondition).NotTo(BeNil(), "Degraded condition should still exist but be False")
			Expect(degradedCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(degradedCondition.Reason).To(Equal("AsExpected"))

			By("Cleaning up — removing MCP servers from the image")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testImageName, Namespace: "default"}, img)).To(Succeed())
			img.Spec.MCPServers = nil
			img.Spec.Env = nil
			Expect(k8sClient.Update(ctx, img)).To(Succeed())
		})

		It("should find affected agents for a skills ConfigMap change", func() {
			By("Creating an AgentImage with enabled skills")
			skilledImg := &ainselv1alpha1.AgentImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "skilled-image",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentImageSpec{
					ImageURL:      "skilled:latest",
					EnabledSkills: []string{"git-review"},
				},
			}
			Expect(k8sClient.Create(ctx, skilledImg)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, skilledImg) })

			By("Creating an AgentImage without enabled skills")
			plainImg := &ainselv1alpha1.AgentImage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plain-image",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentImageSpec{
					ImageURL: "plain:latest",
				},
			}
			Expect(k8sClient.Create(ctx, plainImg)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, plainImg) })

			By("Creating agents referencing both images")
			agent1 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-skilled",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: "skilled-image"},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona00000000000"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent1)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent1) })

			agent2 := &ainselv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-plain",
					Namespace: "default",
				},
				Spec: ainselv1alpha1.AgentSpec{
					ImageRef: ainselv1alpha1.AgentImageRef{Name: "plain-image"},
					Persona:  ainselv1alpha1.AgentPersona{ID: "01hxtestpersona00000000001"},
					LLM:      ainselv1alpha1.AgentLLM{Model: "glm-5.1:cloud"},
				},
			}
			Expect(k8sClient.Create(ctx, agent2)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent2) })

			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Mapping the skills ConfigMap to agents with enabled skills")
			skillsCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "skills",
					Namespace: "default",
				},
			}
			requests := controllerReconciler.findAffectedAgentsFromConfigMap(ctx, skillsCM)
			Expect(requests).To(ConsistOf(
				reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "agent-skilled"}},
			))
		})

		It("should apply security hardening to the agent Deployment by default", func() {
			By("Reconciling without setting spec.runtime.securityHardened (defaults to true)")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			agentName := "agent-" + resourceName
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentName, Namespace: "default"}, deploy)).To(Succeed())

			By("Verifying pod-level securityContext")
			podSec := deploy.Spec.Template.Spec.SecurityContext
			Expect(podSec).NotTo(BeNil())
			Expect(*podSec.RunAsNonRoot).To(BeTrue())
			Expect(*podSec.RunAsUser).To(BeEquivalentTo(1000))
			Expect(*podSec.RunAsGroup).To(BeEquivalentTo(1000))
			Expect(*podSec.FSGroup).To(BeEquivalentTo(1000))
			Expect(podSec.SeccompProfile).NotTo(BeNil())
			Expect(podSec.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

			By("Verifying container-level securityContext on the agent container")
			agentContainer := deploy.Spec.Template.Spec.Containers[0]
			Expect(agentContainer.Name).To(Equal("agent"))
			Expect(agentContainer.SecurityContext).NotTo(BeNil())
			Expect(*agentContainer.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())
			Expect(*agentContainer.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(agentContainer.SecurityContext.Capabilities).NotTo(BeNil())
			Expect(agentContainer.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))

			By("Verifying the app.kubernetes.io/component label is set")
			Expect(deploy.Spec.Template.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "agents"))

			By("Verifying the /tmp emptyDir volume and mount exist")
			var tmpVol *corev1.Volume
			for i := range deploy.Spec.Template.Spec.Volumes {
				if deploy.Spec.Template.Spec.Volumes[i].Name == "tmp" {
					tmpVol = &deploy.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(tmpVol).NotTo(BeNil(), "/tmp emptyDir volume must be present when hardening is on")
			Expect(tmpVol.EmptyDir).NotTo(BeNil())

			var tmpMount *corev1.VolumeMount
			for i := range agentContainer.VolumeMounts {
				if agentContainer.VolumeMounts[i].Name == "tmp" {
					tmpMount = &agentContainer.VolumeMounts[i]
					break
				}
			}
			Expect(tmpMount).NotTo(BeNil(), "/tmp volume mount must be present on the agent container")
			Expect(tmpMount.MountPath).To(Equal("/tmp"))

			By("Verifying the init container has securityContext and no chown in command")
			initContainers := deploy.Spec.Template.Spec.InitContainers
			Expect(initContainers).NotTo(BeEmpty())
			setupInit := initContainers[0]
			Expect(setupInit.Name).To(Equal("setup-pi-models"))
			Expect(setupInit.SecurityContext).NotTo(BeNil())
			Expect(*setupInit.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(*setupInit.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
			Expect(setupInit.Command[2]).NotTo(ContainSubstring("chown"))
		})

		It("should skip security hardening when spec.runtime.securityHardened is false", func() {
			By("Setting spec.runtime.securityHardened to false")
			agent := &ainselv1alpha1.Agent{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, agent)).To(Succeed())
			agent.Spec.Runtime.SecurityHardened = ptr.To(false)
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			agentName := "agent-" + resourceName
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: agentName, Namespace: "default"}, deploy)).To(Succeed())

			By("Verifying pod-level securityContext has only fsGroup")
			podSec := deploy.Spec.Template.Spec.SecurityContext
			Expect(podSec).NotTo(BeNil())
			Expect(podSec.RunAsNonRoot).To(BeNil())
			Expect(podSec.RunAsUser).To(BeNil())
			Expect(podSec.RunAsGroup).To(BeNil())
			Expect(*podSec.FSGroup).To(BeEquivalentTo(1000))
			Expect(podSec.SeccompProfile).To(BeNil())

			By("Verifying agent container has no securityContext")
			agentContainer := deploy.Spec.Template.Spec.Containers[0]
			Expect(agentContainer.SecurityContext).To(BeNil())

			By("Verifying no /tmp volume")
			for _, v := range deploy.Spec.Template.Spec.Volumes {
				Expect(v.Name).NotTo(Equal("tmp"), "/tmp volume must not be present when hardening is off")
			}

			By("Verifying init container has no securityContext and retains chown")
			initContainers := deploy.Spec.Template.Spec.InitContainers
			Expect(initContainers).NotTo(BeEmpty())
			setupInit := initContainers[0]
			Expect(setupInit.SecurityContext).To(BeNil())
			Expect(setupInit.Command[2]).To(ContainSubstring("chown"))
		})
	})
})
