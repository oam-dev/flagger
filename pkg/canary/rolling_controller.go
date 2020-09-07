package canary

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	v12 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	autoscalingapi "k8s.io/api/autoscaling/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	oamworkload "github.com/weaveworks/flagger/pkg/oam"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RollingController struct {
	scaleClient   scale.ScalesGetter
	client        client.Client
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker Tracker
	labels        []string
}

func NewRollingController(cfg *rest.Config,
	kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	logger *zap.SugaredLogger,
	configTracker Tracker,
	labels []string,
) (*RollingController, error) {

	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	restMapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, err
	}
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(kubeClient.Discovery())
	scaleClient, err := scale.NewForConfig(cfg, restMapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	if err != nil {
		return nil, err
	}
	return &RollingController{
		scaleClient:   scaleClient,
		client:        c,
		logger:        logger,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		labels:        labels,
		configTracker: configTracker,
	}, nil
}

func (rdc *RollingController) Initialize(canary *flaggerv1.Canary) (err error) {

	sourceWorkload, err := oamworkload.FindSourceWorkload(canary, rdc.client, rdc.kubeClient)
	if err != nil {
		return err
	}

	if canary.Status.Phase == "" || canary.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !canary.SkipAnalysis() {
			if _, err := rdc.isWorkloadReady(sourceWorkload, canary.GetProgressDeadlineSeconds()); err != nil {
				return fmt.Errorf("IsPrimaryReady failed: %w", err)
			}
		}
	}

	// In there is no sourceRef, run default logic, Initialized -> Progressing need check canary changed.
	// If there is sourceRef, we think canary already changed, directly do Initialized -> Progressing translate.
	if canary.Status.Phase == flaggerv1.CanaryPhaseInitialized {
		if err := rdc.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing}); err != nil {
			return err
		}
	}

	return nil
}

func (rdc *RollingController) IsPrimaryReady(canary *flaggerv1.Canary) error {
	sourceWorkload, err := oamworkload.FindSourceWorkload(canary, rdc.client, rdc.kubeClient)
	if err != nil {
		return err
	}

	_, err = rdc.isWorkloadReady(sourceWorkload, canary.GetProgressDeadlineSeconds())
	return err
}

// isWorkloadReady determines if a workload is ready by checking the status conditions
// if a workload has exceeded the progress deadline it returns a non retriable error
func (rdc *RollingController) isWorkloadReady(workload *unstructured.Unstructured, deadline int) (bool, error) {
	oamWorkload := &OAMWorkload{w: workload}
	return oamWorkload.isWorkloadReady(deadline)
}

func (rdc *RollingController) Promote(canary *flaggerv1.Canary) error {
	ctx := context.Background()
	//TODO (wonderflow): we should do nothing here?
	if canary.Status.CanaryWeight != 100 {
		if err := rdc.SetStatusWeight(canary, 100); err != nil {
			return err
		}
		if new, err := rdc.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(ctx, canary.Name, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("canary %s.%s get query failed: %w", canary.Name, canary.Namespace, err)
		} else {
			new.DeepCopyInto(canary)
		}
	}
	return nil
}

func (rdc *RollingController) ScaleToZero(_ *flaggerv1.Canary) error {
	// We don't need to scale to zero for source workload
	return nil
}

func (rdc *RollingController) ScaleFromZero(_ *flaggerv1.Canary) error {
	// We don't need to scale from zero, only need for cold start from HPA
	return nil
}

func (rdc *RollingController) IsCanaryReady(canary *flaggerv1.Canary) (bool, error) {
	ctx := context.Background()
	workload, err := oamworkload.GetUnstructured(ctx, canary.Spec.TargetRef.Kind, canary.Spec.TargetRef.APIVersion, canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, rdc.client)
	if err != nil {
		return false, fmt.Errorf("get OAM workload %s.%s err %v", canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, err)
	}
	return rdc.isWorkloadReady(workload, canary.GetProgressDeadlineSeconds())
}

// GetMetadata returns selectorLabel and ports for creating service.
// The target ports are already specified in canary service spec,
// ports here are additional ones which were discovered automatically
func (rdc *RollingController) GetMetadata(cd *flaggerv1.Canary) (string, map[string]int32, error) {
	var label string
	//TODO find label from workloadDefinition
	ctx := context.Background()

	var ports map[string]int32
	if cd.Spec.Service.PortDiscovery {
		workload, err := oamworkload.GetUnstructured(ctx, cd.Spec.TargetRef.Kind, cd.Spec.TargetRef.APIVersion, cd.Spec.TargetRef.Name, cd.Spec.TargetRef.Namespace, rdc.client)
		if err != nil {
			return "", nil, fmt.Errorf("get OAM workload %s.%s err %v", cd.Spec.TargetRef.Name, cd.Spec.TargetRef.Namespace, err)
		}
		//Assume the workload will always using spec.template as PodTemplate if ports is discoverable
		obj, found, err := unstructured.NestedMap(workload.Object, "spec", "template")
		if found {
			data, err := json.Marshal(obj)
			if err != nil {
				return label, nil, nil
			}
			var template v12.PodTemplate
			err = json.Unmarshal(data, &template)
			if err != nil {
				return label, nil, nil
			}
			ports = getPorts(cd, template.Template.Spec.Containers)
		}
	}
	return label, ports, nil
}

func (rdc *RollingController) SyncStatus(canary *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	ctx := context.Background()
	workload, err := oamworkload.GetUnstructured(ctx, canary.Spec.TargetRef.Kind, canary.Spec.TargetRef.APIVersion, canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, rdc.client)
	if err != nil {
		return fmt.Errorf("get OAM workload %s.%s err %v", canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, err)
	}
	oamWorkload := &OAMWorkload{
		w: workload,
	}

	configs, err := rdc.configTracker.GetConfigRefs(canary)
	if err != nil {
		if !strings.Contains(err.Error(), "TargetRef.Kind invalid:") {
			return fmt.Errorf("GetConfigRefs failed: %w", err)
		}
		configs = nil
	}

	return syncCanaryStatus(rdc.flaggerClient, canary, status, oamWorkload.GetSpec(), func(cdCopy *flaggerv1.Canary) {
		cdCopy.Status.TrackedConfigs = configs
	})
}

func (rdc *RollingController) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	return setStatusFailedChecks(rdc.flaggerClient, cd, val)
}
func (rdc *RollingController) SetStatusWeight(cd *flaggerv1.Canary, val int) error {
	return setStatusWeight(rdc.flaggerClient, cd, val)
}
func (rdc *RollingController) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	return setStatusIterations(rdc.flaggerClient, cd, val)
}
func (rdc *RollingController) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	return setStatusPhase(rdc.flaggerClient, cd, phase)
}

// HasTargetChanged detect whether the workload has changed, in OAM rolling update mode,
// the target workload is here actually fixed revision, it won't change.
func (rdc *RollingController) HasTargetChanged(_ *flaggerv1.Canary) (bool, error) {
	return false, nil
}
func (rdc *RollingController) HaveDependenciesChanged(canary *flaggerv1.Canary) (bool, error) {
	changed, err := rdc.configTracker.HasConfigChanged(canary)
	if err != nil && strings.Contains(err.Error(), "TargetRef.Kind invalid:") {
		return false, nil
	}
	return changed, err
}

// Finalize will revert rolling update back, we just scale up here.
func (rdc *RollingController) Finalize(canary *flaggerv1.Canary) error {
	source, err := oamworkload.FindSourceWorkload(canary, rdc.client, rdc.kubeClient)
	if err != nil {
		return err
	}
	return rdc.scale(source.GetName(), source.GetNamespace(), source.GetAPIVersion(), int32(canary.Spec.Analysis.MaxReplicas))
}

// Scale sets the canary workload replicas
func (rdc *RollingController) scale(name, namespace, apiVersion string, replicas int32) error {
	ctx := context.Background()
	resourceList, err := rdc.kubeClient.Discovery().ServerResourcesForGroupVersion(apiVersion)
	if err != nil {
		return err
	}
	var resource, group string
	for _, r := range resourceList.APIResources {
		if strings.Contains(r.Name, "/") {
			continue
		}
		group = r.Group
		resource = r.Name
		break
	}
	sc := &autoscalingapi.Scale{}
	sc.Name = name
	sc.Namespace = namespace
	sc.Spec.Replicas = replicas

	sc, err = rdc.scaleClient.Scales(namespace).Update(ctx, schema.GroupResource{
		Group:    group,
		Resource: resource,
	}, sc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v by scale subresource failed: %w", name, namespace, replicas, err)
	}
	return nil
}
