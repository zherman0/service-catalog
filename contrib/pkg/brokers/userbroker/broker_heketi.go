/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package userbroker

import (
	"fmt"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)


// doHeketiProvision Creates a web server Heketi service instance.
// The instance is made up of 1 pod (running Heketi) and 1 secret (containing admin creds)
func doHeketiProvision(instanceID, ns string) (error) {
	if ns == "" {
		glog.Error("Request Context does not contain a Namespace")
		return fmt.Errorf("Namespace not detected in Request")
	}
	cs, err := getKubeClient()
	if err != nil {
		return err
	}
	pod, sec := newHeketiInstanceResources(instanceID)
	sec, err = cs.CoreV1().Secrets(ns).Create(sec)
	if err != nil {
		glog.Errorf("Failed to Create secret: %v", err)
		return err
	}
	pod, err = cs.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		cs.CoreV1().Secrets(ns).Delete(sec.Name, &metav1.DeleteOptions{})
		glog.Errorf("Failed to Create pod: %q", err)
		return err
	}
	glog.Infof("Provisioned Instance Pod %q (ns: %s)", pod.Name, ns)
	return nil
}

// doHeketiDeprovision Deletes a Heketi service instance
// Deprovisioning deletes the Heketi pod and secret.
// On error, does not delete instance so as not to orphan resources.
func doHeketiDeprovision(instanceID, ns string) error {
	if ns == "" {
		glog.Error("Request Context does not contain a Namespace")
		return fmt.Errorf("Namespace not detected in Request")
	}
	cs, err := getKubeClient()
	if err != nil {
		return err
	}
	glog.Infof("Deleting Instance Pod (ID: %v)", instanceID)
	errPod := cs.CoreV1().Pods(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: INST_RESOURCE_LABEL_NAME + "=" + instanceID,
	})
	if err != nil {
		glog.Errorf("Error deleting Instance Pod (ID: %v): %v", instanceID, err)
	}
	glog.Infof("Deleting Instance Secret (ID: %v)", instanceID)
	err = cs.CoreV1().Secrets(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: INST_RESOURCE_LABEL_NAME + "=" + instanceID,
	})
	if err != nil {
		glog.Errorf("Error deleting Instance Secret (ID: %v): %v", instanceID, err)
		if errPod != nil {
			err = fmt.Errorf("Errors deprovisioning instance %q\n%v\n%v", instanceID, errPod, err)
		}
		return err
	}
	return nil
}

// doHeketiBind returns the Heketi pod IP and Port
// TODO implement db user creation via `mgo` package
func doHeketiBind(instanceID, ns string) (string, int32, error) {
	ip, port, err := getHeketiPodIP(instanceID, ns)
	if err != nil {
		return "", 0, err
	}
	return ip, port, nil
}

// doHeketiUnbind does nothing.
// TODO implement db user deletion via `mgo` package
func doHeketiUnbind() (string, error) {
	return "Heketi Unbind not implemented.", nil
}

// getHeketiPodIP uses a k8s api client to get the pod and extract its IP and Port
func getHeketiPodIP(instanceID, ns string) (string, int32, error) {
	cs, err := getKubeClient()
	if err != nil {
		return "", 0, err
	}
	pods, err := cs.CoreV1().Pods(ns).List(metav1.ListOptions{
		LabelSelector: INST_RESOURCE_LABEL_NAME+ "=" + instanceID,
	})
	if err != nil {
		return "", 0, err
	}
	return pods.Items[0].Status.PodIP, pods.Items[0].Spec.Containers[0].Ports[0].ContainerPort, nil
}


// newHeketiInstanceResources returns a Heketi pod and secret definition
func newHeketiInstanceResources(instanceID string) (*v1.Pod, *v1.Secret) {
	secretName := "heketi-" + instanceID + "-secret"
	isOptional := false

	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "heketi-" + instanceID,
			Labels: map[string]string{
				INST_RESOURCE_LABEL_NAME: instanceID,
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "heketi",
					Image:           "heketi/heketi:dev",
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
					Args: []string{},
					Ports: []v1.ContainerPort{
						{
							Name:          "heketi",
							ContainerPort: 8080,
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

			},
		}
}
