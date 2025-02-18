package k8s

import (
	"fmt"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	tassert "github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	testclient "k8s.io/client-go/kubernetes/fake"

	configv1alpha2 "github.com/openservicemesh/osm/pkg/apis/config/v1alpha2"
	policyv1alpha1 "github.com/openservicemesh/osm/pkg/apis/policy/v1alpha1"
	"github.com/openservicemesh/osm/pkg/envoy"
	fakeConfig "github.com/openservicemesh/osm/pkg/gen/client/config/clientset/versioned/fake"
	fakePolicyClient "github.com/openservicemesh/osm/pkg/gen/client/policy/clientset/versioned/fake"
	"github.com/openservicemesh/osm/pkg/identity"
	"github.com/openservicemesh/osm/pkg/messaging"
	"github.com/openservicemesh/osm/pkg/metricsstore"
	"github.com/openservicemesh/osm/pkg/tests"

	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/k8s/informers"
)

var (
	testMeshName = "mesh"
	testNs       = "test-ns"
)

func TestIsMonitoredNamespace(t *testing.T) {
	testCases := []struct {
		name      string
		namespace *corev1.Namespace
		ns        string
		expected  bool
	}{
		{
			name: "namespace is monitored if is found in the namespace cache",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			ns:       "foo",
			expected: true,
		},
		{
			name: "namespace is not monitored if is not in the namespace cache",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			ns:       "invalid",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)

			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyNamespace, tc.namespace, t)

			actual := c.IsMonitoredNamespace(tc.ns)
			a.Equal(tc.expected, actual)
		})
	}
}

func TestGetNamespace(t *testing.T) {
	testCases := []struct {
		name      string
		namespace *corev1.Namespace
		ns        string
		expected  bool
	}{
		{
			name: "gets the namespace from the cache given its key",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			ns:       "foo",
			expected: true,
		},
		{
			name: "returns nil if the namespace is not found in the cache",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			ns:       "invalid",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyNamespace, tc.namespace, t)

			actual := c.GetNamespace(tc.ns)
			if tc.expected {
				a.Equal(tc.namespace, actual)
			} else {
				a.Nil(actual)
			}
		})
	}
}

func TestListNamespaces(t *testing.T) {
	testCases := []struct {
		name       string
		namespaces []*corev1.Namespace
		expected   []string
	}{
		{
			name: "gets the namespace from the cache given its key",
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns2",
					},
				},
			},
			expected: []string{"ns1", "ns2"},
		},
		{
			name:       "gets the namespace from the cache given its key",
			namespaces: nil,
			expected:   []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			for _, ns := range tc.namespaces {
				_ = ic.Add(informers.InformerKeyNamespace, ns, t)
			}

			actual, err := c.ListNamespaces()
			a.Nil(err)
			names := make([]string, 0, len(actual))
			for _, ns := range actual {
				names = append(names, ns.Name)
			}
			a.ElementsMatch(tc.expected, names)
		})
	}
}

func TestGetService(t *testing.T) {
	testCases := []struct {
		name         string
		service      *corev1.Service
		svcName      string
		svcNamespace string
		expected     bool
	}{
		{
			name: "gets the service from the cache given its key",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns1",
				},
			},
			svcName:      "foo",
			svcNamespace: "ns1",
			expected:     true,
		},
		{
			name: "returns nil if the service is not found in the cache",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns1",
				},
			},
			svcName:      "invalid",
			svcNamespace: "ns1",
			expected:     false,
		},
		{
			name: "gets the headless service from the cache from a subdomained MeshService",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo-headless",
					Namespace: "ns1",
				},
			},
			svcName:      "foo-headless",
			svcNamespace: "ns1",
			expected:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyService, tc.service, t)

			actual := c.GetService(tc.svcName, tc.svcNamespace)
			if tc.expected {
				a.Equal(tc.service, actual)
			} else {
				a.Nil(actual)
			}
		})
	}
}

func TestListServices(t *testing.T) {
	testCases := []struct {
		name      string
		namespace *corev1.Namespace
		services  []*corev1.Service
		expected  []*corev1.Service
	}{
		{
			name: "gets the k8s services if their namespaces are monitored",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns2",
						Name:      "s2",
					},
				},
			},
			expected: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyNamespace, tc.namespace, t)

			for _, s := range tc.services {
				_ = ic.Add(informers.InformerKeyService, s, t)
			}

			actual := c.ListServices()
			a.ElementsMatch(tc.expected, actual)
		})
	}
}

func TestListServiceAccounts(t *testing.T) {
	testCases := []struct {
		name      string
		namespace *corev1.Namespace
		sa        []*corev1.ServiceAccount
		expected  []*corev1.ServiceAccount
	}{
		{
			name: "gets the k8s service accounts if their namespaces are monitored",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
				},
			},
			sa: []*corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns2",
						Name:      "s2",
					},
				},
			},
			expected: []*corev1.ServiceAccount{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyNamespace, tc.namespace, t)

			for _, s := range tc.sa {
				_ = ic.Add(informers.InformerKeyServiceAccount, s, t)
			}

			actual := c.ListServiceAccounts()
			a.ElementsMatch(tc.expected, actual)
		})
	}
}

func TestListPods(t *testing.T) {
	testCases := []struct {
		name      string
		namespace *corev1.Namespace
		pods      []*corev1.Pod
		expected  []*corev1.Pod
	}{
		{
			name: "gets the k8s pods if their namespaces are monitored",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ns1",
				},
			},
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns2",
						Name:      "s2",
					},
				},
			},
			expected: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "s1",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyNamespace, tc.namespace, t)

			for _, p := range tc.pods {
				_ = ic.Add(informers.InformerKeyPod, p, t)
			}

			actual := c.ListPods()
			a.ElementsMatch(tc.expected, actual)
		})
	}
}

func TestGetEndpoints(t *testing.T) {
	testCases := []struct {
		name         string
		endpoints    *corev1.Endpoints
		svcName      string
		svcNamespace string
		expected     *corev1.Endpoints
	}{
		{
			name: "gets the service from the cache given its key",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns1",
				},
			},
			svcName:      "foo",
			svcNamespace: "ns1",
			expected: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns1",
				},
			},
		},
		{
			name: "returns nil if the service is not found in the cache",
			endpoints: &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "ns1",
				},
			},
			svcName:      "invalid",
			svcNamespace: "ns1",
			expected:     nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, nil, nil)
			_ = ic.Add(informers.InformerKeyEndpoints, tc.endpoints, t)

			actual, err := c.GetEndpoints(tc.svcName, tc.svcNamespace)
			a.Nil(err)
			a.Equal(tc.expected, actual)
		})
	}
}

func TestUpdateStatus(t *testing.T) {
	testCases := []struct {
		name             string
		existingResource interface{}
		updatedResource  interface{}
		expectErr        bool
	}{
		{
			name: "valid IngressBackend resource",
			existingResource: &policyv1alpha1.IngressBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-backend-1",
					Namespace: "test",
				},
				Spec: policyv1alpha1.IngressBackendSpec{
					Backends: []policyv1alpha1.BackendSpec{
						{
							Name: "backend1",
							Port: policyv1alpha1.PortSpec{
								Number:   80,
								Protocol: "http",
							},
						},
					},
					Sources: []policyv1alpha1.IngressSourceSpec{
						{
							Kind:      "Service",
							Name:      "client",
							Namespace: "foo",
						},
					},
				},
			},
			updatedResource: &policyv1alpha1.IngressBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-backend-1",
					Namespace: "test",
				},
				Spec: policyv1alpha1.IngressBackendSpec{
					Backends: []policyv1alpha1.BackendSpec{
						{
							Name: "backend1",
							Port: policyv1alpha1.PortSpec{
								Number:   80,
								Protocol: "http",
							},
						},
					},
					Sources: []policyv1alpha1.IngressSourceSpec{
						{
							Kind:      "Service",
							Name:      "client",
							Namespace: "foo",
						},
					},
				},
				Status: policyv1alpha1.IngressBackendStatus{
					CurrentStatus: "valid",
					Reason:        "valid",
				},
			},
		},
		{
			name: "valid UpstreamTrafficSetting resource",
			existingResource: &policyv1alpha1.UpstreamTrafficSetting{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: policyv1alpha1.UpstreamTrafficSettingSpec{
					Host: "foo.bar.svc.cluster.local",
				},
			},
			updatedResource: &policyv1alpha1.UpstreamTrafficSetting{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: policyv1alpha1.UpstreamTrafficSettingSpec{
					Host: "foo.bar.svc.cluster.local",
				},
				Status: policyv1alpha1.UpstreamTrafficSettingStatus{
					CurrentStatus: "valid",
					Reason:        "valid",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := tassert.New(t)
			kubeClient := testclient.NewSimpleClientset()
			policyClient := fakePolicyClient.NewSimpleClientset(tc.existingResource.(runtime.Object))
			ic, err := informers.NewInformerCollection(testMeshName, nil, informers.WithKubeClient(kubeClient), informers.WithPolicyClient(policyClient))
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, ic, policyClient, nil)
			switch v := tc.updatedResource.(type) {
			case *policyv1alpha1.IngressBackend:
				_, err = c.UpdateIngressBackendStatus(v)
				a.Equal(tc.expectErr, err != nil)
			case *policyv1alpha1.UpstreamTrafficSetting:
				_, err = c.UpdateUpstreamTrafficSettingStatus(v)
				a.Equal(tc.expectErr, err != nil)
			}
		})
	}
}

func TestGetPodForProxy(t *testing.T) {
	assert := tassert.New(t)
	stop := make(chan struct{})
	defer close(stop)

	proxyUUID := uuid.New()
	someOtherEnvoyUID := uuid.New()
	namespace := tests.BookstoreServiceAccount.Namespace

	podlabels := map[string]string{
		constants.EnvoyUniqueIDLabelName: proxyUUID.String(),
	}
	someOthePodLabels := map[string]string{
		constants.AppLabel:               tests.SelectorValue,
		constants.EnvoyUniqueIDLabelName: someOtherEnvoyUID.String(),
	}

	pod := tests.NewPodFixture(namespace, "pod-1", tests.BookstoreServiceAccountName, podlabels)
	kubeClient := fake.NewSimpleClientset(
		monitoredNS(namespace),
		monitoredNS("bad-namespace"),
		tests.NewPodFixture(namespace, "pod-0", tests.BookstoreServiceAccountName, someOthePodLabels),
		pod,
		tests.NewPodFixture(namespace, "pod-2", tests.BookstoreServiceAccountName, someOthePodLabels),
	)

	ic, err := informers.NewInformerCollection(testMeshName, stop, informers.WithKubeClient(kubeClient))
	assert.Nil(err)

	kubeController := NewClient("osm", tests.OsmMeshConfigName, ic, nil, messaging.NewBroker(nil))

	testCases := []struct {
		name  string
		pod   *corev1.Pod
		proxy *envoy.Proxy
		err   error
	}{
		{
			name:  "fails when UUID does not match",
			proxy: envoy.NewProxy(envoy.KindSidecar, uuid.New(), tests.BookstoreServiceIdentity, nil, 1),
			err:   errDidNotFindPodForUUID,
		},
		{
			name:  "fails when service account does not match certificate",
			proxy: &envoy.Proxy{UUID: proxyUUID, Identity: identity.New("bad-name", namespace)},
			err:   errServiceAccountDoesNotMatchProxy,
		},
		{
			name:  "2 pods with same uuid",
			proxy: envoy.NewProxy(envoy.KindSidecar, someOtherEnvoyUID, tests.BookstoreServiceIdentity, nil, 1),
			err:   errMoreThanOnePodForUUID,
		},
		{
			name:  "fails when namespace does not match certificate",
			proxy: envoy.NewProxy(envoy.KindSidecar, proxyUUID, identity.New(tests.BookstoreServiceAccountName, "bad-namespace"), nil, 1),
			err:   errNamespaceDoesNotMatchProxy,
		},
		{
			name:  "works as expected",
			pod:   pod,
			proxy: envoy.NewProxy(envoy.KindSidecar, proxyUUID, tests.BookstoreServiceIdentity, nil, 1),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := tassert.New(t)
			pod, err := kubeController.GetPodForProxy(tc.proxy)

			assert.Equal(tc.pod, pod)
			assert.Equal(tc.err, err)
		})
	}
}

func monitoredNS(name string) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				constants.OSMKubeResourceMonitorAnnotation: testMeshName,
			},
		},
	}
}

func TestGetMeshConfig(t *testing.T) {
	a := assert.New(t)

	meshConfigClient := fakeConfig.NewSimpleClientset()
	stop := make(chan struct{})
	osmNamespace := "osm"
	osmMeshConfigName := "osm-mesh-config"

	ic, err := informers.NewInformerCollection("osm", stop, informers.WithConfigClient(meshConfigClient, osmMeshConfigName, osmNamespace))
	a.Nil(err)

	c := NewClient(osmNamespace, tests.OsmMeshConfigName, ic, nil, nil)

	// Returns empty MeshConfig if informer cache is empty
	a.Equal(configv1alpha2.MeshConfig{}, c.GetMeshConfig())

	newObj := &configv1alpha2.MeshConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "config.openservicemesh.io",
			Kind:       "MeshConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: osmNamespace,
			Name:      osmMeshConfigName,
		},
	}
	err = c.informers.Add(informers.InformerKeyMeshConfig, newObj, t)
	a.Nil(err)
	a.Equal(*newObj, c.GetMeshConfig())
}

func TestMetricsHandler(t *testing.T) {
	a := assert.New(t)
	osmMeshConfigName := "osm-mesh-config"

	c := &Client{
		informers: &informers.InformerCollection{},
	}
	handlers := c.metricsHandler()
	metricsstore.DefaultMetricsStore.Start(metricsstore.DefaultMetricsStore.FeatureFlagEnabled)

	// Adding the MeshConfig
	handlers.OnAdd(&configv1alpha2.MeshConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: osmMeshConfigName,
		},
		Spec: configv1alpha2.MeshConfigSpec{
			FeatureFlags: configv1alpha2.FeatureFlags{
				EnableRetryPolicy: true,
			},
		},
	})
	a.True(metricsstore.DefaultMetricsStore.Contains(`osm_feature_flag_enabled{feature_flag="enableRetryPolicy"} 1` + "\n"))
	a.True(metricsstore.DefaultMetricsStore.Contains(`osm_feature_flag_enabled{feature_flag="enableSnapshotCacheMode"} 0` + "\n"))

	// Updating the MeshConfig
	handlers.OnUpdate(nil, &configv1alpha2.MeshConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: osmMeshConfigName,
		},
		Spec: configv1alpha2.MeshConfigSpec{
			FeatureFlags: configv1alpha2.FeatureFlags{
				EnableSnapshotCacheMode: true,
			},
		},
	})
	a.True(metricsstore.DefaultMetricsStore.Contains(`osm_feature_flag_enabled{feature_flag="enableRetryPolicy"} 0` + "\n"))
	a.True(metricsstore.DefaultMetricsStore.Contains(`osm_feature_flag_enabled{feature_flag="enableSnapshotCacheMode"} 1` + "\n"))

	// Deleting the MeshConfig
	handlers.OnDelete(&configv1alpha2.MeshConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: osmMeshConfigName,
		},
		Spec: configv1alpha2.MeshConfigSpec{
			FeatureFlags: configv1alpha2.FeatureFlags{
				EnableSnapshotCacheMode: true,
			},
		},
	})
	a.True(metricsstore.DefaultMetricsStore.Contains(`osm_feature_flag_enabled{feature_flag="enableRetryPolicy"} 0` + "\n"))
	a.True(metricsstore.DefaultMetricsStore.Contains(`osm_feature_flag_enabled{feature_flag="enableSnapshotCacheMode"} 0` + "\n"))
}

func TestListEgressPolicies(t *testing.T) {
	egressNsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNs,
		},
	}

	outMeshResource := &policyv1alpha1.Egress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "egress-1",
			Namespace: "wrong-ns",
		},
		Spec: policyv1alpha1.EgressSpec{
			Sources: []policyv1alpha1.EgressSourceSpec{
				{
					Kind:      "ServiceAccount",
					Name:      "sa-1",
					Namespace: testNs,
				},
				{
					Kind:      "ServiceAccount",
					Name:      "sa-2",
					Namespace: testNs,
				},
			},
			Hosts: []string{"foo.com"},
			Ports: []policyv1alpha1.PortSpec{
				{
					Number:   80,
					Protocol: "http",
				},
			},
		},
	}
	inMeshResource := &policyv1alpha1.Egress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "egress-1",
			Namespace: testNs,
		},
		Spec: policyv1alpha1.EgressSpec{
			Sources: []policyv1alpha1.EgressSourceSpec{
				{
					Kind:      "ServiceAccount",
					Name:      "sa-1",
					Namespace: testNs,
				},
				{
					Kind:      "ServiceAccount",
					Name:      "sa-2",
					Namespace: testNs,
				},
			},
			Hosts: []string{"foo.com"},
			Ports: []policyv1alpha1.PortSpec{
				{
					Number:   80,
					Protocol: "http",
				},
			},
		},
	}

	testCases := []struct {
		name             string
		allEgresses      []*policyv1alpha1.Egress
		expectedEgresses []*policyv1alpha1.Egress
	}{
		{
			name:             "Only return egress resources for monitored namespaces",
			allEgresses:      []*policyv1alpha1.Egress{inMeshResource, outMeshResource},
			expectedEgresses: []*policyv1alpha1.Egress{inMeshResource},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("Running test case %d: %s", i, tc.name), func(t *testing.T) {
			a := assert.New(t)

			fakeClient := fakePolicyClient.NewSimpleClientset()
			informerCollection, err := informers.NewInformerCollection("osm", nil,
				informers.WithPolicyClient(fakeClient),
				informers.WithKubeClient(testclient.NewSimpleClientset()))
			a.Nil(err)

			c := NewClient("osm", tests.OsmMeshConfigName, informerCollection, fakeClient, nil)
			a.Nil(err)
			a.NotNil(c)

			// monitor namespaces
			err = c.informers.Add(informers.InformerKeyNamespace, egressNsObj, t)
			a.Nil(err)

			// Create fake egress policies
			for _, egressPolicy := range tc.allEgresses {
				_ = c.informers.Add(informers.InformerKeyEgress, egressPolicy, t)
			}

			policies := c.ListEgressPolicies()
			a.ElementsMatch(tc.expectedEgresses, policies)
		})
	}
}

func TestListRetryPolicy(t *testing.T) {
	policyNsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNs,
		},
	}

	var thresholdUintVal uint32 = 3
	thresholdTimeoutDuration := metav1.Duration{Duration: time.Duration(5 * time.Second)}
	thresholdBackoffDuration := metav1.Duration{Duration: time.Duration(1 * time.Second)}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockKubeController := NewMockController(mockCtrl)
	mockKubeController.EXPECT().IsMonitoredNamespace("test").Return(true).AnyTimes()

	outMeshResource := &policyv1alpha1.Retry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retry-1",
			Namespace: "wrong-ns",
		},
		Spec: policyv1alpha1.RetrySpec{
			Source: policyv1alpha1.RetrySrcDstSpec{
				Kind:      "ServiceAccount",
				Name:      "sa-1",
				Namespace: testNs,
			},
			Destinations: []policyv1alpha1.RetrySrcDstSpec{
				{
					Kind:      "Service",
					Name:      "s1",
					Namespace: testNs,
				},
				{
					Kind:      "Service",
					Name:      "s2",
					Namespace: testNs,
				},
			},
			RetryPolicy: policyv1alpha1.RetryPolicySpec{
				RetryOn:                  "",
				NumRetries:               &thresholdUintVal,
				PerTryTimeout:            &thresholdTimeoutDuration,
				RetryBackoffBaseInterval: &thresholdBackoffDuration,
			},
		},
	}
	inMeshResource := &policyv1alpha1.Retry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retry-1",
			Namespace: testNs,
		},
		Spec: policyv1alpha1.RetrySpec{
			Source: policyv1alpha1.RetrySrcDstSpec{
				Kind:      "ServiceAccount",
				Name:      "sa-1",
				Namespace: testNs,
			},
			Destinations: []policyv1alpha1.RetrySrcDstSpec{
				{
					Kind:      "Service",
					Name:      "s1",
					Namespace: testNs,
				},
				{
					Kind:      "Service",
					Name:      "s2",
					Namespace: testNs,
				},
			},
			RetryPolicy: policyv1alpha1.RetryPolicySpec{
				RetryOn:                  "",
				NumRetries:               &thresholdUintVal,
				PerTryTimeout:            &thresholdTimeoutDuration,
				RetryBackoffBaseInterval: &thresholdBackoffDuration,
			},
		},
	}

	testCases := []struct {
		name            string
		allRetries      []*policyv1alpha1.Retry
		expectedRetries []*policyv1alpha1.Retry
	}{
		{
			name:            "Only return retry resources for monitored namespaces",
			allRetries:      []*policyv1alpha1.Retry{inMeshResource, outMeshResource},
			expectedRetries: []*policyv1alpha1.Retry{inMeshResource},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("Running test case %d: %s", i, tc.name), func(t *testing.T) {
			a := assert.New(t)

			fakeClient := fakePolicyClient.NewSimpleClientset()
			informerCollection, err := informers.NewInformerCollection("osm", nil,
				informers.WithPolicyClient(fakeClient),
				informers.WithKubeClient(testclient.NewSimpleClientset()),
			)
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, informerCollection, fakeClient, nil)
			a.Nil(err)
			a.NotNil(c)

			// monitor namespaces
			err = c.informers.Add(informers.InformerKeyNamespace, policyNsObj, t)
			a.Nil(err)

			// Create fake retry policies
			for _, retryPolicy := range tc.allRetries {
				err := c.informers.Add(informers.InformerKeyRetry, retryPolicy, t)
				a.Nil(err)
			}

			policies := c.ListRetryPolicies()
			a.ElementsMatch(tc.expectedRetries, policies)
		})
	}
}

func TestListUpstreamTrafficSetting(t *testing.T) {
	settingNsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNs,
		},
	}

	inMeshResource := &policyv1alpha1.UpstreamTrafficSetting{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "u1",
			Namespace: testNs,
		},
		Spec: policyv1alpha1.UpstreamTrafficSettingSpec{
			Host: "s1.ns1.svc.cluster.local",
		},
	}
	outMeshResource := &policyv1alpha1.UpstreamTrafficSetting{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "u1",
			Namespace: "wrong-ns",
		},
		Spec: policyv1alpha1.UpstreamTrafficSettingSpec{
			Host: "s1.ns1.svc.cluster.local",
		},
	}
	testCases := []struct {
		name         string
		allResources []*policyv1alpha1.UpstreamTrafficSetting
		expected     []*policyv1alpha1.UpstreamTrafficSetting
	}{
		{
			name:         "Only return upstream traffic settings for monitored namespaces",
			allResources: []*policyv1alpha1.UpstreamTrafficSetting{inMeshResource, outMeshResource},
			expected:     []*policyv1alpha1.UpstreamTrafficSetting{inMeshResource},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)

			fakeClient := fakePolicyClient.NewSimpleClientset()
			informerCollection, err := informers.NewInformerCollection("osm", nil,
				informers.WithPolicyClient(fakeClient),
				informers.WithKubeClient(testclient.NewSimpleClientset()),
			)
			a.Nil(err)
			c := NewClient("osm", tests.OsmMeshConfigName, informerCollection, fakeClient, nil)
			a.Nil(err)
			a.NotNil(c)

			// monitor namespaces
			err = c.informers.Add(informers.InformerKeyNamespace, settingNsObj, t)
			a.Nil(err)

			// Create fake upstream traffic settings
			for _, resource := range tc.allResources {
				_ = c.informers.Add(informers.InformerKeyUpstreamTrafficSetting, resource, t)
			}

			actual := c.ListUpstreamTrafficSettings()
			a.Equal(tc.expected, actual)
		})
	}
}
