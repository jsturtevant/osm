// Package catalog implements the MeshCataloger interface, which forms the central component in OSM that transforms
// outputs from all other components (SMI policies, Kubernetes services, endpoints etc.) into configuration that is
// consumed by the the proxy control plane component to program sidecar proxies.
// Reference: https://github.com/openservicemesh/osm/blob/main/DESIGN.md#5-mesh-catalog
package catalog

import (
	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha4"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"

	"github.com/openservicemesh/osm/pkg/certificate"
	"github.com/openservicemesh/osm/pkg/compute"
	"github.com/openservicemesh/osm/pkg/endpoint"
	"github.com/openservicemesh/osm/pkg/identity"
	"github.com/openservicemesh/osm/pkg/logger"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/openservicemesh/osm/pkg/smi"
	"github.com/openservicemesh/osm/pkg/trafficpolicy"
)

var (
	log = logger.New("mesh-catalog")
)

// MeshCatalog is the struct for the service catalog
type MeshCatalog struct {
	compute.Interface
	meshSpec    smi.MeshSpec
	certManager *certificate.Manager
}

// MeshCataloger is the mechanism by which the Service Mesh controller discovers all Envoy proxies connected to the catalog.
type MeshCataloger interface {
	compute.Interface

	// ListOutboundServicesForIdentity list the services the given service identity is allowed to initiate outbound connections to
	ListOutboundServicesForIdentity(identity.ServiceIdentity) []service.MeshService

	// ListInboundServiceIdentities lists the downstream service identities that are allowed to connect to the given service identity
	ListInboundServiceIdentities(identity.ServiceIdentity) []identity.ServiceIdentity

	// ListOutboundServiceIdentities lists the upstream service identities the given service identity are allowed to connect to
	ListOutboundServiceIdentities(identity.ServiceIdentity) []identity.ServiceIdentity

	// ListAllowedUpstreamEndpointsForService returns the list of endpoints over which the downstream client identity
	// is allowed access the upstream service
	ListAllowedUpstreamEndpointsForService(identity.ServiceIdentity, service.MeshService) []endpoint.Endpoint

	// GetIngressTrafficPolicies returns a list of IngressTrafficPolicy objects for the given MeshService list
	GetIngressTrafficPolicies([]service.MeshService) []*trafficpolicy.IngressTrafficPolicy

	// GetIngressTrafficPolicy returns the ingress traffic policy for the given mesh service
	// TODO: deprecate in favor of GetIngressTrafficPolicies
	GetIngressTrafficPolicy(service.MeshService) (*trafficpolicy.IngressTrafficPolicy, error)

	// ListInboundTrafficTargetsWithRoutes returns a list traffic target objects composed of its routes for the given destination service identity
	ListInboundTrafficTargetsWithRoutes(identity.ServiceIdentity) ([]trafficpolicy.TrafficTargetWithRoutes, error)

	// GetEgressTrafficPolicy returns the Egress traffic policy associated with the given service identity.
	GetEgressTrafficPolicy(identity.ServiceIdentity) (*trafficpolicy.EgressTrafficPolicy, error)

	// GetOutboundMeshTrafficPolicy returns the outbound mesh traffic policy for the given downstream identity
	GetOutboundMeshTrafficPolicy(identity.ServiceIdentity) *trafficpolicy.OutboundMeshTrafficPolicy

	// GetInboundMeshTrafficPolicy returns the inbound mesh traffic policy for the given upstream identity and services
	GetInboundMeshTrafficPolicy(identity.ServiceIdentity, []service.MeshService) *trafficpolicy.InboundMeshTrafficPolicy

	ListSMIPolicies() ([]*split.TrafficSplit, []identity.K8sServiceAccount, []*spec.HTTPRouteGroup, []*access.TrafficTarget)
}

type trafficDirection string

const (
	inbound  trafficDirection = "inbound"
	outbound trafficDirection = "outbound"
)
