package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) createSecret(ctx context.Context, name string, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.ns,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "ainsel-hub"},
		},
		Data: data,
	}
	return s.client.Create(ctx, secret)
}

func (s *Server) deleteSecret(ctx context.Context, name string) error {
	var secret corev1.Secret
	nn := types.NamespacedName{Name: name, Namespace: s.ns}
	if err := s.client.Get(ctx, nn, &secret); err != nil {
		return client.IgnoreNotFound(err)
	}
	return s.client.Delete(ctx, &secret)
}

func webhookSecretName(connectorName string) string {
	return fmt.Sprintf("connector-%s-webhook-hmac", connectorName)
}

