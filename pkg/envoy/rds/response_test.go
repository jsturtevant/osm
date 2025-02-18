package rds

import (
	"testing"
	"time"

	mapset "github.com/deckarep/golang-set"
	xds_route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/uuid"
	access "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha3"
	spec "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha4"
	split "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"github.com/stretchr/testify/assert"
	tassert "github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"

	"github.com/openservicemesh/osm/pkg/catalog"
	catalogFake "github.com/openservicemesh/osm/pkg/catalog/fake"
	tresorFake "github.com/openservicemesh/osm/pkg/certificate/providers/tresor/fake"
	"github.com/openservicemesh/osm/pkg/compute"
	"github.com/openservicemesh/osm/pkg/compute/kube"
	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/endpoint"
	"github.com/openservicemesh/osm/pkg/envoy"
	"github.com/openservicemesh/osm/pkg/identity"
	"github.com/openservicemesh/osm/pkg/k8s"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/openservicemesh/osm/pkg/smi"
	"github.com/openservicemesh/osm/pkg/tests"
	"github.com/openservicemesh/osm/pkg/trafficpolicy"
)

func TestNewResponse(t *testing.T) {
	assert := tassert.New(t)

	testCases := []struct {
		name                     string
		downstreamSA             identity.ServiceIdentity
		upstreamSA               identity.ServiceIdentity
		upstreamServices         []service.MeshService
		meshServices             []service.MeshService
		trafficSpec              spec.HTTPRouteGroup
		trafficSplit             split.TrafficSplit
		ingressInboundPolicies   []*trafficpolicy.InboundTrafficPolicy
		expectedInboundPolicies  map[int][]*trafficpolicy.InboundTrafficPolicy
		expectedOutboundPolicies map[int][]*trafficpolicy.OutboundTrafficPolicy
	}{
		{
			name:             "Test RDS NewResponse",
			downstreamSA:     tests.BookbuyerServiceIdentity,
			upstreamSA:       tests.BookstoreServiceIdentity,
			upstreamServices: []service.MeshService{tests.BookstoreV1Service, tests.BookstoreV2Service},
			meshServices:     []service.MeshService{tests.BookstoreV1Service, tests.BookstoreV2Service, tests.BookstoreApexService},
			trafficSpec: spec.HTTPRouteGroup{
				TypeMeta: v1.TypeMeta{
					APIVersion: "specs.smi-spec.io/v1alpha4",
					Kind:       "HTTPRouteGroup",
				},
				ObjectMeta: v1.ObjectMeta{
					Namespace: tests.Namespace,
					Name:      tests.RouteGroupName,
				},
				Spec: spec.HTTPRouteGroupSpec{
					Matches: []spec.HTTPMatch{
						{
							Name:      tests.BuyBooksMatchName,
							PathRegex: tests.BookstoreBuyPath,
							Methods:   []string{"GET"},
							Headers: map[string]string{
								"user-agent": tests.HTTPUserAgent,
							},
						},
						{
							Name:      tests.SellBooksMatchName,
							PathRegex: tests.BookstoreSellPath,
							Methods:   []string{"GET"},
							Headers: map[string]string{
								"user-agent": tests.HTTPUserAgent,
							},
						},
					},
				},
			},
			trafficSplit: split.TrafficSplit{
				ObjectMeta: v1.ObjectMeta{
					Namespace: tests.Namespace,
				},
				Spec: split.TrafficSplitSpec{
					Service: tests.BookstoreApexServiceName,
					Backends: []split.TrafficSplitBackend{
						{
							Service: tests.BookstoreV1ServiceName,
							Weight:  tests.Weight90,
						},
						{
							Service: tests.BookstoreV2ServiceName,
							Weight:  tests.Weight10,
						},
					},
				},
			},
			ingressInboundPolicies: []*trafficpolicy.InboundTrafficPolicy{
				{
					Name:      "bookstore-v1-default-bookstore-v1.default.svc.cluster.local",
					Hostnames: []string{"bookstore-v1.default.svc.cluster.local"},
					Rules: []*trafficpolicy.Rule{
						{
							Route: trafficpolicy.RouteWeightedClusters{
								HTTPRouteMatch: trafficpolicy.HTTPRouteMatch{
									Path:          tests.BookstoreBuyPath,
									PathMatchType: trafficpolicy.PathMatchRegex,
									Methods:       []string{constants.WildcardHTTPMethod},
								},
								WeightedClusters: mapset.NewSet(tests.BookstoreV1DefaultWeightedCluster),
							},
							AllowedPrincipals: mapset.NewSet(tests.BookstoreServiceAccount.AsPrincipal("cluster.local")),
						},
					},
				},
				{
					Name:      "bookstore-v1.default|*",
					Hostnames: []string{"*"},
					Rules: []*trafficpolicy.Rule{
						{
							Route: trafficpolicy.RouteWeightedClusters{
								HTTPRouteMatch: trafficpolicy.HTTPRouteMatch{
									Path:          tests.BookstoreBuyPath,
									PathMatchType: trafficpolicy.PathMatchRegex,
									Methods:       []string{constants.WildcardHTTPMethod},
								},
								WeightedClusters: mapset.NewSet(tests.BookstoreV1DefaultWeightedCluster),
							},
							AllowedPrincipals: mapset.NewSet(tests.BookstoreServiceAccount.AsPrincipal("cluster.local")),
						},
					},
				},
			},
			expectedInboundPolicies: map[int][]*trafficpolicy.InboundTrafficPolicy{
				8888: {
					{
						Name: "bookstore-v1.default",
						Hostnames: []string{
							"bookstore-v1",
							"bookstore-v1.default",
							"bookstore-v1.default.svc",
							"bookstore-v1.default.svc.cluster",
							"bookstore-v1.default.svc.cluster.local",
							"bookstore-v1:8888",
							"bookstore-v1.default:8888",
							"bookstore-v1.default.svc:8888",
							"bookstore-v1.default.svc.cluster:8888",
							"bookstore-v1.default.svc.cluster.local:8888",
						},
						Rules: []*trafficpolicy.Rule{
							{
								Route: trafficpolicy.RouteWeightedClusters{
									HTTPRouteMatch: tests.BookstoreBuyHTTPRoute,
									WeightedClusters: mapset.NewSet(service.WeightedCluster{
										ClusterName: "default/bookstore-v1",
										Weight:      100,
									}),
								},
								AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
									Name:      tests.BookbuyerServiceAccountName,
									Namespace: tests.Namespace,
								}.AsPrincipal("cluster.local")),
							},
							{
								Route: trafficpolicy.RouteWeightedClusters{
									HTTPRouteMatch: tests.BookstoreSellHTTPRoute,
									WeightedClusters: mapset.NewSet(service.WeightedCluster{
										ClusterName: "default/bookstore-v1",
										Weight:      100,
									}),
								},
								AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
									Name:      tests.BookbuyerServiceAccountName,
									Namespace: tests.Namespace,
								}.AsPrincipal("cluster.local")),
							},
						},
					},
					{
						Name: tests.BookstoreApexServiceName,
						Hostnames: []string{
							"bookstore-apex",
							"bookstore-apex.default",
							"bookstore-apex.default.svc",
							"bookstore-apex.default.svc.cluster",
							"bookstore-apex.default.svc.cluster.local",
							"bookstore-apex:8888",
							"bookstore-apex.default:8888",
							"bookstore-apex.default.svc:8888",
							"bookstore-apex.default.svc.cluster:8888",
							"bookstore-apex.default.svc.cluster.local:8888",
						},
						Rules: []*trafficpolicy.Rule{
							{
								Route: trafficpolicy.RouteWeightedClusters{
									HTTPRouteMatch: tests.BookstoreBuyHTTPRoute,
									WeightedClusters: mapset.NewSet(service.WeightedCluster{
										ClusterName: "default/bookstore-v1",
										Weight:      100,
									}),
								},
								AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
									Name:      tests.BookbuyerServiceAccountName,
									Namespace: tests.Namespace,
								}.AsPrincipal("cluster.local")),
							},
							{
								Route: trafficpolicy.RouteWeightedClusters{
									HTTPRouteMatch: tests.BookstoreSellHTTPRoute,
									WeightedClusters: mapset.NewSet(service.WeightedCluster{
										ClusterName: "default/bookstore-v1",
										Weight:      100,
									}),
								},
								AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
									Name:      tests.BookbuyerServiceAccountName,
									Namespace: tests.Namespace,
								}.AsPrincipal("cluster.local")),
							},
						},
					},
				},
			},
			expectedOutboundPolicies: map[int][]*trafficpolicy.OutboundTrafficPolicy{
				80: {
					{
						Name:      tests.BookstoreApexServiceName,
						Hostnames: tests.BookstoreApexHostnames,
						Routes: []*trafficpolicy.RouteWeightedClusters{
							{
								HTTPRouteMatch: tests.WildCardRouteMatch,
								WeightedClusters: mapset.NewSetFromSlice([]interface{}{
									service.WeightedCluster{ClusterName: "default/bookstore-v1|80", Weight: 0},
									service.WeightedCluster{ClusterName: "default/bookstore-v2|80", Weight: 100},
								}),
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockKubeController := k8s.NewMockController(mockCtrl)
			mockMeshSpec := smi.NewMockMeshSpec(mockCtrl)
			mockEndpointProvider := endpoint.NewMockProvider(mockCtrl)
			mockCatalog := catalog.NewMockMeshCataloger(mockCtrl)
			kubeClient := testclient.NewSimpleClientset()
			proxy, err := getBookstoreV1Proxy(kubeClient)
			assert.Nil(err)

			for _, meshSvc := range tc.meshServices {
				k8sService := tests.NewServiceFixture(meshSvc.Name, meshSvc.Namespace, map[string]string{})
				mockKubeController.EXPECT().GetService(meshSvc.Name, meshSvc.Namespace).Return(k8sService).AnyTimes()
			}

			mockEndpointProvider.EXPECT().GetID().Return("fake").AnyTimes()

			mockMeshSpec.EXPECT().ListHTTPTrafficSpecs().Return([]*spec.HTTPRouteGroup{&tc.trafficSpec}).AnyTimes()
			mockMeshSpec.EXPECT().ListTrafficSplits().Return([]*split.TrafficSplit{&tc.trafficSplit}).AnyTimes()
			trafficTarget := tests.NewSMITrafficTarget(tc.downstreamSA, tc.upstreamSA)
			mockMeshSpec.EXPECT().ListTrafficTargets().Return([]*access.TrafficTarget{&trafficTarget}).AnyTimes()

			mockCatalog.EXPECT().GetMeshConfig().AnyTimes()
			mockCatalog.EXPECT().GetInboundMeshTrafficPolicy(gomock.Any(), gomock.Any()).Return(&trafficpolicy.InboundMeshTrafficPolicy{HTTPRouteConfigsPerPort: tc.expectedInboundPolicies}).AnyTimes()
			mockCatalog.EXPECT().GetOutboundMeshTrafficPolicy(gomock.Any()).Return(&trafficpolicy.OutboundMeshTrafficPolicy{HTTPRouteConfigsPerPort: tc.expectedOutboundPolicies}).AnyTimes()
			mockCatalog.EXPECT().GetIngressTrafficPolicy(gomock.Any()).Return(&trafficpolicy.IngressTrafficPolicy{HTTPRoutePolicies: tc.ingressInboundPolicies}, nil).AnyTimes()
			mockCatalog.EXPECT().GetEgressTrafficPolicy(gomock.Any()).Return(nil, nil).AnyTimes()
			mockCatalog.EXPECT().ListServicesForProxy(proxy).Return([]service.MeshService{tests.BookstoreV1Service}, nil).AnyTimes()
			// Empty discovery request
			mc := tresorFake.NewFake(1 * time.Hour)

			resources, err := NewResponse(mockCatalog, proxy, mc, nil)
			assert.Nil(err)
			assert.NotNil(resources)

			// The RDS response will have two route configurations
			// 1. rds-inbound
			// 2. rds-outbound
			// 3. rds-ingress
			assert.Equal(3, len(resources))

			// Check the inbound route configuration
			routeConfig, ok := resources[0].(*xds_route.RouteConfiguration)
			assert.True(ok)

			// The rds-inbound will have the following virtual hosts :
			// inbound_virtual-host|bookstore-v1.default
			// inbound_virtual-host|bookstore-apex
			assert.Equal("rds-inbound.8888", routeConfig.Name)
			assert.Equal(2, len(routeConfig.VirtualHosts))

			assert.Equal("inbound_virtual-host|bookstore-v1.default", routeConfig.VirtualHosts[0].Name)
			assert.Equal(tests.BookstoreV1Hostnames, routeConfig.VirtualHosts[0].Domains)
			assert.Equal(2, len(routeConfig.VirtualHosts[0].Routes))
			assert.Equal(tests.BookstoreBuyHTTPRoute.Path, routeConfig.VirtualHosts[0].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})
			assert.Equal(tests.BookstoreSellHTTPRoute.Path, routeConfig.VirtualHosts[0].Routes[1].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes[1].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[1].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})

			assert.Equal("inbound_virtual-host|bookstore-apex", routeConfig.VirtualHosts[1].Name)
			assert.Equal(tests.BookstoreApexHostnames, routeConfig.VirtualHosts[1].Domains)
			assert.Equal(2, len(routeConfig.VirtualHosts[1].Routes))
			assert.Equal(tests.BookstoreBuyHTTPRoute.Path, routeConfig.VirtualHosts[1].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[1].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[1].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})
			assert.Equal(tests.BookstoreSellHTTPRoute.Path, routeConfig.VirtualHosts[1].Routes[1].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[1].Routes[1].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[1].Routes[1].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})

			// Check the outbound route configuration
			routeConfig, ok = resources[1].(*xds_route.RouteConfiguration)
			assert.True(ok)

			// The rds-outbound will have the following virtual hosts :
			// outbound_virtual-host|bookstore-apex
			assert.Equal("rds-outbound.80", routeConfig.Name)
			assert.Equal(1, len(routeConfig.VirtualHosts))

			assert.Equal("outbound_virtual-host|bookstore-apex", routeConfig.VirtualHosts[0].Name)
			assert.Equal(tests.BookstoreApexHostnames, routeConfig.VirtualHosts[0].Domains)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes))
			assert.Equal(tests.WildCardRouteMatch.Path, routeConfig.VirtualHosts[0].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(2, len(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})

			// Check the ingress route configuration
			routeConfig, ok = resources[2].(*xds_route.RouteConfiguration)
			assert.True(ok)

			// ingress_virtual-host|bookstore-v1-default-bookstore-v1.default.svc.cluster.local
			assert.Equal("ingress_virtual-host|bookstore-v1-default-bookstore-v1.default.svc.cluster.local", routeConfig.VirtualHosts[0].Name)
			assert.Equal([]string{"bookstore-v1.default.svc.cluster.local"}, routeConfig.VirtualHosts[0].Domains)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes))
			assert.Equal(tests.BookstoreBuyHTTPRoute.Path, routeConfig.VirtualHosts[0].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})

			// inbound_virtual-host|bookstore-v1.default|*
			assert.Equal("ingress_virtual-host|bookstore-v1.default|*", routeConfig.VirtualHosts[1].Name)
			assert.Equal([]string{"*"}, routeConfig.VirtualHosts[1].Domains)
			assert.Equal(1, len(routeConfig.VirtualHosts[1].Routes))
			assert.Equal(tests.BookstoreBuyHTTPRoute.Path, routeConfig.VirtualHosts[1].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[1].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[1].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})
		})
	}
}

func TestRDSNewResponseWithTrafficSplit(t *testing.T) {
	a := assert.New(t)

	// ---[  Setup the test context  ]---------
	kubeClient := testclient.NewSimpleClientset()
	mockCtrl := gomock.NewController(t)
	services := []service.MeshService{tests.BookstoreApexService, tests.BookstoreV1Service, tests.BookstoreV2Service}
	provider := compute.NewMockInterface(mockCtrl)
	provider.EXPECT().GetMeshConfig().AnyTimes()
	provider.EXPECT().GetServicesForServiceIdentity(gomock.Any()).Return(services).AnyTimes()
	provider.EXPECT().GetResolvableEndpointsForService(gomock.Any()).Return([]endpoint.Endpoint{tests.Endpoint}).AnyTimes()
	provider.EXPECT().GetMeshService(tests.BookstoreApexService.Name, tests.BookstoreApexService.Namespace, tests.BookstoreApexService.Port).Return(tests.BookstoreApexService, nil).AnyTimes()
	provider.EXPECT().GetMeshService(tests.BookstoreV1Service.Name, tests.BookstoreV1Service.Namespace, tests.BookstoreV1Service.Port).Return(tests.BookstoreV1Service, nil).AnyTimes()
	provider.EXPECT().GetMeshService(tests.BookstoreV2Service.Name, tests.BookstoreV2Service.Namespace, tests.BookstoreV2Service.Port).Return(tests.BookstoreV2Service, nil).AnyTimes()
	provider.EXPECT().ListServicesForProxy(gomock.Any()).Return(nil, nil).AnyTimes()

	provider.EXPECT().ListEgressPoliciesForServiceAccount(gomock.Any()).Return(nil).AnyTimes()
	provider.EXPECT().GetIngressBackendPolicyForService(gomock.Any()).Return(nil).AnyTimes()
	provider.EXPECT().GetUpstreamTrafficSettingByService(gomock.Any()).Return(nil).AnyTimes()
	provider.EXPECT().GetUpstreamTrafficSettingByNamespace(gomock.Any()).Return(nil).AnyTimes()
	for _, svc := range services {
		provider.EXPECT().GetHostnamesForService(svc, true).Return(kube.NewClient(nil).GetHostnamesForService(svc, true)).AnyTimes()
	}

	meshCatalog := catalogFake.NewFakeMeshCatalog(provider)

	proxy, err := getSidecarProxy(kubeClient, uuid.MustParse(tests.ProxyUUID), identity.New(tests.BookbuyerServiceAccountName, tests.Namespace))
	a.Nil(err)
	a.NotNil(proxy)

	mc := tresorFake.NewFake(1 * time.Hour)
	a.NotNil(a)

	resources, err := NewResponse(meshCatalog, proxy, mc, nil)
	a.Nil(err)
	a.Len(resources, 1) // only outbound routes configured for this test

	// ---[  Prepare the config for testing  ]-------
	// Order matters. In this test, we do not expect rds-inbound route configuration, and rds-outbound is expected
	// to be configured per outbound port.
	routeCfg, ok := resources[0].(*xds_route.RouteConfiguration)
	a.True(ok)
	a.Equal("rds-outbound.8888", routeCfg.Name)

	const (
		apexName = "outbound_virtual-host|bookstore-apex.default.svc.cluster.local"
		v1Name   = "outbound_virtual-host|bookstore-v1.default.svc.cluster.local"
		v2Name   = "outbound_virtual-host|bookstore-v2.default.svc.cluster.local"
	)
	expectedVHostNames := []string{apexName, v1Name, v2Name}

	// ---[  Compare with expectations  ]-------
	// Expect an XDS Route Configuration with 3 outbound virtual hosts"
	var actualNames []string
	for _, vHost := range routeCfg.VirtualHosts {
		actualNames = append(actualNames, vHost.Name)
	}
	a.Len(routeCfg.VirtualHosts, len(expectedVHostNames))
	a.ElementsMatch(expectedVHostNames, actualNames)

	// Get the 3 VirtualHost configurations into variables so it is easier to
	// test them (they are stored in a slice w/ non-deterministic order)
	var apex, v1, v2 *xds_route.VirtualHost
	for _, virtualHost := range routeCfg.VirtualHosts {
		switch virtualHost.Name {
		case apexName:
			apex = virtualHost
		case v1Name:
			v1 = virtualHost
		case v2Name:
			v2 = virtualHost
		}
	}

	testCases := []struct {
		name                    string
		virtualHost             *xds_route.VirtualHost
		expectedDomains         []string
		expectedWeightedCluster *xds_route.WeightedCluster
	}{
		{
			name:        "bookstore-v1",
			virtualHost: v1,
			expectedDomains: []string{
				"bookstore-v1",
				"bookstore-v1:8888",
				"bookstore-v1.default",
				"bookstore-v1.default.svc",
				"bookstore-v1.default:8888",
				"bookstore-v1.default.svc:8888",
				"bookstore-v1.default.svc.cluster",
				"bookstore-v1.default.svc.cluster:8888",
				"bookstore-v1.default.svc.cluster.local",
				"bookstore-v1.default.svc.cluster.local:8888",
			},
			expectedWeightedCluster: &xds_route.WeightedCluster{
				Clusters: []*xds_route.WeightedCluster_ClusterWeight{
					{
						Name: "default/bookstore-v1|8888",
						Weight: &wrappers.UInt32Value{
							Value: 100,
						},
					},
				},
				TotalWeight: &wrappers.UInt32Value{
					Value: 100,
				},
			},
		},
		{
			name:        "bookstore-v2",
			virtualHost: v2,
			expectedDomains: []string{
				"bookstore-v2",
				"bookstore-v2:8888",
				"bookstore-v2.default",
				"bookstore-v2.default.svc",
				"bookstore-v2.default:8888",
				"bookstore-v2.default.svc:8888",
				"bookstore-v2.default.svc.cluster",
				"bookstore-v2.default.svc.cluster:8888",
				"bookstore-v2.default.svc.cluster.local",
				"bookstore-v2.default.svc.cluster.local:8888",
			},
			expectedWeightedCluster: &xds_route.WeightedCluster{
				Clusters: []*xds_route.WeightedCluster_ClusterWeight{
					{
						Name: "default/bookstore-v2|8888",
						Weight: &wrappers.UInt32Value{
							Value: 100,
						},
					},
				},
				TotalWeight: &wrappers.UInt32Value{
					Value: 100,
				},
			},
		},
		{
			name:        "bookstore-apex",
			virtualHost: apex,
			expectedDomains: []string{
				"bookstore-apex",
				"bookstore-apex:8888",
				"bookstore-apex.default",
				"bookstore-apex.default.svc",
				"bookstore-apex.default:8888",
				"bookstore-apex.default.svc:8888",
				"bookstore-apex.default.svc.cluster",
				"bookstore-apex.default.svc.cluster:8888",
				"bookstore-apex.default.svc.cluster.local",
				"bookstore-apex.default.svc.cluster.local:8888",
			},
			expectedWeightedCluster: &xds_route.WeightedCluster{
				Clusters: []*xds_route.WeightedCluster_ClusterWeight{
					{
						Name: "default/bookstore-v1|8888",
						Weight: &wrappers.UInt32Value{
							Value: 90,
						},
					},
					{
						Name: "default/bookstore-v2|8888",
						Weight: &wrappers.UInt32Value{
							Value: 10,
						},
					},
				},
				TotalWeight: &wrappers.UInt32Value{
					Value: 100,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a.ElementsMatch(tc.expectedDomains, tc.virtualHost.Domains)
			a.Len(tc.virtualHost.Routes, 1)
			a.ElementsMatch(tc.expectedWeightedCluster.Clusters, tc.virtualHost.Routes[0].GetRoute().GetWeightedClusters().Clusters)
			a.Equal(tc.expectedWeightedCluster.TotalWeight, tc.virtualHost.Routes[0].GetRoute().GetWeightedClusters().TotalWeight)
		})
	}
}

func TestRDSRespose(t *testing.T) {
	assert := tassert.New(t)

	testCases := []struct {
		name                     string
		downstreamSA             identity.ServiceIdentity
		upstreamSA               identity.ServiceIdentity
		upstreamServices         []service.MeshService
		meshServices             []service.MeshService
		trafficSpec              spec.HTTPRouteGroup
		trafficSplit             split.TrafficSplit
		expectedInboundPolicies  []*trafficpolicy.InboundTrafficPolicy
		expectedOutboundPolicies []*trafficpolicy.OutboundTrafficPolicy
	}{
		{
			name:             "Test RDS response with a traffic split having zero weight",
			downstreamSA:     tests.BookbuyerServiceIdentity,
			upstreamSA:       tests.BookstoreServiceIdentity,
			upstreamServices: []service.MeshService{tests.BookstoreV1Service, tests.BookstoreV2Service},
			meshServices:     []service.MeshService{tests.BookstoreV1Service, tests.BookstoreV2Service, tests.BookstoreApexService},
			trafficSpec: spec.HTTPRouteGroup{
				TypeMeta: v1.TypeMeta{
					APIVersion: "specs.smi-spec.io/v1alpha4",
					Kind:       "HTTPRouteGroup",
				},
				ObjectMeta: v1.ObjectMeta{
					Namespace: tests.Namespace,
					Name:      tests.RouteGroupName,
				},
				Spec: spec.HTTPRouteGroupSpec{
					Matches: []spec.HTTPMatch{
						{
							Name:      tests.BuyBooksMatchName,
							PathRegex: tests.BookstoreBuyPath,
							Methods:   []string{"GET"},
							Headers: map[string]string{
								"user-agent": tests.HTTPUserAgent,
							},
						},
						{
							Name:      tests.SellBooksMatchName,
							PathRegex: tests.BookstoreSellPath,
							Methods:   []string{"GET"},
							Headers: map[string]string{
								"user-agent": tests.HTTPUserAgent,
							},
						},
					},
				},
			},
			trafficSplit: split.TrafficSplit{
				ObjectMeta: v1.ObjectMeta{
					Namespace: tests.Namespace,
				},
				Spec: split.TrafficSplitSpec{
					Service: tests.BookstoreApexServiceName,
					Backends: []split.TrafficSplitBackend{
						{
							Service: tests.BookstoreV1ServiceName,
							Weight:  0,
						},
						{
							Service: tests.BookstoreV2ServiceName,
							Weight:  100,
						},
					},
				},
			},
			expectedInboundPolicies: []*trafficpolicy.InboundTrafficPolicy{
				{
					Name: "bookstore-v1.default.svc.cluster.local",
					Hostnames: []string{
						"bookstore-v1",
						"bookstore-v1.default",
						"bookstore-v1.default.svc",
						"bookstore-v1.default.svc.cluster",
						"bookstore-v1.default.svc.cluster.local",
						"bookstore-v1:8888",
						"bookstore-v1.default:8888",
						"bookstore-v1.default.svc:8888",
						"bookstore-v1.default.svc.cluster:8888",
						"bookstore-v1.default.svc.cluster.local:8888",
					},
					Rules: []*trafficpolicy.Rule{
						{
							Route: trafficpolicy.RouteWeightedClusters{
								HTTPRouteMatch: tests.BookstoreBuyHTTPRoute,
								WeightedClusters: mapset.NewSet(service.WeightedCluster{
									ClusterName: "default/bookstore-v1|8888",
									Weight:      100,
								}),
							},
							AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
								Name:      tests.BookbuyerServiceAccountName,
								Namespace: tests.Namespace,
							}.AsPrincipal("cluster.local")),
						},
						{
							Route: trafficpolicy.RouteWeightedClusters{
								HTTPRouteMatch: tests.BookstoreSellHTTPRoute,
								WeightedClusters: mapset.NewSet(service.WeightedCluster{
									ClusterName: "default/bookstore-v1|8888",
									Weight:      100,
								}),
							},
							AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
								Name:      tests.BookbuyerServiceAccountName,
								Namespace: tests.Namespace,
							}.AsPrincipal("cluster.local")),
						},
					},
				},
				{
					Name: tests.BookstoreApexServiceName + ".default.svc.cluster.local",
					Hostnames: []string{
						"bookstore-apex",
						"bookstore-apex.default",
						"bookstore-apex.default.svc",
						"bookstore-apex.default.svc.cluster",
						"bookstore-apex.default.svc.cluster.local",
						"bookstore-apex:8888",
						"bookstore-apex.default:8888",
						"bookstore-apex.default.svc:8888",
						"bookstore-apex.default.svc.cluster:8888",
						"bookstore-apex.default.svc.cluster.local:8888",
					},
					Rules: []*trafficpolicy.Rule{
						{
							Route: trafficpolicy.RouteWeightedClusters{
								HTTPRouteMatch: tests.BookstoreBuyHTTPRoute,
								WeightedClusters: mapset.NewSet(service.WeightedCluster{
									ClusterName: "default/bookstore-v1|8888",
									Weight:      100,
								}),
							},
							AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
								Name:      tests.BookbuyerServiceAccountName,
								Namespace: tests.Namespace,
							}.AsPrincipal("cluster.local")),
						},
						{
							Route: trafficpolicy.RouteWeightedClusters{
								HTTPRouteMatch: tests.BookstoreSellHTTPRoute,
								WeightedClusters: mapset.NewSet(service.WeightedCluster{
									ClusterName: "default/bookstore-v1|8888",
									Weight:      100,
								}),
							},
							AllowedPrincipals: mapset.NewSet(identity.K8sServiceAccount{
								Name:      tests.BookbuyerServiceAccountName,
								Namespace: tests.Namespace,
							}.AsPrincipal("cluster.local")),
						},
					},
				},
			},
			expectedOutboundPolicies: []*trafficpolicy.OutboundTrafficPolicy{
				{
					Name:      tests.BookstoreApexServiceName + ".default.svc.cluster.local",
					Hostnames: tests.BookstoreApexHostnames,
					Routes: []*trafficpolicy.RouteWeightedClusters{
						{
							HTTPRouteMatch: tests.WildCardRouteMatch,
							WeightedClusters: mapset.NewSetFromSlice([]interface{}{
								service.WeightedCluster{ClusterName: "default/bookstore-v1|8888", Weight: 0},
								service.WeightedCluster{ClusterName: "default/bookstore-v2|8888", Weight: 100},
							}),
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockKubeController := k8s.NewMockController(mockCtrl)
			mockMeshSpec := smi.NewMockMeshSpec(mockCtrl)
			mockEndpointProvider := endpoint.NewMockProvider(mockCtrl)
			mockCatalog := catalog.NewMockMeshCataloger(mockCtrl)
			kubeClient := testclient.NewSimpleClientset()
			proxy, err := getBookstoreV1Proxy(kubeClient)
			assert.Nil(err)

			for _, meshSvc := range tc.meshServices {
				k8sService := tests.NewServiceFixture(meshSvc.Name, meshSvc.Namespace, map[string]string{})
				mockKubeController.EXPECT().GetService(meshSvc.Name, meshSvc.Namespace).Return(k8sService).AnyTimes()
			}

			mockEndpointProvider.EXPECT().GetID().Return("fake").AnyTimes()

			mockMeshSpec.EXPECT().ListHTTPTrafficSpecs().Return([]*spec.HTTPRouteGroup{&tc.trafficSpec}).AnyTimes()
			mockMeshSpec.EXPECT().ListTrafficSplits().Return([]*split.TrafficSplit{&tc.trafficSplit}).AnyTimes()
			trafficTarget := tests.NewSMITrafficTarget(tc.downstreamSA, tc.upstreamSA)
			mockMeshSpec.EXPECT().ListTrafficTargets().Return([]*access.TrafficTarget{&trafficTarget}).AnyTimes()

			outboundTestPort := 8888 // Port used for the outbound services in this test
			inboundTestPort := 80    // Port used for the inbound services in this test
			expectedInboundMeshPolicy := &trafficpolicy.InboundMeshTrafficPolicy{HTTPRouteConfigsPerPort: map[int][]*trafficpolicy.InboundTrafficPolicy{inboundTestPort: tc.expectedInboundPolicies}}
			expectedOutboundMeshPolicy := &trafficpolicy.OutboundMeshTrafficPolicy{HTTPRouteConfigsPerPort: map[int][]*trafficpolicy.OutboundTrafficPolicy{outboundTestPort: tc.expectedOutboundPolicies}}
			mockCatalog.EXPECT().GetInboundMeshTrafficPolicy(gomock.Any(), gomock.Any()).Return(expectedInboundMeshPolicy).AnyTimes()
			mockCatalog.EXPECT().GetOutboundMeshTrafficPolicy(gomock.Any()).Return(expectedOutboundMeshPolicy).AnyTimes()
			mockCatalog.EXPECT().GetIngressTrafficPolicy(gomock.Any()).Return(nil, nil).AnyTimes()
			mockCatalog.EXPECT().GetEgressTrafficPolicy(gomock.Any()).Return(nil, nil).AnyTimes()
			mockCatalog.EXPECT().GetMeshConfig().AnyTimes()
			mockCatalog.EXPECT().ListServicesForProxy(proxy).Return(nil, nil).AnyTimes()

			cm := tresorFake.NewFake(1 * time.Hour)

			resources, err := NewResponse(mockCatalog, proxy, cm, nil)
			assert.Nil(err)
			assert.NotNil(resources)

			// The RDS response will have two route configurations
			// 1. rds-inbound
			// 2. rds-outbound
			assert.Equal(2, len(resources))

			// Check the inbound route configuration
			routeConfig, ok := resources[0].(*xds_route.RouteConfiguration)
			assert.True(ok)

			// The rds-inbound will have the following virtual hosts :
			// inbound_virtual-host|bookstore-v1.default
			// inbound_virtual-host|bookstore-apex
			assert.Equal("rds-inbound.80", routeConfig.Name)
			assert.Equal(2, len(routeConfig.VirtualHosts))

			assert.Equal("inbound_virtual-host|bookstore-v1.default.svc.cluster.local", routeConfig.VirtualHosts[0].Name)
			assert.Equal(tests.BookstoreV1Hostnames, routeConfig.VirtualHosts[0].Domains)
			assert.Equal(2, len(routeConfig.VirtualHosts[0].Routes))
			assert.Equal(tests.BookstoreBuyHTTPRoute.Path, routeConfig.VirtualHosts[0].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})
			assert.Equal(tests.BookstoreSellHTTPRoute.Path, routeConfig.VirtualHosts[0].Routes[1].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes[1].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[1].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})

			assert.Equal("inbound_virtual-host|bookstore-apex.default.svc.cluster.local", routeConfig.VirtualHosts[1].Name)
			assert.Equal(tests.BookstoreApexHostnames, routeConfig.VirtualHosts[1].Domains)
			assert.Equal(2, len(routeConfig.VirtualHosts[1].Routes))
			assert.Equal(tests.BookstoreBuyHTTPRoute.Path, routeConfig.VirtualHosts[1].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[1].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[1].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})
			assert.Equal(tests.BookstoreSellHTTPRoute.Path, routeConfig.VirtualHosts[1].Routes[1].GetMatch().GetSafeRegex().Regex)
			assert.Equal(1, len(routeConfig.VirtualHosts[1].Routes[1].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[1].Routes[1].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})

			// Check the outbound route configuration
			routeConfig, ok = resources[1].(*xds_route.RouteConfiguration)
			assert.True(ok)

			// The rds-outbound will have the following virtual hosts :
			// outbound_virtual-host|bookstore-apex
			assert.Equal("rds-outbound.8888", routeConfig.Name)
			assert.Equal(1, len(routeConfig.VirtualHosts))

			assert.Equal("outbound_virtual-host|bookstore-apex.default.svc.cluster.local", routeConfig.VirtualHosts[0].Name)
			assert.Equal(tests.BookstoreApexHostnames, routeConfig.VirtualHosts[0].Domains)
			assert.Equal(1, len(routeConfig.VirtualHosts[0].Routes))
			assert.Equal(tests.WildCardRouteMatch.Path, routeConfig.VirtualHosts[0].Routes[0].GetMatch().GetSafeRegex().Regex)
			assert.Equal(2, len(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters))
			assert.Equal(routeConfig.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().TotalWeight, &wrappers.UInt32Value{Value: uint32(100)})
		})
	}
}

func getBookstoreV1Proxy(kubeClient kubernetes.Interface) (*envoy.Proxy, error) {
	// Create pod for bookbuyer
	bookbuyerPodLabels := map[string]string{
		constants.AppLabel:               tests.BookbuyerService.Name,
		constants.EnvoyUniqueIDLabelName: uuid.New().String(),
	}
	if _, err := tests.MakePod(kubeClient, tests.Namespace, tests.BookbuyerServiceName, tests.BookbuyerServiceAccountName, bookbuyerPodLabels); err != nil {
		return nil, err
	}

	// Create pod for bookstore-v1
	bookstoreV1PodLabels := map[string]string{
		constants.AppLabel:               tests.BookstoreV1ServiceName,
		constants.EnvoyUniqueIDLabelName: tests.ProxyUUID,
	}
	if _, err := tests.MakePod(kubeClient, tests.Namespace, tests.BookstoreV1ServiceName, tests.BookstoreServiceAccountName, bookstoreV1PodLabels); err != nil {
		return nil, err
	}

	// Create a pod for bookstore-v2
	bookstoreV2PodLabels := map[string]string{
		constants.AppLabel:               tests.BookstoreV1ServiceName,
		constants.EnvoyUniqueIDLabelName: uuid.New().String(),
	}
	if _, err := tests.MakePod(kubeClient, tests.Namespace, tests.BookstoreV2ServiceName, tests.BookstoreServiceAccountName, bookstoreV2PodLabels); err != nil {
		return nil, err
	}

	// Create service for bookstore-v1 and bookstore-v2
	for _, svcName := range []string{tests.BookstoreV1ServiceName, tests.BookstoreV2ServiceName} {
		selectors := map[string]string{
			constants.AppLabel: svcName,
		}
		if _, err := tests.MakeService(kubeClient, svcName, selectors); err != nil {
			return nil, err
		}
	}

	// Create service for traffic split apex
	for _, svcName := range []string{tests.BookstoreApexServiceName} {
		selectors := map[string]string{
			constants.AppLabel: "bookstore",
		}
		if _, err := tests.MakeService(kubeClient, svcName, selectors); err != nil {
			return nil, err
		}
	}

	return envoy.NewProxy(envoy.KindSidecar, uuid.MustParse(tests.ProxyUUID), tests.BookstoreServiceIdentity, nil, 1), nil
}

func getSidecarProxy(kubeClient kubernetes.Interface, proxyUUID uuid.UUID, svcIdentity identity.ServiceIdentity) (*envoy.Proxy, error) {
	bookbuyerPodLabels := map[string]string{
		constants.AppLabel:               tests.BookbuyerService.Name,
		constants.EnvoyUniqueIDLabelName: tests.ProxyUUID,
	}
	if _, err := tests.MakePod(kubeClient, tests.Namespace, tests.BookbuyerServiceName, tests.BookbuyerServiceAccountName, bookbuyerPodLabels); err != nil {
		return nil, err
	}

	bookstorePodLabels := map[string]string{
		constants.AppLabel:               "bookstore",
		constants.EnvoyUniqueIDLabelName: uuid.New().String(),
	}
	if _, err := tests.MakePod(kubeClient, tests.Namespace, "bookstore", tests.BookstoreServiceAccountName, bookstorePodLabels); err != nil {
		return nil, err
	}

	selectors := map[string]string{
		constants.AppLabel: tests.BookbuyerServiceName,
	}
	if _, err := tests.MakeService(kubeClient, tests.BookbuyerServiceName, selectors); err != nil {
		return nil, err
	}

	for _, svcName := range []string{tests.BookstoreApexServiceName, tests.BookstoreV1ServiceName, tests.BookstoreV2ServiceName} {
		selectors := map[string]string{
			constants.AppLabel: "bookstore",
		}
		if _, err := tests.MakeService(kubeClient, svcName, selectors); err != nil {
			return nil, err
		}
	}

	return envoy.NewProxy(envoy.KindSidecar, proxyUUID, svcIdentity, nil, 1), nil
}
