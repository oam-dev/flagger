package oam

import (
	"context"
	"fmt"
	"sort"

	v1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2/applicationconfiguration"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

func GetUnstructured(ctx context.Context, kind, apiVersion, name, namespace string, c client.Client) (*unstructured.Unstructured, error) {
	var workload unstructured.Unstructured
	workload.SetKind(kind)
	workload.SetAPIVersion(apiVersion)
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &workload); err != nil {
		return nil, err
	}
	return &workload, nil
}

type Revisions []v1.ControllerRevision

func (r Revisions) Len() int           { return len(r) }
func (r Revisions) Less(i, j int) bool { return r[i].Revision > r[j].Revision }
func (r Revisions) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }

func FindSourceWorkload(canary *flaggerv1.Canary, c client.Client, kubeClient kubernetes.Interface) (*unstructured.Unstructured, error) {
	ctx := context.Background()
	workload, err := GetUnstructured(ctx, canary.Spec.TargetRef.Kind, canary.Spec.TargetRef.APIVersion, canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, c)
	if err != nil {
		return nil, fmt.Errorf("get OAM workload %s.%s err %v", canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, err)
	}
	lb := workload.GetLabels()
	if lb == nil {
		return nil, fmt.Errorf("didn't find any labels from OAM workload %v", canary.Spec.TargetRef)
	}
	componentName := lb[oam.LabelAppComponent]
	revisionList, err := kubeClient.AppsV1().ControllerRevisions(canary.Namespace).List(ctx,
		metav1.ListOptions{LabelSelector: labels.SelectorFromValidatedSet(
			map[string]string{applicationconfiguration.ControllerRevisionComponentLabel: componentName},
		).String()})
	if err != nil {
		return nil, fmt.Errorf("get revision from component %s err %v", componentName, err)
	}
	var r Revisions = revisionList.Items
	sort.Sort(r)
	for _, rev := range revisionList.Items {
		if rev.Name == canary.Spec.TargetRef.Name {
			continue
		}
		workload, err := GetUnstructured(ctx, canary.Spec.TargetRef.Kind, canary.Spec.TargetRef.APIVersion, rev.Name, canary.Spec.TargetRef.Namespace, c)
		if err != nil {
			//TODO(wonderflow): handle errors besides not-exist
			continue
		}
		// Searching from latest revision to oldest, and use the latest running workload as source workload.
		return workload, nil
	}
	return nil, fmt.Errorf("source workload of %v not found", componentName)
}
