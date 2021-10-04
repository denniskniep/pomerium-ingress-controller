package pomerium

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	pb "github.com/pomerium/pomerium/pkg/grpc/config"

	"github.com/pomerium/ingress-controller/model"
)

func TestHttp01Solver(t *testing.T) {
	ptype := networkingv1.PathTypeExact
	routes, err := ingressToRoutes(context.Background(), &model.IngressConfig{
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cm-acme-http-solver-9m9mw",
				Namespace: "default",
				Labels: map[string]string{
					"acme.cert-manager.io/http01-solver": "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{{
					Host: "ingress-to-create.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/.well-known/acme-challenge/0zdvVjgtDwEjCX6zIlynXvaP5Zekff4ZKQgezH_B4IM",
								PathType: &ptype,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "cm-acme-http-solver-7pf4j",
										Port: networkingv1.ServiceBackendPort{Number: 8089},
									},
									Resource: &corev1.TypedLocalObjectReference{},
								},
							}},
						},
					},
				}},
			},
		},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "cm-acme-http-solver-7pf4j", Namespace: "default"}: {
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cm-acme-http-solver-7pf4j",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Protocol:    "TCP",
						AppProtocol: new(string),
						Port:        8089,
						TargetPort:  intstr.FromInt(8089),
					}},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, routes, 1)
	require.True(t, routes[0].AllowPublicUnauthenticatedAccess)
	require.True(t, routes[0].PreserveHostHeader)
}

func TestUpsertIngress(t *testing.T) {
	typePrefix := networkingv1.PathTypePrefix
	ic := &model.IngressConfig{
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "default"},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/a",
								PathType: &typePrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "service",
										Port: networkingv1.ServiceBackendPort{
											Name: "http",
										},
									},
								},
							}},
						},
					},
				}},
			},
		},
		Secrets: map[types.NamespacedName]*corev1.Secret{
			{Name: "secret", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					corev1.TLSPrivateKeyKey: []byte("A"),
					corev1.TLSCertKey:       []byte("A"),
				},
				Type: corev1.SecretTypeTLS,
			}},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Protocol:   "TCP",
						Port:       80,
						TargetPort: intstr.IntOrString{IntVal: 80},
					}},
				},
				Status: corev1.ServiceStatus{},
			},
		},
	}

	cfg := new(pb.Config)
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err := routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	require.NotNil(t, routes[routeID{
		Name:      "ingress",
		Namespace: "default",
		Path:      "/a",
		Host:      "service.localhost.pomerium.io",
	}])

	ic.Ingress.Spec.Rules[0].HTTP.Paths = append(ic.Ingress.Spec.Rules[0].HTTP.Paths, networkingv1.HTTPIngressPath{
		Path:     "/b",
		PathType: &typePrefix,
		Backend: networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: "service",
				Port: networkingv1.ServiceBackendPort{
					Name: "http",
				},
			},
		},
	})
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err = routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/a", Host: "service.localhost.pomerium.io"}])
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/b", Host: "service.localhost.pomerium.io"}])

	ic.Ingress.Spec.Rules[0].HTTP.Paths[0].Path = "/c"
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err = routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	require.Nil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/a", Host: "service.localhost.pomerium.io"}])
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/b", Host: "service.localhost.pomerium.io"}])
	require.NotNil(t, routes[routeID{Name: "ingress", Namespace: "default", Path: "/c", Host: "service.localhost.pomerium.io"}])
}

func TestSecureUpstream(t *testing.T) {
	typePrefix := networkingv1.PathTypePrefix
	ic := &model.IngressConfig{
		AnnotationPrefix: "p",
		Ingress: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ingress",
				Namespace: "default",
				Annotations: map[string]string{
					fmt.Sprintf("p/%s", model.SecureUpstream): "true",
				},
			},
			Spec: networkingv1.IngressSpec{
				TLS: []networkingv1.IngressTLS{{
					Hosts:      []string{"service.localhost.pomerium.io"},
					SecretName: "secret",
				}},
				Rules: []networkingv1.IngressRule{{
					Host: "service.localhost.pomerium.io",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/a",
								PathType: &typePrefix,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "service",
										Port: networkingv1.ServiceBackendPort{
											Name: "https",
										},
									},
								},
							}},
						},
					},
				}},
			},
		},
		Services: map[types.NamespacedName]*corev1.Service{
			{Name: "service", Namespace: "default"}: {
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "https",
						Protocol:   "TCP",
						Port:       443,
						TargetPort: intstr.IntOrString{IntVal: 443},
					}},
				},
				Status: corev1.ServiceStatus{},
			},
		},
	}

	cfg := new(pb.Config)
	require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
	routes, err := routeList(cfg.Routes).toMap()
	require.NoError(t, err)
	route := routes[routeID{
		Name:      "ingress",
		Namespace: "default",
		Path:      "/a",
		Host:      "service.localhost.pomerium.io",
	}]
	require.NotNil(t, route)
	require.Equal(t, []string{
		"https://service.default.svc.cluster.local:443",
	}, route.To)
}

func TestExternalService(t *testing.T) {
	makeRoute := func(t *testing.T, secure bool) (*pb.Route, error) {
		typePrefix := networkingv1.PathTypePrefix
		ic := &model.IngressConfig{
			AnnotationPrefix: "p",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress",
					Namespace: "default",
				},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{{
						Hosts:      []string{"service.localhost.pomerium.io"},
						SecretName: "secret",
					}},
					Rules: []networkingv1.IngressRule{{
						Host: "service.localhost.pomerium.io",
						IngressRuleValue: networkingv1.IngressRuleValue{
							HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{
									Path:     "/a",
									PathType: &typePrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "service",
											Port: networkingv1.ServiceBackendPort{
												Name: "app",
											},
										},
									},
								}},
							},
						},
					}},
				},
			},
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "service", Namespace: "default"}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Type:         corev1.ServiceTypeExternalName,
						ExternalName: "service.external.com",
						Ports: []corev1.ServicePort{{
							Name:     "app",
							Protocol: "TCP",
							Port:     9999,
						}},
					},
				},
			},
		}
		if secure {
			ic.Ingress.Annotations = map[string]string{fmt.Sprintf("p/%s", model.SecureUpstream): "true"}
		}

		cfg := new(pb.Config)
		if err := upsertRoutes(context.Background(), cfg, ic); err != nil {
			return nil, fmt.Errorf("upsert routes: %w", err)
		}
		routes, err := routeList(cfg.Routes).toMap()
		if err != nil {
			return nil, err
		}
		return routes[routeID{
			Name:      "ingress",
			Namespace: "default",
			Path:      "/a",
			Host:      "service.localhost.pomerium.io",
		}], nil
	}

	for _, tc := range []struct {
		secure    bool
		expectURL string
	}{
		{
			secure:    false,
			expectURL: "http://service.external.com:9999",
		},
		{
			secure:    true,
			expectURL: "https://service.external.com:9999",
		},
	} {
		t.Run(fmt.Sprintf("%+v", tc), func(t *testing.T) {
			route, err := makeRoute(t, tc.secure)
			require.NoError(t, err)
			require.Equal(t, []string{tc.expectURL}, route.To)
		})
	}
}

func TestDefaultBackendService(t *testing.T) {
	typePrefix := networkingv1.PathTypePrefix
	typeExact := networkingv1.PathTypeExact
	icTemplate := func() *model.IngressConfig {
		return &model.IngressConfig{
			AnnotationPrefix: "p",
			Ingress: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "default"},
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{{
						Hosts:      []string{"service.localhost.pomerium.io"},
						SecretName: "secret",
					}},
					DefaultBackend: &networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: "service",
							Port: networkingv1.ServiceBackendPort{
								Name: "app",
							},
						},
					},
				},
			},
			Services: map[types.NamespacedName]*corev1.Service{
				{Name: "service", Namespace: "default"}: {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Type:         corev1.ServiceTypeExternalName,
						ExternalName: "service.external.com",
						Ports: []corev1.ServicePort{{
							Name:     "app",
							Protocol: "TCP",
							Port:     9999,
						}},
					},
				},
			},
		}
	}

	t.Run("just default backend", func(t *testing.T) {
		ic := icTemplate()
		cfg := new(pb.Config)
		t.Log(protojson.Format(cfg))
		require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
		require.Len(t, cfg.Routes, 1)
		assert.Equal(t, "/", cfg.Routes[0].Prefix)
	})

	t.Run("default backend and rule", func(t *testing.T) {
		ic := icTemplate()
		ic.Spec.Rules = []networkingv1.IngressRule{{
			Host: "service.localhost.pomerium.io",
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{{
						Path:     "/two",
						PathType: &typeExact,
						Backend:  *ic.Spec.DefaultBackend,
					}, {
						Path:     "/one",
						PathType: &typePrefix,
						Backend:  *ic.Spec.DefaultBackend,
					}},
				},
			}}}
		cfg := new(pb.Config)
		require.NoError(t, upsertRoutes(context.Background(), cfg, ic))
		sort.Sort(routeList(cfg.Routes))
		require.Len(t, cfg.Routes, 3)
		assert.Equal(t, "/", cfg.Routes[2].Prefix, protojson.Format(cfg))
	})
}

// TestRouteSortOrder ensures we're following
// https://kubernetes.io/docs/concepts/services-networking/ingress/#multiple-matches
// 1. precedence will be given first to the longest matching path.
// 2. If two paths are still equally matched, precedence will be given to paths with an exact path type over prefix path type.
func TestRouteSortOrder(t *testing.T) {
	routes := routeList{{
		From:   "https://site",
		Prefix: "/",
		Id:     "c",
	}, {
		From: "https://site",
		Path: "/.well-known/something",
		Id:   "a",
	}, {
		From: "https://site",
		Path: "/.well-known/else",
		Id:   "b",
	}}
	assert.True(t, routes.Less(1, 0))
	sort.Sort(routes)
	assert.Equal(t, "a", routes[0].Id)
	assert.Equal(t, "b", routes[1].Id)
}
