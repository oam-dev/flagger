package canary

import (
	"errors"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
)

type Factory struct {
	kubeConfig    *rest.Config
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker Tracker
	labels        []string
}

func NewFactory(kubeConfig *rest.Config,
	kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	configTracker Tracker,
	labels []string,
	logger *zap.SugaredLogger) *Factory {
	return &Factory{
		kubeConfig:    kubeConfig,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		logger:        logger,
		configTracker: configTracker,
		labels:        labels,
	}
}

func (factory *Factory) Controller(kind string) (Controller, error) {
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

	switch kind {
	case "DaemonSet":
		return daemonSetCtrl, nil
	case "Deployment":
		return deploymentCtrl, nil
	case "Service":
		return serviceCtrl, nil
	case "rolling":
		return NewRollingController(factory.kubeConfig,
			factory.kubeClient,
			factory.flaggerClient,
			factory.logger,
			factory.configTracker,
			factory.labels)
	case "inplace":
		//TODO implement inplace controller
		return nil, errors.New("inplace strategy not implemented")
	default:
		return deploymentCtrl, nil
	}
}
