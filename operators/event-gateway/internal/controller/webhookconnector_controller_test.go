package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/DominikPinsel/ainsel/shared/api/api/v1alpha1"
)

// sha256Hex returns the hex-encoded SHA-256 hash of s.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

var _ = Describe("WebhookConnector Controller", func() {
	const (
		namespace       = "default"
		connectorName   = "test-webhook"
		webhookEndpoint = "https://example.com/ainsel-dev/webhooks/test-webhook"
		resourceName    = "connector-" + connectorName
		ingressName     = "connector-" + connectorName + "-webhook"
	)

	ctx := context.Background()
	connectorKey := types.NamespacedName{Name: connectorName, Namespace: namespace}
	deployKey := types.NamespacedName{Name: resourceName, Namespace: namespace}
	svcKey := types.NamespacedName{Name: resourceName, Namespace: namespace}
	ingKey := types.NamespacedName{Name: ingressName, Namespace: namespace}

	newConnector := func(disabled bool) *commonv1alpha1.WebhookConnector {
		return &commonv1alpha1.WebhookConnector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      connectorName,
				Namespace: namespace,
			},
			Spec: commonv1alpha1.WebhookConnectorSpec{
				WebhookEndpoint: webhookEndpoint,
				SignatureHeader: "X-Hub-Signature-256",
				WebhookSecret: commonv1alpha1.SecretKeyRef{
					SecretRef: commonv1alpha1.SecretRef{Name: "test-secret", Key: "secret"},
				},
				Image: commonv1alpha1.ConnectorImage{
					Repository: "example.com/webhook-receiver",
					Tag:        "latest",
				},
				Disabled: disabled,
			},
		}
	}

	reconcileOnce := func() {
		r := &WebhookConnectorReconciler{
			Client:                   k8sClient,
			Scheme:                   k8sClient.Scheme(),
			ConnectorImagePullPolicy: corev1.PullAlways,
		}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: connectorKey})
		Expect(err).NotTo(HaveOccurred())
	}

	AfterEach(func() {
		conn := &commonv1alpha1.WebhookConnector{}
		if err := k8sClient.Get(ctx, connectorKey, conn); err == nil {
			Expect(k8sClient.Delete(ctx, conn)).To(Succeed())
		}

		dep := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, deployKey, dep); err == nil {
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
		}

		svc := &corev1.Service{}
		if err := k8sClient.Get(ctx, svcKey, svc); err == nil {
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
		}

		ing := &networkingv1.Ingress{}
		if err := k8sClient.Get(ctx, ingKey, ing); err == nil {
			Expect(k8sClient.Delete(ctx, ing)).To(Succeed())
		}

		sec := &corev1.Secret{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-secret", Namespace: namespace}, sec); err == nil {
			Expect(k8sClient.Delete(ctx, sec)).To(Succeed())
		}
	})

	It("creates Deployment, Service and Ingress and sets Ready=True", func() {
		Expect(k8sClient.Create(ctx, newConnector(false))).To(Succeed())
		reconcileOnce()

		By("verifying Deployment has one replica and the correct image")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
		Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		containers := dep.Spec.Template.Spec.Containers
		Expect(containers).To(HaveLen(1))
		Expect(containers[0].Image).To(Equal("example.com/webhook-receiver:latest"))
		Expect(containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))

		By("verifying Service exposes port 80")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, svcKey, svc)).To(Succeed())
		Expect(svc.Spec.Ports).To(HaveLen(1))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(80)))

		By("verifying Ingress routes the webhook path")
		ing := &networkingv1.Ingress{}
		Expect(k8sClient.Get(ctx, ingKey, ing)).To(Succeed())
		Expect(ing.Spec.Rules).To(HaveLen(1))
		Expect(ing.Spec.Rules[0].Host).To(Equal("example.com"))
		paths := ing.Spec.Rules[0].HTTP.Paths
		Expect(paths).To(HaveLen(1))
		Expect(paths[0].Path).To(Equal("/ainsel-dev/webhooks/test-webhook"))

		By("verifying Ready condition is True")
		Eventually(func() bool {
			conn := &commonv1alpha1.WebhookConnector{}
			if err := k8sClient.Get(ctx, connectorKey, conn); err != nil {
				return false
			}
			for _, c := range conn.Status.Conditions {
				if c.Type == commonv1alpha1.ConnectorConditionReady && c.Status == metav1.ConditionTrue {
					return true
				}
			}
			return false
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("scales Deployment to zero and sets Ready=False when disabled", func() {
		Expect(k8sClient.Create(ctx, newConnector(true))).To(Succeed())
		reconcileOnce()

		By("verifying Deployment has zero replicas")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
		Expect(*dep.Spec.Replicas).To(Equal(int32(0)))

		By("verifying Ready condition is False with Disabled reason")
		Eventually(func() bool {
			conn := &commonv1alpha1.WebhookConnector{}
			if err := k8sClient.Get(ctx, connectorKey, conn); err != nil {
				return false
			}
			for _, c := range conn.Status.Conditions {
				if c.Type == commonv1alpha1.ConnectorConditionReady &&
					c.Status == metav1.ConditionFalse &&
					c.Reason == "Disabled" {
					return true
				}
			}
			return false
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
	})

	It("returns without error when the connector does not exist", func() {
		r := &WebhookConnectorReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		_, err := r.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
		})
		Expect(err).NotTo(HaveOccurred())

		dep := &appsv1.Deployment{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "connector-nonexistent", Namespace: namespace}, dep)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("stamps the pod template with a hash of the webhook secret", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: namespace},
			Data:       map[string][]byte{"secret": []byte("first-secret")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		Expect(k8sClient.Create(ctx, newConnector(false))).To(Succeed())
		reconcileOnce()

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
		Expect(dep.Spec.Template.Annotations).To(HaveKeyWithValue(
			"ainsel.dev/webhook-secret-hash", sha256Hex("first-secret")))
	})

	It("uses an empty hash when the webhook secret does not exist yet", func() {
		// newConnector references "test-secret", which is not created here.
		Expect(k8sClient.Create(ctx, newConnector(false))).To(Succeed())
		reconcileOnce()

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
		Expect(dep.Spec.Template.Annotations).To(HaveKeyWithValue(
			"ainsel.dev/webhook-secret-hash", ""))
	})

	It("rolls the Deployment when the webhook secret is rotated", func() {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: namespace},
			Data:       map[string][]byte{"secret": []byte("first-secret")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		Expect(k8sClient.Create(ctx, newConnector(false))).To(Succeed())
		reconcileOnce()

		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
		Expect(dep.Spec.Template.Annotations).To(HaveKeyWithValue(
			"ainsel.dev/webhook-secret-hash", sha256Hex("first-secret")))

		By("rotating the secret and reconciling again")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-secret", Namespace: namespace}, secret)).To(Succeed())
		secret.Data["secret"] = []byte("second-secret")
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())
		reconcileOnce()

		// The changed pod-template annotation forces the deployment controller
		// to roll new pods, which resolve the rotated secret at startup.
		Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
		Expect(dep.Spec.Template.Annotations).To(HaveKeyWithValue(
			"ainsel.dev/webhook-secret-hash", sha256Hex("second-secret")))
	})
})

var _ = Describe("mapWebhookSecretToConnector", func() {
	r := &WebhookConnectorReconciler{}
	const namespace = "default"

	It("maps a labeled secret to its connector", func() {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "anything",
				Namespace: namespace,
				Labels:    map[string]string{"ainsel.dev/connector": "c-1234"},
			},
		}
		reqs := r.mapWebhookSecretToConnector(ctx, sec)
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0].Name).To(Equal("c-1234"))
		Expect(reqs[0].Namespace).To(Equal(namespace))
	})

	It("maps an unlabeled secret by its naming convention", func() {
		sec := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "connector-c-1234-webhook-hmac",
				Namespace: namespace,
			},
		}
		reqs := r.mapWebhookSecretToConnector(ctx, sec)
		Expect(reqs).To(HaveLen(1))
		Expect(reqs[0].Name).To(Equal("c-1234"))
		Expect(reqs[0].Namespace).To(Equal(namespace))
	})

	It("ignores unrelated secrets", func() {
		for _, name := range []string{"some-other-secret", "connector-", "-webhook-hmac", "connector--webhook-hmac"} {
			sec := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			}
			Expect(r.mapWebhookSecretToConnector(ctx, sec)).To(BeEmpty(), "expected no mapping for %q", name)
		}
	})
})
