package canary

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	restclient "k8s.io/client-go/rest"

	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

type Factory struct {
	kubeCfg       *restclient.Config
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker Tracker
	labels        []string
}

func NewFactory(kubeCfg *restclient.Config,
	kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	configTracker Tracker,
	labels []string,
	logger *zap.SugaredLogger) *Factory {
	return &Factory{
		kubeCfg:       kubeCfg,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		logger:        logger,
		configTracker: configTracker,
		labels:        labels,
	}
}

func (factory *Factory) Controller(kind string) Controller {
	deploymentCtrl := &DeploymentController{
		logger:        factory.logger,
		kubeClient:    factory.kubeClient,
		flaggerClient: factory.flaggerClient,
		labels:        factory.labels,
		configTracker: factory.configTracker,
	}
	daemonSetCtrl := &DaemonSetController{
		logger:        factory.logger,
		kubeClient:    factory.kubeClient,
		flaggerClient: factory.flaggerClient,
		labels:        factory.labels,
		configTracker: factory.configTracker,
	}
	serviceCtrl := &ServiceController{
		logger:        factory.logger,
		kubeClient:    factory.kubeClient,
		flaggerClient: factory.flaggerClient,
	}
	extDeploymentController := &ExtDeploymentController{
		deploymentCtrl,
	}
	switch kind {
	case "DaemonSet":
		return daemonSetCtrl
	case "Deployment":
		return extDeploymentController
	case "Service":
		return serviceCtrl
	default:
		return deploymentCtrl
	}
}
