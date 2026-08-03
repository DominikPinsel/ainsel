package api

import (
	"fmt"
	"os"
	"strings"
)

type ConnectorConfig struct {
	PlatformExternalURL string
	ImageWebhook        string
}

func LoadConnectorConfig() ConnectorConfig {
	return ConnectorConfig{
		PlatformExternalURL: os.Getenv("PLATFORM_EXTERNAL_URL"),
		ImageWebhook:        os.Getenv("CONNECTOR_IMAGE_WEBHOOK"),
	}
}

func (c ConnectorConfig) WebhookEndpoint(connectorName string) string {
	base := strings.TrimRight(c.PlatformExternalURL, "/")
	return fmt.Sprintf("%s/webhooks/%s", base, connectorName)
}

func (c ConnectorConfig) WebhookImage() (repository, tag string, err error) {
	img := c.ImageWebhook
	if img == "" {
		return "", "", fmt.Errorf("CONNECTOR_IMAGE_WEBHOOK not configured")
	}
	parts := strings.SplitN(img, ":", 2)
	repository = parts[0]
	tag = "latest"
	if len(parts) == 2 {
		tag = parts[1]
	}
	return repository, tag, nil
}
