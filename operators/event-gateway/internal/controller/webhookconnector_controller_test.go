package controller

import (
	"context"
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
})
