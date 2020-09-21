package ads_server

import "istio.io/client-go/pkg/apis/networking/v1alpha3"

type EventType string

const (
	AddEvent    EventType = "add"
	DeleteEvent EventType = "delete"
	UpdateEvent EventType = "update"
)

type ServiceEvent struct {
	Service *v1alpha3.Gateway
	Event   EventType
}
