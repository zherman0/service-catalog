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

package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/service-catalog/contrib/pkg/broker/controller"
	"github.com/kubernetes-incubator/service-catalog/pkg/brokerapi"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	//"k8s.io/apimachinery/pkg/api/errors"  // TODO decomment once we start using the k8s client
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)


type errNoSuchInstance struct {
	instanceID string
}

func (e errNoSuchInstance) Error() string {
	return fmt.Sprintf("no such instance with ID %s", e.instanceID)
}

type userProvidedServiceInstance struct {
	Name       string
	Credential *brokerapi.Credential
}

type userProvidedController struct {
	rwMutex     sync.RWMutex
	instanceMap map[string]*userProvidedServiceInstance
}

// CreateController creates an instance of a User Provided service broker controller.
func CreateController() controller.Controller {
	var instanceMap = make(map[string]*userProvidedServiceInstance)
	return &userProvidedController{
		instanceMap: instanceMap,
	}
}

// TODO add our DB service here
// TODO EITHER: figure out how to pass namespace to here  (track down brokerapi.CreateServiceInstanceRequest.Parameters)
// TODO     OR: See if we can create pod in the default namespace
func (c *userProvidedController) Catalog() (*brokerapi.Catalog, error) {
	glog.Info("Controller Catalog Call")
	return &brokerapi.Catalog{
		Services: []*brokerapi.Service{
			{
				Name:        "user-provided-service",
				ID:          "4f6e6cf6-ffdd-425f-a2c7-3c9258ad2468",
				Description: "A user provided service",
				Plans: []brokerapi.ServicePlan{{
					Name:        "default",
					ID:          "86064792-7ea2-467b-af93-ac9694d96d52",
					Description: "Sample plan description",
					Free:        true,
				},
				},
				Bindable: true,
			},
		},
	}, nil
}

func (c *userProvidedController) CreateServiceInstance(
	id string,
	req *brokerapi.CreateServiceInstanceRequest,
) (*brokerapi.CreateServiceInstanceResponse, error) {
	credString, ok := req.Parameters["credentials"]
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()
	if ok {
		jsonCred, err := json.Marshal(credString)
		if err != nil {
			glog.Errorf("Failed to marshal credentials: %v", err)
			return nil, err
		}
		var cred brokerapi.Credential
		err = json.Unmarshal(jsonCred, &cred)

		c.instanceMap[id] = &userProvidedServiceInstance{
			Name:       id,
			Credential: &cred,
		}
	} else {
		c.instanceMap[id] = &userProvidedServiceInstance{
			Name: id,
			Credential: &brokerapi.Credential{
				"special-key-1": "special-value-1",
				"special-key-2": "special-value-2",
			},
		}
	}

	// Pod Provisioning Code
	cs, err := getKubeClient()
	if err != nil {
		return nil, err
	}
	ns := "test-ns"
	pod := newDatabasePod(ns)
	pod, err = cs.CoreV1().Pods(ns).Create(pod)
	if err != nil {
		glog.Error("Failed to Create pod: %q", err)
	} else {
		pjson, _ := json.MarshalIndent(pod, "", "    ")
		glog.Infof("New Pod (ns: %s): %v", ns, string(pjson))
	}
	glog.Infof("Created User Provided Service Instance:\n%v\n", c.instanceMap[id])
	return &brokerapi.CreateServiceInstanceResponse{}, nil
}

// TODO implement pod get
func (c *userProvidedController) GetServiceInstance(id string) (string, error) {
	return "", errors.New("Unimplemented")
}

// TODO implement pod deletion
func (c *userProvidedController) RemoveServiceInstance(id string) (*brokerapi.DeleteServiceInstanceResponse, error) {
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()
	_, ok := c.instanceMap[id]
	if ok {
		delete(c.instanceMap, id)
		return &brokerapi.DeleteServiceInstanceResponse{}, nil
	}

	return &brokerapi.DeleteServiceInstanceResponse{}, nil
}

// TODO implement DB binding
func (c *userProvidedController) Bind(
	instanceID,
	bindingID string,
	req *brokerapi.BindingRequest,
) (*brokerapi.CreateServiceBindingResponse, error) {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()
	instance, ok := c.instanceMap[instanceID]
	if !ok {
		return nil, errNoSuchInstance{instanceID: instanceID}
	}
	cred := instance.Credential
	return &brokerapi.CreateServiceBindingResponse{Credentials: *cred}, nil
}


//TODO implement DB unbinding
func (c *userProvidedController) UnBind(instanceID string, bindingID string) error {
	// Since we don't persist the binding, there's nothing to do here.
	return nil
}

func (c *userProvidedController) Debug() (string, error) {
	glog.Warning("[DEBUG] External debug request.")
	cs, err := getKubeClient()
	if err != nil {
		return "", err
	}
	msg, err := cs.ServerVersion()
	return  msg.String(), err
}

func getKubeClient() (*kubernetes.Clientset, error){
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
func newDatabasePod(ns string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind: "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "debian",	// to mongo
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container {
				{
					Name: "debian",					  // to "mongo"
					Image: "docker.io/debian:latest", // to "docker.io/mongo"
					ImagePullPolicy: "IfNotPresent",
					Ports: []v1.ContainerPort{
						{
							Name: "mongodb",
							ContainerPort: 27017, // mongoDB port
						},
					},
					Command: []string {"/bin/bash"},
					Args: []string {"-c", "while : ; do sleep 10; done"},
				},
			},
		},
	}
}