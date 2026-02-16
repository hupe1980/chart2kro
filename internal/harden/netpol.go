package harden

import (
	"context"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/hupe1980/chart2kro/internal/k8s"
)

// serviceKind identifies Kubernetes Service resources.
const serviceKind = "Service"

// NetworkPolicyGenerator generates NetworkPolicy resources based on resource relationships.
type NetworkPolicyGenerator struct {
	resourceIDs map[*k8s.Resource]string
}

// NewNetworkPolicyGenerator creates a generator that produces deny-all + allow NetworkPolicies.
func NewNetworkPolicyGenerator(resourceIDs map[*k8s.Resource]string) *NetworkPolicyGenerator {
	return &NetworkPolicyGenerator{resourceIDs: resourceIDs}
}

// Name returns the policy name.
func (g *NetworkPolicyGenerator) Name() string {
	return "network-policy-generator"
}

// Apply generates one NetworkPolicy per workload with a deny-all default + ingress
// from services that select it.
func (g *NetworkPolicyGenerator) Apply(ctx context.Context, resources []*k8s.Resource, result *Result) error {
	// Build a map of resource names to resources for cross-referencing.
	workloads := make(map[string]*k8s.Resource)
	services := make(map[string]*k8s.Resource)

	for _, res := range resources {
		if isWorkload(res) {
			workloads[res.Name] = res
		}

		if res.Kind() == serviceKind {
			services[res.Name] = res
		}
	}

	// Generate a NetworkPolicy for each workload.
	for name, workload := range workloads {
		netpol := generateNetworkPolicy(name, workload, services)

		result.Resources = append(result.Resources, netpol)
		result.Changes = append(result.Changes, Change{
			ResourceID: netpol.QualifiedName(),
			FieldPath:  "",
			NewValue:   "generated",
			Reason:     "network-policy-generator",
		})
	}

	return nil
}

// generateNetworkPolicy creates a deny-all NetworkPolicy for a workload,
// with ingress rules from services that select it.
func generateNetworkPolicy(name string, workload *k8s.Resource, services map[string]*k8s.Resource) *k8s.Resource {
	// Use the workload's matchLabels as the pod selector.
	podSelector := extractMatchLabels(workload)

	// Build ingress rules from services that select this workload.
	var ingressRules []interface{}

	for _, svc := range services {
		svcSelector := extractServiceSelector(svc)
		if svcSelector == nil {
			continue
		}

		// Check if the service selects pods matching this workload's labels.
		if !selectorsOverlap(svcSelector, podSelector) {
			continue
		}

		// Extract service ports for the ingress rule.
		ports := extractServicePorts(svc)
		if len(ports) > 0 {
			rule := map[string]interface{}{
				"ports": ports,
			}
			ingressRules = append(ingressRules, rule)
		}
	}

	// Build the NetworkPolicy spec.
	spec := map[string]interface{}{
		"podSelector": map[string]interface{}{
			"matchLabels": podSelector,
		},
		"policyTypes": []interface{}{"Ingress", "Egress"},
	}

	if len(ingressRules) > 0 {
		spec["ingress"] = ingressRules
	}

	// Allow DNS egress by default.
	spec["egress"] = []interface{}{
		map[string]interface{}{
			"ports": []interface{}{
				map[string]interface{}{
					"port":     int64(53),
					"protocol": "UDP",
				},
				map[string]interface{}{
					"port":     int64(53),
					"protocol": "TCP",
				},
			},
		},
	}

	netpolName := name + "-netpol"

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]interface{}{
				"name": netpolName,
			},
			"spec": spec,
		},
	}

	return &k8s.Resource{
		GVK: schema.GroupVersionKind{
			Group:   "networking.k8s.io",
			Version: "v1",
			Kind:    "NetworkPolicy",
		},
		Name:   netpolName,
		Object: obj,
	}
}

// extractMatchLabels extracts spec.selector.matchLabels from a workload.
func extractMatchLabels(res *k8s.Resource) map[string]interface{} {
	if res.Object == nil {
		return map[string]interface{}{"app": res.Name}
	}

	spec, ok := res.Object.Object["spec"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"app": res.Name}
	}

	selector, ok := spec["selector"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"app": res.Name}
	}

	matchLabels, ok := selector["matchLabels"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"app": res.Name}
	}

	return matchLabels
}

// extractServiceSelector extracts spec.selector from a Service.
func extractServiceSelector(svc *k8s.Resource) map[string]interface{} {
	if svc.Object == nil {
		return nil
	}

	spec, ok := svc.Object.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	selector, ok := spec["selector"].(map[string]interface{})
	if !ok {
		return nil
	}

	return selector
}

// selectorsOverlap returns true if all keys in svcSelector are present in podLabels.
func selectorsOverlap(svcSelector, podLabels map[string]interface{}) bool {
	if len(svcSelector) == 0 {
		return false
	}

	for k, v := range svcSelector {
		if podV, ok := podLabels[k]; !ok || !reflect.DeepEqual(podV, v) {
			return false
		}
	}

	return true
}

// extractServicePorts extracts port definitions from a Service for use in NetworkPolicy rules.
func extractServicePorts(svc *k8s.Resource) []interface{} {
	if svc.Object == nil {
		return nil
	}

	spec, ok := svc.Object.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	ports, ok := spec["ports"].([]interface{})
	if !ok {
		return nil
	}

	var netpolPorts []interface{}

	for _, p := range ports {
		port, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		np := map[string]interface{}{}

		if portNum, ok := port["port"]; ok {
			np["port"] = portNum
		}

		if protocol, ok := port["protocol"].(string); ok {
			np["protocol"] = protocol
		} else {
			np["protocol"] = "TCP"
		}

		netpolPorts = append(netpolPorts, np)
	}

	return netpolPorts
}
