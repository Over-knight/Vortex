package vortexkube

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type K8sClient struct {
	Clientset kubernetes.Clientset
}

func NewK8sClient() (*K8sClient, error) {
	// Try in-cluster config first (when running inside kubernetes)
	config, err := rest.InClusterConfig()

	//fallback to kube-config file (when running locally)
	if err != nil {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	//create the clientset from config
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &K8sClient{Clientset: *clientset}, nil
}
