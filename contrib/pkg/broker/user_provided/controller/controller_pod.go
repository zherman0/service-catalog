package controller

import (
	"errors"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

const (
	MONGO_INITDB_ROOT_USERNAME_NAME  = "MONGO_INITDB_ROOT_USERNAME" // DO NOT CHANGE - must match docker image variable
	MONGO_INITDB_ROOT_USERNAME_VALUE = "admin"
	MONGO_INITDB_ROOT_PASSWORD_NAME  = "MONGO_INITDB_ROOT_PASSWORD" // DO NOT CHANGE - must match docker image variable
	MONGO_INITDB_ROOT_PASSWORD_VALUE = "password"
	INST_RESOURCE_LABEL_NAME         = "instanceId"
)

func provisionDBInstance(instanceID, ns string) (string, error) {
	cs, err := getKubeClient()
	if err != nil {
		return "", err
	}
	if ns == "" {
		glog.Error("Request Context does not contain a Namespace")
		return "", errors.New("Namespace not detected in Request")
	}
	pod, sec := newDatabaseInstance(instanceID)
	sec, err = cs.CoreV1().Secrets(ns).Create(sec)
	if err != nil {
		glog.Errorf("Failed to Create secret: %v", err)
		return "", nil
	}
	pod, err = cs.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		cs.CoreV1().Secrets(ns).Delete(sec.Name, &metav1.DeleteOptions{})
		glog.Errorf("Failed to Create pod: %q", err)
		return "", err
	}
	glog.Infof("Provisioned Instance Pod %q (ns: %s)", pod.Name, ns)
	return pod.Namespace, nil
}

func deprovisionDBInstance(instanceID, ns string) error {
	cs, err := getKubeClient()
	if err != nil {
		return err
	}
	glog.Infof("Deleting Instance Pod (ID: %v)", instanceID)
	err = cs.CoreV1().Pods(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: INST_RESOURCE_LABEL_NAME + "=" + instanceID,
	})
	if err != nil {
		glog.Errorf("Error deleting Instance Pod (ID: %v): %v", instanceID, err)
		return err
	}
	glog.Infof("Deleting Instance Secret (ID: %v)", instanceID)
	err = cs.CoreV1().Secrets(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: INST_RESOURCE_LABEL_NAME + "=" + instanceID,
	})
	if err != nil {
		glog.Errorf("Error deleting Instance Secret (ID: %v): %v", instanceID, err)
		return err
	}
	return nil
}

func getInstancePodIP(instance *userProvidedServiceInstance) (string, int32, error) {
	cs, err := getKubeClient()
	if err != nil {
		return "", 0, err
	}
	pods, err := cs.CoreV1().Pods(instance.Namespace).List(metav1.ListOptions{
		LabelSelector: INST_RESOURCE_LABEL_NAME,
		FieldSelector: instance.Id,
	})
	if err != nil {
		return "", 0, err
	}
	return pods.Items[0].Status.PodIP, pods.Items[0].Spec.Containers[0].Ports[0].ContainerPort, nil
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
func newDatabaseInstance(instanceID string) (*v1.Pod, *v1.Secret) {
	secretName := "db-" + instanceID + "-secret"
	isOptional := false

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mongo-" + instanceID,
			Labels: map[string]string{
				INST_RESOURCE_LABEL_NAME: instanceID,
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "mongo",
					Image:           "docker.io/mongo:latest",
					ImagePullPolicy: "IfNotPresent",
					EnvFrom: []v1.EnvFromSource{
						{
							SecretRef: &v1.SecretEnvSource{
								LocalObjectReference: v1.LocalObjectReference{
									Name: secretName,
								},
								Optional: &isOptional,
							},
						},
					},
					Args: []string{"mongod"},  // TODO this is where createUser cmd will be appended
					Ports: []v1.ContainerPort{
						{
							Name:          "mongodb",
							ContainerPort: 27017,
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "admin-credentials",
					VolumeSource: v1.VolumeSource{
						Secret: &v1.SecretVolumeSource{
							SecretName: secretName,
						},
					},
				},
			},
		},
	},
		&v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: secretName,
				Labels: map[string]string{
					INST_RESOURCE_LABEL_NAME: instanceID,
				},
			},
			StringData: map[string]string{
				MONGO_INITDB_ROOT_USERNAME_NAME: MONGO_INITDB_ROOT_USERNAME_VALUE,
				MONGO_INITDB_ROOT_PASSWORD_NAME: MONGO_INITDB_ROOT_PASSWORD_VALUE,
			},
		}
}
