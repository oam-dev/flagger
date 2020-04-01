package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/internal"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// KubernetesDefaultRouter is managing ClusterIP services
type ExtKubernetesDefaultRouter struct {
	innerK8sRouter *KubernetesDefaultRouter
}

// Initialize creates the primary and canary services
func (e *ExtKubernetesDefaultRouter) Initialize(canary *flaggerv1.Canary) error {
	primaryName, _, canaryName := canary.GetServiceNames()
	_, exist := internal.CanaryDistinguishLabelsExisted(canary)
	if exist {
		if internal.HasSourceTargetRef(canary) && !internal.IsInitializing(canary) {
			// not in initialization status, return
			return nil
		}

		if !internal.HasSourceTargetRef(canary) && !internal.IsInitialized(canary) {
			// only in initialized status, primary get ready
			return nil
		}

		// primary svc
		sourceName := internal.GetSourceName(canary)
		err := e.reconcileService(canary, primaryName, sourceName, true, canary.Spec.Service.Canary)
		if err != nil {
			return fmt.Errorf("reconcileService failed: %w", err)
		}

		// canary svc
		err = e.reconcileService(canary, canaryName, canary.Spec.TargetRef.Name, true, canary.Spec.Service.Canary)
		if err != nil {
			return fmt.Errorf("reconcileService failed: %w", err)
		}
		return nil
	}
	return e.innerK8sRouter.Initialize(canary)
}

// Reconcile creates or updates the main service
func (e *ExtKubernetesDefaultRouter) Reconcile(canary *flaggerv1.Canary) error {
	apexName, _, _ := canary.GetServiceNames()

	var err error
	// main svc
	primaryPodSelector := internal.GetSourceName(canary)
	if internal.IsFinished(canary) {
		err = e.reconcileService(canary, apexName, primaryPodSelector, false, canary.Spec.Service.Canary)
	} else {
		err = e.reconcileService(canary, apexName, primaryPodSelector, true, canary.Spec.Service.Canary)
	}

	if err != nil {
		return fmt.Errorf("reconcileService failed: %w", err)
	}

	return nil
}

// Finalize reverts the apex router if not owned by the Flagger controller.
func (e *ExtKubernetesDefaultRouter) Finalize(canary *flaggerv1.Canary) error {
	return e.innerK8sRouter.Finalize(canary)
}

// reconcile service to specific deployment
func (e *ExtKubernetesDefaultRouter) reconcileService(canary *flaggerv1.Canary, name string, deploymentName string, withDistinguish bool, metadata *flaggerv1.CustomMetadata) error {
	c := e.innerK8sRouter
	deploy, err := c.kubeClient.AppsV1().Deployments(canary.Namespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s get query error: %w", deploymentName, err)
	}

	selector := make(map[string]string)

	if withDistinguish {
		svcSelectLabels, _ := internal.CanaryDistinguishLabelsExisted(canary)
		for _, l := range svcSelectLabels {
			lTrim := strings.TrimSpace(l)
			res, ok := deploy.Spec.Template.Labels[lTrim]
			if !ok {
				return fmt.Errorf("service label %s missing", lTrim)
			}
			selector[lTrim] = res
		}
	} else {
		svcSelectLabels, exist := internal.CanaryGeneralLabelsExisted(canary)
		if !exist {
			selector = map[string]string{
				c.labelSelector: deploy.Spec.Template.Labels[c.labelSelector],
			}
		} else {
			for _, l := range svcSelectLabels {
				lTrim := strings.TrimSpace(l)
				res, ok := deploy.Spec.Template.Labels[lTrim]
				if !ok {
					return fmt.Errorf("service label %s missing", lTrim)
				}
				selector[lTrim] = res
			}
		}
	}

	portName := canary.Spec.Service.PortName
	if portName == "" {
		portName = "http"
	}

	targetPort := intstr.IntOrString{
		Type:   intstr.Int,
		IntVal: canary.Spec.Service.Port,
	}

	if canary.Spec.Service.TargetPort.String() != "0" {
		targetPort = canary.Spec.Service.TargetPort
	}

	// set pod selector and apex port
	svcSpec := corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: selector,
		Ports: []corev1.ServicePort{
			{
				Name:       portName,
				Protocol:   corev1.ProtocolTCP,
				Port:       canary.Spec.Service.Port,
				TargetPort: targetPort,
			},
		},
	}

	// set additional ports
	for n, p := range c.ports {
		cp := corev1.ServicePort{
			Name:     n,
			Protocol: corev1.ProtocolTCP,
			Port:     p,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: p,
			},
		}

		svcSpec.Ports = append(svcSpec.Ports, cp)
	}

	if metadata == nil {
		metadata = &flaggerv1.CustomMetadata{}
	}

	if metadata.Labels == nil {
		metadata.Labels = make(map[string]string)
	}
	metadata.Labels[c.labelSelector] = name

	if metadata.Annotations == nil {
		metadata.Annotations = make(map[string]string)
	}

	// create service if it doesn't exists
	svc, err := c.kubeClient.CoreV1().Services(canary.Namespace).Get(context.Background(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   canary.Namespace,
				Labels:      metadata.Labels,
				Annotations: metadata.Annotations,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(canary, schema.GroupVersionKind{
						Group:   flaggerv1.SchemeGroupVersion.Group,
						Version: flaggerv1.SchemeGroupVersion.Version,
						Kind:    flaggerv1.CanaryKind,
					}),
				},
			},
			Spec: svcSpec,
		}

		_, err := c.kubeClient.CoreV1().Services(canary.Namespace).Create(context.Background(), svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("service %s.%s create error: %w", svc.Name, canary.Namespace, err)
		}

		c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("Service %s.%s created", svc.GetName(), canary.Namespace)
		return nil
	} else if err != nil {
		return fmt.Errorf("service %s get query error: %w", name, err)
	}

	// update existing service pod selector and ports
	if svc != nil {
		sortPorts := func(a, b interface{}) bool {
			return a.(corev1.ServicePort).Port < b.(corev1.ServicePort).Port
		}

		// copy node ports from existing service
		for _, port := range svc.Spec.Ports {
			for i, servicePort := range svcSpec.Ports {
				if port.Name == servicePort.Name && port.NodePort > 0 {
					svcSpec.Ports[i].NodePort = port.NodePort
					break
				}
			}
		}

		portsDiff := cmp.Diff(svcSpec.Ports, svc.Spec.Ports, cmpopts.SortSlices(sortPorts))
		selectorsDiff := cmp.Diff(svcSpec.Selector, svc.Spec.Selector)
		if portsDiff != "" || selectorsDiff != "" {
			svcClone := svc.DeepCopy()
			svcClone.Spec.Ports = svcSpec.Ports
			svcClone.Spec.Selector = svcSpec.Selector
			_, err = c.kubeClient.CoreV1().Services(canary.Namespace).Update(context.Background(), svcClone, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("service %s update error: %w", name, err)
			}
			c.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
				Infof("Service %s updated", svc.GetName())
		}
	}

	return nil
}
