package canary

import (
	"context"
	"encoding/json"
	"fmt"

	velav1alpha1 "github.com/oam-dev/kubevela/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type oamWorkload struct {
	w *unstructured.Unstructured
}

func (o *oamWorkload) GetSpec() map[string]interface{} {
	obg, found, _ := unstructured.NestedMap(o.w.Object, "spec")
	if !found {
		return nil
	}
	return obg
}

func (o *oamWorkload) GetStatusObservedGeneration() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "observedGeneration")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "spec", "replicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetStatusReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "replicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetStatusUpdatedReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "updatedReplicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetStatusAvailableReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "availableReplicas")
	if !found {
		return nil
	}
	return &obg
}

// IsWorkloadReady determines if a workload is ready by checking the status conditions
// if a workload has exceeded the progress deadline it returns a non retriable error
func (orc *OAMRolloutController) IsWorkloadReady(workload *unstructured.Unstructured, deadline int) (bool, error) {
	switch workload.GetKind() {
	case "Deployment":
		deployData, err := workload.MarshalJSON()
		if err != nil {
			return false, err
		}
		var deploy appsv1.Deployment
		err = json.Unmarshal(deployData, &deploy)
		if err != nil {
			return false, err
		}
		return IsDeploymentReady(&deploy, deadline)
	case "PodSpecWorkload":
		// unmarshal to podSpec
		podSpecData, err := workload.MarshalJSON()
		if err != nil {
			return false, err
		}
		var podSpec velav1alpha1.PodSpecWorkload
		err = json.Unmarshal(podSpecData, &podSpec)
		if err != nil {
			return false, err
		}
		// get the deployment
		var deployName string
		for _, res := range podSpec.Status.Resources {
			if res.Kind == "Deployment" {
				deployName = res.Name
			}
		}
		if len(deployName) == 0 {
			return true, fmt.Errorf("Deployment not found for podSpecWorkload %s", workload.GetName())
		}
		deploy, err := orc.kubeClient.AppsV1().Deployments(workload.GetNamespace()).Get(context.TODO(), deployName,
			metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		return IsDeploymentReady(deploy, deadline)
	}
	return true, nil
}
