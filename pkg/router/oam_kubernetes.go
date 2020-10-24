package router

import (
	"fmt"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

// KubernetesOAMRouter is managing ClusterIP services for OAM flagger
type KubernetesOAMRouter struct {
	innerK8sRouter *KubernetesDefaultRouter
	componentName  string
}

func NewKubernetesOAMRouter(factory *Factory, labelSelector string, ports map[string]int32,
	componentName string) *KubernetesOAMRouter {
	return &KubernetesOAMRouter{
		innerK8sRouter: &KubernetesDefaultRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			labelSelector: labelSelector,
			ports:         ports,
		},
		componentName: componentName,
	}
}

// Initialize creates the primary and canary services
func (kor *KubernetesOAMRouter) Initialize(canary *flaggerv1.Canary) error {
	c := kor.innerK8sRouter
	_, primaryName, canaryName := canary.GetServiceNames()

	// both the canary and primary service select all the component pod
	err := c.reconcileService(canary, canaryName, kor.componentName, canary.Spec.Service.Canary)
	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}
	// primary svc, which points to the source object
	err = c.reconcileService(canary, primaryName, kor.componentName, canary.Spec.Service.Primary)
	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}

	return nil
}

// Reconcile creates or updates the main service
func (kor *KubernetesOAMRouter) Reconcile(canary *flaggerv1.Canary) error {
	c := kor.innerK8sRouter
	apexName, _, _ := canary.GetServiceNames()

	// main svc also elect all the component pod
	err := c.reconcileService(canary, apexName, kor.componentName, canary.Spec.Service.Apex)
	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}

	return nil
}

// OAM Router doesn't do finalize
func (kor *KubernetesOAMRouter) Finalize(canary *flaggerv1.Canary) error {
	return fmt.Errorf("OAM router doesn't do finalize")
}
