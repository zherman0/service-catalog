package controller

import (
	"errors"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	"fmt"
)

func provisionInstancePod(nameSuffix, ns string) (*metav1.ObjectMeta, error) {
	cs, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	if ns == "" {
		glog.Error("Request Context does not contain a Namespace")
		return nil, errors.New("Namespace not detected in Request")
	}
	pod := newDatabasePod(nameSuffix, ns)
	glog.Infof("Deploying Database PodMeta %q (ns: %q)", pod.Name, pod.Namespace )
	pod, err = cs.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		glog.Errorf("Failed to Create pod: %q", err)
		return nil, err
	} else {
		glog.Infof("Provisioned Instance Pod %q (ns: %s)", pod.Name, ns)
	}
	return &pod.ObjectMeta, nil
}

func deprovisionInstancePod(podMeta *metav1.ObjectMeta) error {
	cs, err := getKubeClient()
	if err != nil {
		return err
	}
	glog.Infof("Deleting Instance pod %q (ns: %s)", podMeta.Name, podMeta.Namespace)
	err = cs.CoreV1().Pods(podMeta.Namespace).Delete(podMeta.Name, &metav1.DeleteOptions{})
	if ! apierrs.IsNotFound(err) {
		return err
	}
	return nil
}

func getInstancePod() (*v1.Pod, error) {
	return nil, fmt.Errorf("getInstancePod not implemented")
}

func getKubeClient() (*kubernetes.Clientset, error) {
	glog.Info("Getting API Client config")
	kubeClientConfig, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	glog.Info("Creating new Kubernetes Clientset")
	cs, err := kubernetes.NewForConfig(kubeClientConfig)
	return cs, err
}

// TODO find a DB image to use here
// TODO figure out how to get the credentials
// TODO currently just a debian pod for testing
// TODO probably better to use a Deployment so we can keep it behind a known IP.
// TODO DB and webserver pod templates in kubernetes/examples.  Might be useful
func newDatabasePod(instanceID, ns string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodMeta",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "podinst-" + instanceID , // to mongo // TODO generate unique but identifiable names
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "debian",                  // to "mongo"
					Image:           "docker.io/debian:latest", // to "docker.io/mongo"
					ImagePullPolicy: "IfNotPresent",
					Ports: []v1.ContainerPort{
						{
							Name:          "mongodb",
							ContainerPort: 27017, // mongoDB port
						},
					},
					Command: []string{"/bin/bash"},
					Args:    []string{"-c", "while : ; do sleep 10; done"},
				},
			},
		},
	}
}
