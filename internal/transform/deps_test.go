package transform_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/chart2kro/internal/k8s"
	"github.com/hupe1980/chart2kro/internal/transform"
)

func TestDependencyGraph_AddNodeAndNodes(t *testing.T) {
	g := transform.NewDependencyGraph()
	r1 := makeFullResource("v1", "ConfigMap", "cm1", nil)
	r2 := makeFullResource("v1", "Secret", "sec1", nil)

	g.AddNode("configmap", r1)
	g.AddNode("secret", r2)

	nodes := g.Nodes()
	assert.Equal(t, []string{"configmap", "secret"}, nodes)
}

func TestDependencyGraph_AddEdgeAndDependenciesOf(t *testing.T) {
	g := transform.NewDependencyGraph()
	r1 := makeFullResource("apps/v1", "Deployment", "dep1", nil)
	r2 := makeFullResource("v1", "ConfigMap", "cm1", nil)

	g.AddNode("deployment", r1)
	g.AddNode("configmap", r2)
	g.AddEdge("deployment", "configmap")

	deps := g.DependenciesOf("deployment")
	assert.Equal(t, []string{"configmap"}, deps)

	deps = g.DependenciesOf("configmap")
	assert.Empty(t, deps)
}

func TestDependencyGraph_Resource(t *testing.T) {
	g := transform.NewDependencyGraph()
	r := makeFullResource("v1", "ConfigMap", "cm1", nil)
	g.AddNode("configmap", r)

	assert.Equal(t, r, g.Resource("configmap"))
	assert.Nil(t, g.Resource("nonexistent"))
}

func TestDependencyGraph_EdgeCount(t *testing.T) {
	t.Run("empty graph", func(t *testing.T) {
		g := transform.NewDependencyGraph()
		assert.Equal(t, 0, g.EdgeCount())
	})

	t.Run("nodes without edges", func(t *testing.T) {
		g := transform.NewDependencyGraph()
		g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
		g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
		assert.Equal(t, 0, g.EdgeCount())
	})

	t.Run("linear chain", func(t *testing.T) {
		g := transform.NewDependencyGraph()
		g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
		g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
		g.AddNode("c", makeFullResource("apps/v1", "Deployment", "c", nil))
		g.AddEdge("c", "b")
		g.AddEdge("b", "a")
		assert.Equal(t, 2, g.EdgeCount())
	})

	t.Run("diamond", func(t *testing.T) {
		g := transform.NewDependencyGraph()
		g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
		g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
		g.AddNode("c", makeFullResource("v1", "Secret", "c", nil))
		g.AddNode("d", makeFullResource("apps/v1", "Deployment", "d", nil))
		g.AddEdge("d", "b")
		g.AddEdge("d", "c")
		g.AddEdge("b", "a")
		g.AddEdge("c", "a")
		assert.Equal(t, 4, g.EdgeCount())
	})
}

func TestTopologicalSort_LinearChain(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
	g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
	g.AddNode("c", makeFullResource("apps/v1", "Deployment", "c", nil))

	// c depends on b, b depends on a => order: a, b, c.
	g.AddEdge("c", "b")
	g.AddEdge("b", "a")

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestTopologicalSort_Diamond(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
	g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
	g.AddNode("c", makeFullResource("v1", "Secret", "c", nil))
	g.AddNode("d", makeFullResource("apps/v1", "Deployment", "d", nil))

	// d depends on b and c, both depend on a.
	g.AddEdge("d", "b")
	g.AddEdge("d", "c")
	g.AddEdge("b", "a")
	g.AddEdge("c", "a")

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	// a must come first, d must come last, b and c in alphabetical order.
	assert.Equal(t, "a", order[0])
	assert.Equal(t, "d", order[3])
}

func TestTopologicalSort_MultipleRoots(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
	g.AddNode("b", makeFullResource("v1", "ConfigMap", "b", nil))
	g.AddNode("c", makeFullResource("v1", "ConfigMap", "c", nil))

	// No edges - all independent. Should be alphabetical.
	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestTopologicalSort_Cycle(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
	g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))

	g.AddEdge("a", "b")
	g.AddEdge("b", "a")

	_, err := g.TopologicalSort()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestDetectCycles_NoCycle(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
	g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
	g.AddEdge("b", "a")

	cycles := g.DetectCycles()
	assert.Empty(t, cycles)
}

func TestDetectCycles_WithCycle(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("a", makeFullResource("v1", "ConfigMap", "a", nil))
	g.AddNode("b", makeFullResource("v1", "Secret", "b", nil))
	g.AddNode("c", makeFullResource("v1", "Service", "c", nil))

	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "a")

	cycles := g.DetectCycles()
	assert.NotEmpty(t, cycles)
	// With deduplication, there should be exactly 1 unique cycle.
	assert.Len(t, cycles, 1, "duplicate cycles should be deduplicated")
}

func TestBuildDependencyGraph_SelectorDeps(t *testing.T) {
	deploy := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "web",
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "web",
							"image": "nginx",
						},
					},
				},
			},
		},
	})

	svc := makeFullResource("v1", "Service", "web-svc", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "web",
			},
		},
	})

	resources := map[*k8s.Resource]string{
		deploy: "deployment",
		svc:    "service",
	}

	g := transform.BuildDependencyGraph(resources)

	// Service should depend on Deployment (selector match).
	deps := g.DependenciesOf("service")
	assert.Equal(t, []string{"deployment"}, deps)

	// Deployment should have no deps.
	deps = g.DependenciesOf("deployment")
	assert.Empty(t, deps)
}

func TestBuildDependencyGraph_VolumeDeps(t *testing.T) {
	cm := makeFullResource("v1", "ConfigMap", "app-config", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})

	secret := makeFullResource("v1", "Secret", "app-secret", map[string]interface{}{
		"data": map[string]interface{}{"key": "dmFsdWU="},
	})

	pvc := makeFullResource("v1", "PersistentVolumeClaim", "data-pvc", map[string]interface{}{
		"spec": map[string]interface{}{
			"accessModes": []interface{}{"ReadWriteOnce"},
		},
	})

	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test"},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "config-vol",
							"configMap": map[string]interface{}{
								"name": "app-config",
							},
						},
						map[string]interface{}{
							"name": "secret-vol",
							"secret": map[string]interface{}{
								"secretName": "app-secret",
							},
						},
						map[string]interface{}{
							"name": "data-vol",
							"persistentVolumeClaim": map[string]interface{}{
								"claimName": "data-pvc",
							},
						},
					},
				},
			},
		},
	})

	resources := map[*k8s.Resource]string{
		cm:     "configmap",
		secret: "secret",
		pvc:    "persistentvolumeclaim",
		deploy: "deployment",
	}

	g := transform.BuildDependencyGraph(resources)

	deps := g.DependenciesOf("deployment")
	assert.Contains(t, deps, "configmap")
	assert.Contains(t, deps, "secret")
	assert.Contains(t, deps, "persistentvolumeclaim")
	assert.Len(t, deps, 3)
}

func TestBuildDependencyGraph_ServiceAccountDeps(t *testing.T) {
	sa := makeFullResource("v1", "ServiceAccount", "app-sa", map[string]interface{}{})

	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test"},
				},
				"spec": map[string]interface{}{
					"serviceAccountName": "app-sa",
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		},
	})

	resources := map[*k8s.Resource]string{
		sa:     "serviceaccount",
		deploy: "deployment",
	}

	g := transform.BuildDependencyGraph(resources)
	deps := g.DependenciesOf("deployment")
	assert.Equal(t, []string{"serviceaccount"}, deps)
}

func TestBuildDependencyGraph_EnvDeps(t *testing.T) {
	secret := makeFullResource("v1", "Secret", "db-secret", map[string]interface{}{
		"data": map[string]interface{}{"password": "cGFzcw=="},
	})

	cm := makeFullResource("v1", "ConfigMap", "app-env", map[string]interface{}{
		"data": map[string]interface{}{"LOG_LEVEL": "info"},
	})

	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test"},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name": "DB_PASS",
									"valueFrom": map[string]interface{}{
										"secretKeyRef": map[string]interface{}{
											"name": "db-secret",
											"key":  "password",
										},
									},
								},
							},
							"envFrom": []interface{}{
								map[string]interface{}{
									"configMapRef": map[string]interface{}{
										"name": "app-env",
									},
								},
							},
						},
					},
				},
			},
		},
	})

	resources := map[*k8s.Resource]string{
		secret: "secret",
		cm:     "configmap",
		deploy: "deployment",
	}

	g := transform.BuildDependencyGraph(resources)
	deps := g.DependenciesOf("deployment")
	assert.Contains(t, deps, "secret")
	assert.Contains(t, deps, "configmap")
	assert.Len(t, deps, 2)
}

func TestBuildDependencyGraph_NoObjectSkipped(t *testing.T) {
	r := makeResource("ConfigMap", "orphan")

	resources := map[*k8s.Resource]string{r: "configmap"}
	g := transform.BuildDependencyGraph(resources)
	assert.Equal(t, []string{"configmap"}, g.Nodes())
	assert.Empty(t, g.DependenciesOf("configmap"))
}

func TestBuildDependencyGraph_FullStack(t *testing.T) {
	sa := makeFullResource("v1", "ServiceAccount", "app-sa", map[string]interface{}{})
	cm := makeFullResource("v1", "ConfigMap", "app-config", map[string]interface{}{
		"data": map[string]interface{}{"key": "val"},
	})
	secret := makeFullResource("v1", "Secret", "app-secret", map[string]interface{}{
		"data": map[string]interface{}{"key": "dmFs"},
	})
	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "myapp"},
				},
				"spec": map[string]interface{}{
					"serviceAccountName": "app-sa",
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name": "SECRET_KEY",
									"valueFrom": map[string]interface{}{
										"secretKeyRef": map[string]interface{}{
											"name": "app-secret",
											"key":  "key",
										},
									},
								},
							},
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "config",
							"configMap": map[string]interface{}{
								"name": "app-config",
							},
						},
					},
				},
			},
		},
	})
	svc := makeFullResource("v1", "Service", "app-svc", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"app": "myapp"},
		},
	})

	resources := map[*k8s.Resource]string{
		sa:     "serviceaccount",
		cm:     "configmap",
		secret: "secret",
		deploy: "deployment",
		svc:    "service",
	}

	g := transform.BuildDependencyGraph(resources)

	// Deployment depends on SA, Secret (env), ConfigMap (volume).
	deployDeps := g.DependenciesOf("deployment")
	assert.Contains(t, deployDeps, "serviceaccount")
	assert.Contains(t, deployDeps, "secret")
	assert.Contains(t, deployDeps, "configmap")

	// Service depends on Deployment (selector match).
	svcDeps := g.DependenciesOf("service")
	assert.Equal(t, []string{"deployment"}, svcDeps)

	// SA, CM, Secret have no deps.
	assert.Empty(t, g.DependenciesOf("serviceaccount"))
	assert.Empty(t, g.DependenciesOf("configmap"))
	assert.Empty(t, g.DependenciesOf("secret"))

	// Topological sort should succeed.
	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Len(t, order, 5)

	// SA, CM, Secret must come before Deployment. Service must come after Deployment.
	deployIdx := indexOf(order, "deployment")
	svcIdx := indexOf(order, "service")
	saIdx := indexOf(order, "serviceaccount")
	cmIdx := indexOf(order, "configmap")
	secretIdx := indexOf(order, "secret")

	assert.Less(t, saIdx, deployIdx)
	assert.Less(t, cmIdx, deployIdx)
	assert.Less(t, secretIdx, deployIdx)
	assert.Less(t, deployIdx, svcIdx)
}

func TestTopologicalSort_SingleNode(t *testing.T) {
	g := transform.NewDependencyGraph()
	g.AddNode("only", makeFullResource("v1", "ConfigMap", "only", nil))

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []string{"only"}, order)
}

func TestBuildDependencyGraph_SelectorNoMatch(t *testing.T) {
	deploy := makeFullResource("apps/v1", "Deployment", "web", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "web",
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "web",
							"image": "nginx",
						},
					},
				},
			},
		},
	})

	svc := makeFullResource("v1", "Service", "other-svc", map[string]interface{}{
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "other",
			},
		},
	})

	resources := map[*k8s.Resource]string{
		deploy: "deployment",
		svc:    "service",
	}

	g := transform.BuildDependencyGraph(resources)
	assert.Empty(t, g.DependenciesOf("service"))
}

func TestBuildDependencyGraph_EnvConfigMapKeyRef(t *testing.T) {
	cm := makeFullResource("v1", "ConfigMap", "env-config", map[string]interface{}{
		"data": map[string]interface{}{"key": "val"},
	})

	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test"},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name": "CONFIG_VAL",
									"valueFrom": map[string]interface{}{
										"configMapKeyRef": map[string]interface{}{
											"name": "env-config",
											"key":  "key",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	resources := map[*k8s.Resource]string{
		cm:     "configmap",
		deploy: "deployment",
	}

	g := transform.BuildDependencyGraph(resources)
	deps := g.DependenciesOf("deployment")
	assert.Equal(t, []string{"configmap"}, deps)
}

func TestBuildDependencyGraph_EnvFromSecretRef(t *testing.T) {
	secret := makeFullResource("v1", "Secret", "env-secret", map[string]interface{}{
		"data": map[string]interface{}{"key": "dmFs"},
	})

	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test"},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"envFrom": []interface{}{
								map[string]interface{}{
									"secretRef": map[string]interface{}{
										"name": "env-secret",
									},
								},
							},
						},
					},
				},
			},
		},
	})

	resources := map[*k8s.Resource]string{
		secret: "secret",
		deploy: "deployment",
	}

	g := transform.BuildDependencyGraph(resources)
	deps := g.DependenciesOf("deployment")
	assert.Equal(t, []string{"secret"}, deps)
}

func TestAddEdge_SelfReference(t *testing.T) {
	g := transform.NewDependencyGraph()
	r := makeResource("ConfigMap", "test")
	g.AddNode("cm", r)
	g.AddEdge("cm", "cm") // self-reference should be ignored

	deps := g.DependenciesOf("cm")
	assert.Empty(t, deps, "self-references should be ignored")

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []string{"cm"}, order)
}

func TestAddEdge_NonExistentTarget(t *testing.T) {
	g := transform.NewDependencyGraph()
	r := makeResource("ConfigMap", "test")
	g.AddNode("cm", r)
	g.AddEdge("cm", "nonexistent") // target doesn't exist — should be ignored

	deps := g.DependenciesOf("cm")
	assert.Empty(t, deps, "edges to non-existent nodes should be ignored")
}

func TestBuildDependencyGraph_InitContainerEnvRef(t *testing.T) {
	secret := makeFullResource("v1", "Secret", "init-secret", map[string]interface{}{
		"data": map[string]interface{}{"key": "value"},
	})
	deploy := makeFullResource("apps/v1", "Deployment", "app", map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{"app": "test"},
				},
				"spec": map[string]interface{}{
					"initContainers": []interface{}{
						map[string]interface{}{
							"name":  "init",
							"image": "init:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name": "SECRET_KEY",
									"valueFrom": map[string]interface{}{
										"secretKeyRef": map[string]interface{}{
											"name": "init-secret",
											"key":  "key",
										},
									},
								},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		},
	})

	resources := map[*k8s.Resource]string{
		secret: "secret",
		deploy: "deployment",
	}

	g := transform.BuildDependencyGraph(resources)
	deps := g.DependenciesOf("deployment")
	assert.Contains(t, deps, "secret", "initContainers env refs should be detected")
}
