package kube

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	mapset "github.com/deckarep/golang-set"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/utils/pointer"

	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"

	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/envoy"

	"github.com/openservicemesh/osm/pkg/endpoint"
	"github.com/openservicemesh/osm/pkg/identity"
	"github.com/openservicemesh/osm/pkg/k8s"
	"github.com/openservicemesh/osm/pkg/service"
)

// NewClient returns a client that has all components necessary to connect to and maintain state of a Kubernetes cluster.
func NewClient(kubeController k8s.Controller) *client { //nolint: revive // unexported-return
	return &client{
		PassthroughInterface: kubeController,
		kubeController:       kubeController,
	}
}

// ListEndpointsForService retrieves the list of IP addresses for the given service
func (c *client) ListEndpointsForService(svc service.MeshService) []endpoint.Endpoint {
	log.Trace().Msgf("Getting Endpoints for MeshService %s on Kubernetes", svc)

	kubernetesEndpoints, err := c.kubeController.GetEndpoints(svc.Name, svc.Namespace)
	if err != nil || kubernetesEndpoints == nil {
		log.Info().Msgf("No k8s endpoints found for MeshService %s", svc)
		return nil
	}

	var endpoints []endpoint.Endpoint
	for _, kubernetesEndpoint := range kubernetesEndpoints.Subsets {
		for _, port := range kubernetesEndpoint.Ports {
			// If a TargetPort is specified for the service, filter the endpoint by this port.
			// This is required to ensure we do not attempt to filter the endpoints when the endpoints
			// are being listed for a MeshService whose TargetPort is not known.
			if svc.TargetPort != 0 && port.Port != int32(svc.TargetPort) {
				// k8s service's port does not match MeshService port, ignore this port
				continue
			}
			for _, address := range kubernetesEndpoint.Addresses {
				if svc.Subdomain != "" && svc.Subdomain != address.Hostname {
					// if there's a subdomain on this meshservice, make sure it matches the endpoint's hostname
					continue
				}
				ip := net.ParseIP(address.IP)
				if ip == nil {
					log.Error().Msgf("Error parsing endpoint IP address %s for MeshService %s", address.IP, svc)
					continue
				}
				ept := endpoint.Endpoint{
					IP:   ip,
					Port: endpoint.Port(port.Port),
				}
				endpoints = append(endpoints, ept)
			}
		}
	}

	log.Trace().Msgf("Endpoints for MeshService %s: %v", svc, endpoints)

	return endpoints
}

// ListEndpointsForIdentity retrieves the list of IP addresses for the given service account
// Note: ServiceIdentity must be in the format "name.namespace" [https://github.com/openservicemesh/osm/issues/3188]
func (c *client) ListEndpointsForIdentity(serviceIdentity identity.ServiceIdentity) []endpoint.Endpoint {
	sa := serviceIdentity.ToK8sServiceAccount()
	log.Trace().Msgf("(ListEndpointsForIdentity) Getting Endpoints for service account %s on Kubernetes", sa)

	var endpoints []endpoint.Endpoint
	for _, pod := range c.kubeController.ListPods() {
		if pod.Namespace != sa.Namespace {
			continue
		}
		if pod.Spec.ServiceAccountName != sa.Name {
			continue
		}

		for _, podIP := range pod.Status.PodIPs {
			ip := net.ParseIP(podIP.IP)
			if ip == nil {
				log.Error().Msgf("Error parsing IP address %s", podIP.IP)
				break
			}
			ept := endpoint.Endpoint{IP: ip}
			endpoints = append(endpoints, ept)
		}
	}

	log.Trace().Msgf("[ListEndpointsForIdentity] Endpoints for service identity (serviceAccount=%s) %s: %+v", serviceIdentity, sa, endpoints)

	return endpoints
}

// GetServicesForServiceIdentity retrieves a list of services for the given service identity.
func (c *client) GetServicesForServiceIdentity(svcIdentity identity.ServiceIdentity) []service.MeshService {
	var meshServices []service.MeshService
	svcSet := mapset.NewSet() // mapset is used to avoid duplicate elements in the output list
	svcAccount := svcIdentity.ToK8sServiceAccount()

	for _, pod := range c.kubeController.ListPods() {
		if pod.Namespace != svcAccount.Namespace {
			continue
		}

		if pod.Spec.ServiceAccountName != svcAccount.Name {
			continue
		}

		for _, svc := range c.listServicesForPod(pod) {
			if added := svcSet.Add(svc); added {
				meshServices = append(meshServices, svc)
			}
		}
	}

	log.Trace().Msgf("Services for service account %s: %v", svcAccount, meshServices)
	return meshServices
}

// ListServicesForProxy maps an Envoy instance to a number of Kubernetes services.
func (c *client) ListServicesForProxy(p *envoy.Proxy) ([]service.MeshService, error) {
	pod, err := c.kubeController.GetPodForProxy(p)
	if err != nil {
		return nil, err
	}
	return c.listServicesForPod(pod), nil
}

func (c *client) listServicesForPod(pod *corev1.Pod) []service.MeshService {
	var meshServices []service.MeshService
	for _, svc := range c.getServicesByLabels(pod.ObjectMeta.Labels, pod.Namespace) {
		// Filter out headless services that point to a specific proxy.
		if svc.Subdomain == pod.Name || svc.Subdomain == "" {
			meshServices = append(meshServices, svc)
		}
	}

	log.Trace().Msgf("Services associated with Pod with UID=%s Name=%s/%s: %v",
		pod.ObjectMeta.UID, pod.Namespace, pod.Name, meshServices)

	return meshServices
}

// getServicesByLabels gets Kubernetes services whose selectors match the given labels
func (c *client) getServicesByLabels(podLabels map[string]string, targetNamespace string) []service.MeshService {
	var finalList []service.MeshService
	serviceList := c.kubeController.ListServices()

	for _, svc := range serviceList {
		// TODO: #1684 Introduce APIs to dynamically allow applying selectors, instead of callers implementing
		// filtering themselves
		if svc.Namespace != targetNamespace {
			continue
		}

		svcRawSelector := svc.Spec.Selector
		// service has no selectors, we do not need to match against the pod label
		if len(svcRawSelector) == 0 {
			continue
		}
		selector := labels.Set(svcRawSelector).AsSelector()
		if selector.Matches(labels.Set(podLabels)) {
			finalList = append(finalList, c.serviceToMeshServices(*svc)...)
		}
	}

	return finalList
}

// GetResolvableEndpointsForService returns the expected endpoints that are to be reached when the service
// FQDN is resolved
func (c *client) GetResolvableEndpointsForService(svc service.MeshService) []endpoint.Endpoint {
	var endpoints []endpoint.Endpoint

	// Check if the service has been given Cluster IP
	kubeService := c.kubeController.GetService(svc.Name, svc.Namespace)
	if kubeService == nil {
		log.Info().Msgf("No k8s services found for MeshService %s", svc)
		return nil
	}

	if len(kubeService.Spec.ClusterIP) == 0 || kubeService.Spec.ClusterIP == corev1.ClusterIPNone {
		// If service has no cluster IP or cluster IP is <none>, use final endpoint as resolvable destinations
		return c.ListEndpointsForService(svc)
	}

	// Cluster IP is present
	ip := net.ParseIP(kubeService.Spec.ClusterIP)
	if ip == nil {
		log.Error().Msgf("Could not parse Cluster IP %s", kubeService.Spec.ClusterIP)
		return nil
	}

	for _, svcPort := range kubeService.Spec.Ports {
		endpoints = append(endpoints, endpoint.Endpoint{
			IP:   ip,
			Port: endpoint.Port(svcPort.Port),
		})
	}

	return endpoints
}

// ListServices returns a list of services that are part of monitored namespaces
func (c *client) ListServices() []service.MeshService {
	var services []service.MeshService
	for _, svc := range c.kubeController.ListServices() {
		services = append(services, c.serviceToMeshServices(*svc)...)
	}
	return services
}

// IsMetricsEnabled checks if prometheus metrics scraping are enabled on this pod.
func (c *client) IsMetricsEnabled(proxy *envoy.Proxy) (bool, error) {
	pod, err := c.kubeController.GetPodForProxy(proxy)
	if err != nil {
		return false, err
	}
	val, ok := pod.Annotations[constants.PrometheusScrapeAnnotation]
	if !ok {
		return false, nil
	}

	return strconv.ParseBool(val)
}

// GetHostnamesForService returns the hostnames over which the service is accessible
func (c *client) GetHostnamesForService(svc service.MeshService, localNamespace bool) []string {
	var hostnames []string

	if localNamespace {
		hostnames = append(hostnames, []string{
			svc.Name,                                 // service
			fmt.Sprintf("%s:%d", svc.Name, svc.Port), // service:port
		}...)
	}

	hostnames = append(hostnames, []string{
		fmt.Sprintf("%s.%s", svc.Name, svc.Namespace),                                // service.namespace
		fmt.Sprintf("%s.%s:%d", svc.Name, svc.Namespace, svc.Port),                   // service.namespace:port
		fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),                            // service.namespace.svc
		fmt.Sprintf("%s.%s.svc:%d", svc.Name, svc.Namespace, svc.Port),               // service.namespace.svc:port
		fmt.Sprintf("%s.%s.svc.cluster", svc.Name, svc.Namespace),                    // service.namespace.svc.cluster
		fmt.Sprintf("%s.%s.svc.cluster:%d", svc.Name, svc.Namespace, svc.Port),       // service.namespace.svc.cluster:port
		fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace),              // service.namespace.svc.cluster.local
		fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, svc.Port), // service.namespace.svc.cluster.local:port
	}...)

	return hostnames
}

// ListEgressPoliciesForServiceAccount lists the Egress policies for the given source identity based on service accounts
func (c *client) ListEgressPoliciesForServiceAccount(source identity.K8sServiceAccount) []*policyv1alpha1.Egress {
	var policies []*policyv1alpha1.Egress

	for _, egress := range c.kubeController.ListEgressPolicies() {
		for _, sourceSpec := range egress.Spec.Sources {
			if sourceSpec.Kind == kindSvcAccount && sourceSpec.Name == source.Name && sourceSpec.Namespace == source.Namespace {
				policies = append(policies, egress)
			}
		}
	}

	return policies
}

// GetIngressBackendPolicyForService returns the IngressBackend policy for the given backend MeshService
func (c *client) GetIngressBackendPolicyForService(svc service.MeshService) *policyv1alpha1.IngressBackend {
	for _, ingressBackend := range c.kubeController.ListIngressBackendPolicies() {
		// Return the first IngressBackend corresponding to the given MeshService.
		// Multiple IngressBackend policies for the same backend will be prevented
		// using a validating webhook.
		for _, backend := range ingressBackend.Spec.Backends {
			// we need to check ports to allow ingress to multiple ports on the same svc
			if backend.Name == svc.Name && backend.Port.Number == int(svc.TargetPort) {
				return ingressBackend
			}
		}
	}
	return nil
}

// ListRetryPoliciesForServiceAccount returns the retry policies for the given source identity based on service accounts.
func (c *client) ListRetryPoliciesForServiceAccount(source identity.K8sServiceAccount) []*policyv1alpha1.Retry {
	var retries []*policyv1alpha1.Retry

	for _, retry := range c.kubeController.ListRetryPolicies() {
		if retry.Spec.Source.Kind == kindSvcAccount && retry.Spec.Source.Name == source.Name && retry.Spec.Source.Namespace == source.Namespace {
			retries = append(retries, retry)
		}
	}

	return retries
}

// GetUpstreamTrafficSettingByNamespace returns the UpstreamTrafficSetting resource that matches the namespace
func (c *client) GetUpstreamTrafficSettingByNamespace(namespace *types.NamespacedName) *policyv1alpha1.UpstreamTrafficSetting {
	if namespace == nil {
		log.Error().Msgf("No option specified to get UpstreamTrafficSetting resource")
		return nil
	}

	return c.kubeController.GetUpstreamTrafficSetting(namespace)
}

// GetUpstreamTrafficSettingByService returns the UpstreamTrafficSetting resource that matches the given service
func (c *client) GetUpstreamTrafficSettingByService(meshService *service.MeshService) *policyv1alpha1.UpstreamTrafficSetting {
	if meshService == nil {
		log.Error().Msgf("No option specified to get UpstreamTrafficSetting resource")
		return nil
	}

	// Filter by MeshService
	for _, setting := range c.kubeController.ListUpstreamTrafficSettings() {
		if setting != nil && setting.Namespace == meshService.Namespace && setting.Spec.Host == meshService.FQDN() {
			return setting
		}
	}

	return nil
}

// GetUpstreamTrafficSettingByHost returns the UpstreamTrafficSetting resource that matches the host
func (c *client) GetUpstreamTrafficSettingByHost(host string) *policyv1alpha1.UpstreamTrafficSetting {
	if host == "" {
		log.Error().Msgf("No option specified to get UpstreamTrafficSetting resource")
		return nil
	}

	// Filter by Host
	for _, setting := range c.kubeController.ListUpstreamTrafficSettings() {
		if setting.Spec.Host == host {
			return setting
		}
	}

	return nil
}

// DetectIngressBackendConflicts detects conflicts between the given IngressBackend resources
func DetectIngressBackendConflicts(x policyv1alpha1.IngressBackend, y policyv1alpha1.IngressBackend) []error {
	var conflicts []error // multiple conflicts could exist

	// Check if the backends conflict
	xSet := mapset.NewSet()
	type setKey struct {
		name string
		port int
	}
	for _, backend := range x.Spec.Backends {
		key := setKey{
			name: backend.Name,
			port: backend.Port.Number,
		}
		xSet.Add(key)
	}
	ySet := mapset.NewSet()
	for _, backend := range y.Spec.Backends {
		key := setKey{
			name: backend.Name,
			port: backend.Port.Number,
		}
		ySet.Add(key)
	}

	duplicates := xSet.Intersect(ySet)
	for b := range duplicates.Iter() {
		err := fmt.Errorf("Backend %s specified in %s and %s conflicts", b.(setKey).name, x.Name, y.Name)
		conflicts = append(conflicts, err)
	}

	return conflicts
}

// GetProxyStatsHeaders returns stats headers for the given proxy.
func (c *client) GetProxyStatsHeaders(p *envoy.Proxy) (map[string]string, error) {
	pod, err := c.kubeController.GetPodForProxy(p)
	if err != nil {
		log.Warn().Str("proxy", p.String()).Msg("Could not find pod for connecting proxy. No metadata was recorded.")
		return nil, err
	}

	workloadKind := "unknown"
	workloadName := "unknown"
	for _, ref := range pod.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			workloadKind = ref.Kind
			workloadName = ref.Name
			// Assume ReplicaSets are controlled by a Deployment unless their names
			// do not contain a hyphen. This aligns with the behavior of the
			// Prometheus config in the OSM Helm chart.
			hyp := strings.LastIndex(workloadName, "-")
			if workloadKind == "ReplicaSet" && hyp >= 0 {
				workloadKind = "Deployment"
				workloadName = workloadName[:hyp]
			}
			break
		}
	}

	return map[string]string{
		"osm-stats-pod":       pod.Name,
		"osm-stats-namespace": pod.Namespace,
		"osm-stats-kind":      workloadKind,
		"osm-stats-name":      workloadName,
	}, nil
}

// VerifyProxy attempts to lookup a pod that matches the given proxy instance by service identity, namespace, and UUID.
func (c *client) VerifyProxy(proxy *envoy.Proxy) error {
	_, err := c.kubeController.GetPodForProxy(proxy)
	return err
}

// ListServiceIdentitiesForService lists ServiceAccounts associated with the given service
func (c *client) ListServiceIdentitiesForService(name, namespace string) ([]identity.ServiceIdentity, error) {
	var identities []identity.ServiceIdentity

	k8sSvc := c.kubeController.GetService(name, namespace)
	if k8sSvc == nil {
		return nil, fmt.Errorf("Error fetching service %s/%s: %s", name, namespace, errServiceNotFound)
	}

	svcAccountsSet := mapset.NewSet()
	pods := c.kubeController.ListPods()
	for _, pod := range pods {
		svcRawSelector := k8sSvc.Spec.Selector
		selector := labels.Set(svcRawSelector).AsSelector()
		// service has no selectors, we do not need to match against the pod label
		if len(svcRawSelector) == 0 {
			continue
		}
		if selector.Matches(labels.Set(pod.Labels)) {
			podSvcAccount := identity.K8sServiceAccount{
				Name:      pod.Spec.ServiceAccountName,
				Namespace: pod.Namespace, // ServiceAccount must belong to the same namespace as the pod
			}
			svcAccountsSet.Add(podSvcAccount)
		}
	}

	for svcAcc := range svcAccountsSet.Iter() {
		identities = append(identities, svcAcc.(identity.K8sServiceAccount).ToServiceIdentity())
	}
	return identities, nil
}

// GetMeshService returns the service.MeshService corresponding to the Port used by clients
// to communicate with it.
func (c *client) GetMeshService(name, namespace string, port uint16) (service.MeshService, error) {
	v1Svc := c.kubeController.GetService(name, namespace)
	if v1Svc == nil {
		return service.MeshService{}, errServiceNotFound
	}
	for _, svc := range c.serviceToMeshServices(*v1Svc) {
		if svc.Port == port {
			return svc, nil
		}
	}
	return service.MeshService{}, fmt.Errorf("service %s/%s does not have a port %d", namespace, name, port)
}

// serviceToMeshServices translates a k8s service with one or more ports to one or more
// MeshService objects per port.
func (c *client) serviceToMeshServices(svc corev1.Service) []service.MeshService {
	var meshServices []service.MeshService

	for _, portSpec := range svc.Spec.Ports {
		meshSvc := service.MeshService{
			Namespace: svc.Namespace,
			Name:      svc.Name,
			Port:      uint16(portSpec.Port),
		}

		// attempt to parse protocol from port name
		// Order of Preference is:
		// 1. port.appProtocol field
		// 2. protocol prefixed to port name (e.g. tcp-my-port)
		// 3. default to http
		protocol := constants.ProtocolHTTP
		for _, p := range constants.SupportedProtocolsInMesh {
			if strings.HasPrefix(portSpec.Name, p+"-") {
				protocol = p
				break
			}
		}

		// use port.appProtocol if specified, else use port protocol
		meshSvc.Protocol = pointer.StringDeref(portSpec.AppProtocol, protocol)

		// The endpoints for the kubernetes service carry information that allows
		// us to retrieve the TargetPort for the MeshService.
		endpoints, _ := c.kubeController.GetEndpoints(svc.Name, svc.Namespace)
		if endpoints != nil {
			meshSvc.TargetPort = GetTargetPortFromEndpoints(portSpec.Name, *endpoints)
		} else {
			log.Warn().Msgf("k8s service %s/%s does not have endpoints but is being represented as a MeshService", svc.Namespace, svc.Name)
		}
		if !IsHeadlessService(svc) || endpoints == nil {
			meshServices = append(meshServices, meshSvc)
			continue
		}

		for _, subset := range endpoints.Subsets {
			for _, address := range subset.Addresses {
				if address.Hostname == "" {
					continue
				}
				meshServices = append(meshServices, service.MeshService{
					Namespace:  svc.Namespace,
					Name:       svc.Name,
					Subdomain:  address.Hostname,
					Port:       meshSvc.Port,
					TargetPort: meshSvc.TargetPort,
					Protocol:   meshSvc.Protocol,
				})
			}
		}
	}
	return meshServices
}

func (c *client) ListNamespaces() ([]string, error) {
	namespaces, err := c.kubeController.ListNamespaces()
	if err != nil {
		return nil, err
	}

	names := make([]string, len(namespaces))

	for i, ns := range namespaces {
		names[i] = ns.Name
	}
	return names, nil
}
