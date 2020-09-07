module github.com/weaveworks/flagger

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.0.3
	github.com/aws/aws-sdk-go v1.30.19
	github.com/crossplane/crossplane-runtime v0.9.0 // indirect
	github.com/crossplane/oam-kubernetes-runtime v0.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/go-logr/logr v0.2.1 // indirect
	github.com/google/go-cmp v0.4.0
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.5.1
	github.com/rs/xid v1.2.1 // indirect
	github.com/stretchr/testify v1.5.1
	go.uber.org/zap v1.16.0
	golang.org/x/net v0.0.0-20200904194848-62affa334b73 // indirect
	gopkg.in/h2non/gock.v1 v1.0.15
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.18.8
	k8s.io/code-generator v0.18.8
	k8s.io/klog/v2 v2.3.0 // indirect
	sigs.k8s.io/controller-runtime v0.6.2 // indirect
)

replace k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
