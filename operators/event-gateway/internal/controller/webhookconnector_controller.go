package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

const (
	// webhookSecretHashAnnotation is set on the connector deployment's pod
	// template. Its value changes whenever the webhook HMAC secret changes,
	// which forces a rolling restart of the webhook-receiver pod. This is
	// required because the secret is injected as an environment variable,
	// and env vars from secrets are only resolved at pod startup — without
	// the restart, a rotated secret would never take effect.
	webhookSecretHashAnnotation = "ainsel.dev/webhook-secret-hash" // #nosec G101 -- annotation key name, not a credential

	// webhookConnectorLabel links a webhook HMAC secret back to its
	// WebhookConnector. It is used to map secret events to reconcile
	// requests.
	webhookConnectorLabel = "ainsel.dev/connector"
)

// WebhookConnectorReconciler reconciles a WebhookConnector object.
//
// The reconciler creates and manages:
//   - Deployment: runs the webhook-receiver pod
//   - Service: exposes the webhook endpoint within the cluster
//   - Ingress: routes external webhook traffic to the connector service
type WebhookConnectorReconciler struct {
	client.Client
	Scheme                   *runtime.Scheme
	HubURL                   string
	HubInternalSecret        string
	ConnectorImagePullPolicy corev1.PullPolicy
}

//+kubebuilder:rbac:groups=ainsel.dev,resources=webhookconnectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ainsel.dev,resources=webhookconnectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// Reconcile ensures that the Deployment, Service, and Ingress for the
// WebhookConnector exist and are up to date.
func (r *WebhookConnectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var connector commonv1alpha1.WebhookConnector
	if err := r.Get(ctx, req.NamespacedName, &connector); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !connector.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling WebhookConnector", "name", connector.Name)

	if err := r.reconcileWebhookDeployment(ctx, &connector); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling deployment: %w", err)
	}
	if err := r.reconcileWebhookService(ctx, &connector); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling service: %w", err)
	}
	if err := r.reconcileWebhookIngress(ctx, &connector); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ingress: %w", err)
	}
	if err := r.updateWebhookStatus(ctx, &connector); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	return ctrl.Result{}, nil
}

func webhookConnectorResourceName(cr *commonv1alpha1.WebhookConnector) string {
	return "connector-" + cr.Name
}

func webhookConnectorLabels(cr *commonv1alpha1.WebhookConnector) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       webhookConnectorResourceName(cr),
		"app.kubernetes.io/managed-by": "connector-operator",
		"ainsel.dev/connector":         cr.Name,
	}
}

func (r *WebhookConnectorReconciler) reconcileWebhookDeployment(ctx context.Context, cr *commonv1alpha1.WebhookConnector) error {
	name := webhookConnectorResourceName(cr)
	labels := webhookConnectorLabels(cr)

	replicas := int32(1)
	if cr.Spec.Disabled {
		replicas = 0
	}

	secretHash, err := r.webhookSecretHash(ctx, cr)
	if err != nil {
		return err
	}

	hubURL := r.HubURL

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := ctrl.SetControllerReference(cr, dep, r.Scheme); err != nil {
			return err
		}

		dep.Labels = labels
		dep.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					// Changing the annotation forces a rolling restart so
					// the pod picks up the (possibly rotated) secret.
					Annotations: map[string]string{
						webhookSecretHashAnnotation: secretHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "webhook-receiver",
							Image:           fmt.Sprintf("%s:%s", cr.Spec.Image.Repository, cr.Spec.Image.Tag),
							ImagePullPolicy: r.ConnectorImagePullPolicy,
							Env: []corev1.EnvVar{
								{Name: "CONNECTOR_NAME", Value: cr.Name},
								{Name: "SIGNATURE_HEADER", Value: cr.Spec.SignatureHeader},
								{Name: "HUB_URL", Value: hubURL},
								{Name: "HUB_INTERNAL_SECRET", Value: r.HubInternalSecret},
								{
									Name: "WEBHOOK_SECRET",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.WebhookSecret.SecretRef.Name,
											},
											Key: cr.Spec.WebhookSecret.SecretRef.Key,
										},
									},
								},
							},
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromString("http"),
									},
								},
							},
						},
					},
				},
			},
		}
		return nil
	})
	return err
}

func (r *WebhookConnectorReconciler) reconcileWebhookService(ctx context.Context, cr *commonv1alpha1.WebhookConnector) error {
	name := webhookConnectorResourceName(cr)
	labels := webhookConnectorLabels(cr)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := ctrl.SetControllerReference(cr, svc, r.Scheme); err != nil {
			return err
		}

		svc.Labels = labels
		svc.Spec.Selector = labels
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "http", Port: 80, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
		}
		return nil
	})
	return err
}

// webhookSecretHash returns the hex-encoded SHA-256 hash of the webhook HMAC
// secret referenced by the connector. The hash (rather than the secret
// itself) is stored in the pod template annotation so deployments do not leak
// the credential. A missing secret yields an empty hash: the pod cannot start
// without the secret anyway, and reconciles will retry once it exists.
func (r *WebhookConnectorReconciler) webhookSecretHash(ctx context.Context, cr *commonv1alpha1.WebhookConnector) (string, error) {
	ref := cr.Spec.WebhookSecret.SecretRef
	var sec corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: cr.Namespace}, &sec); err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("fetching webhook secret %s: %w", ref.Name, err)
	}
	sum := sha256.Sum256(sec.Data[ref.Key])
	return hex.EncodeToString(sum[:]), nil
}

// reconcileWebhookIngress creates/updates the Ingress for this WebhookConnector.
// The host and path are extracted from spec.webhookEndpoint, following the same
// pattern as the WebhookConnector controller.
func (r *WebhookConnectorReconciler) reconcileWebhookIngress(ctx context.Context, cr *commonv1alpha1.WebhookConnector) error {
	// Parse the webhook endpoint to extract the host and path.
	// e.g. "https://ainsel.example.com/webhooks/c-79177193"
	//        -> host: "ainsel.example.com", path: "/webhooks/c-79177193"
	webhookURL, err := url.Parse(cr.Spec.WebhookEndpoint)
	if err != nil {
		return fmt.Errorf("parsing webhook endpoint: %w", err)
	}

	path := webhookURL.Path
	if path == "" {
		return fmt.Errorf("webhook endpoint has no path: %s", cr.Spec.WebhookEndpoint)
	}

	name := webhookConnectorResourceName(cr) + "-webhook"
	labels := webhookConnectorLabels(cr)
	svcName := webhookConnectorResourceName(cr)

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cr.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, ing, func() error {
		if err := ctrl.SetControllerReference(cr, ing, r.Scheme); err != nil {
			return err
		}

		ing.Labels = labels
		ing.Annotations = map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/publish",
			"nginx.ingress.kubernetes.io/ssl-redirect":   "true",
		}

		ing.Spec = networkingv1.IngressSpec{
			IngressClassName: ptrString("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: webhookURL.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypeExact; return &pt }(),
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: svcName,
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []networkingv1.IngressTLS{
				{
					Hosts:      []string{webhookURL.Host},
					SecretName: "webhook-tls", // #nosec G101 -- K8s secret reference name, not a credential
				},
			},
		}
		return nil
	})
	return err
}

func (r *WebhookConnectorReconciler) updateWebhookStatus(ctx context.Context, cr *commonv1alpha1.WebhookConnector) error {
	now := metav1.Now()

	readyCondition := metav1.Condition{
		Type:               commonv1alpha1.ConnectorConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "ReconcileSucceeded",
		Message:            "Deployment, Service, and Ingress are reconciled",
		LastTransitionTime: now,
	}

	if cr.Spec.Disabled {
		readyCondition.Status = metav1.ConditionFalse
		readyCondition.Reason = "Disabled"
		readyCondition.Message = "Connector is disabled; Deployment scaled to zero replicas"
	}

	newStatus := commonv1alpha1.WebhookConnectorStatus{
		ObservedGeneration: cr.Generation,
		Conditions:         []metav1.Condition{readyCondition},
	}

	// Preserve the existing transition time when the condition has not changed.
	if existing := findCondition(cr.Status.Conditions, readyCondition.Type); existing != nil &&
		existing.Status == readyCondition.Status && existing.Reason == readyCondition.Reason {
		newStatus.Conditions[0].LastTransitionTime = existing.LastTransitionTime
	}

	cr.Status = newStatus
	return r.Status().Update(ctx, cr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebhookConnectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&commonv1alpha1.WebhookConnector{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		// Watch webhook HMAC secrets so that a secret rotation triggers a
		// reconcile and the deployment rolls to pick up the new value.
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.mapWebhookSecretToConnector)).
		Named("webhookconnector").
		Complete(r)
}

// mapWebhookSecretToConnector maps a webhook HMAC secret event to a reconcile
// request for its WebhookConnector. The connector is identified via the
// ainsel.dev/connector label; secrets created before the label existed are
// matched by their naming convention (connector-<id>-webhook-hmac).
func (r *WebhookConnectorReconciler) mapWebhookSecretToConnector(_ context.Context, obj client.Object) []reconcile.Request {
	if id := obj.GetLabels()[webhookConnectorLabel]; id != "" {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: id, Namespace: obj.GetNamespace()},
		}}
	}

	const prefix, suffix = "connector-", "-webhook-hmac"
	name := obj.GetName()
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) ||
		len(name) <= len(prefix)+len(suffix) {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix),
			Namespace: obj.GetNamespace(),
		},
	}}
}
