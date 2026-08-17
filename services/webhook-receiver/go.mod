module github.com/DominikPinsel/ainsel/services/webhook-receiver

go 1.26.1

require (
	github.com/DominikPinsel/ainsel/shared/api v0.1.0
	github.com/google/uuid v1.6.0
	github.com/prometheus/client_golang v1.24.1
	github.com/prometheus/client_model v0.6.2
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	golang.org/x/sys v0.47.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace github.com/DominikPinsel/ainsel/shared/api => ../../shared/api
