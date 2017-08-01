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
	"fmt"
	"sync"

	"github.com/golang/glog"
	"github.com/kubernetes-incubator/service-catalog/contrib/pkg/broker/controller"
	"github.com/kubernetes-incubator/service-catalog/pkg/brokerapi"
)

type errNoSuchInstance struct {
	instanceID string
}

func (e errNoSuchInstance) Error() string {
	return fmt.Sprintf("No such instance with ID %s", e.instanceID)
}

type userProvidedServiceInstance struct {
	Id         string                   `json:"id"`
	Namespace  string                   `json:"namespace"`
	ServiceID  string                   `json:"serviceid"`
	Credential *brokerapi.Credential    `json:"credential"`
}

type userProvidedController struct {
	rwMutex     sync.RWMutex
	instanceMap map[string]*userProvidedServiceInstance
}

const (
	serviceidUserProvided string = "4f6e6cf6-ffdd-425f-a2c7-3c9258ad2468"
	serviceidDatabasePod  string = "database-1"
)

// CreateController creates an instance of a User Provided service broker controller.
func CreateController() controller.Controller {
	var instanceMap = make(map[string]*userProvidedServiceInstance)
	return &userProvidedController{
		instanceMap: instanceMap,
	}
}

func (c *userProvidedController) Catalog() (*brokerapi.Catalog, error) {
	glog.Info("[DEBUG] Handling Catalog Request")
	return &brokerapi.Catalog{
		Services: []*brokerapi.Service{
			{
				Name:        "user-provided-service",
				ID:          serviceidUserProvided,
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
			{
				Name:        "database-service",
				ID:          serviceidDatabasePod,
				Description: "A Hacky little pod service.",
				Plans: []brokerapi.ServicePlan{
					{
						Name:        "default",
						ID:          "default",
						Description: "There is only one, and this is it.",
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
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()

	//DEBUG
	glog.Info("[DEBUG] Create ServiceInstance Request (ID: %q)", id)

	if _, ok := c.instanceMap[id]; ok {
		return nil, fmt.Errorf("Instance %q already exists", id)
	}
	// Create New Instance
	newInstance := &userProvidedServiceInstance{
		Id:        id,
		ServiceID: req.ServiceID,
		Namespace: req.ContextProfile.Namespace,
	}
	// Do provisioning logic based on service id
	switch newInstance.ServiceID {
	case serviceidUserProvided:
		break
	case serviceidDatabasePod:
		err := doDBProvision(id, newInstance.Namespace)
		if err != nil {
			return nil, err
		}
	}
	glog.Infof("Provisioned Instance %q in Namespace %q", newInstance.Id, newInstance.Namespace)
	c.instanceMap[id] = newInstance
	return nil, nil
}

func (c *userProvidedController) GetServiceInstance(id string) (string, error) {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()

	// DEBUG
	glog.Infof("[DEBUG] Get ServiceInstance Request (ID: %q)", id)

	if _, ok := c.instanceMap[id]; ! ok {
		return "", errNoSuchInstance{instanceID: id }
	}
	instance, _ := json.Marshal(c.instanceMap[id])
	return string(instance), nil
}

func (c *userProvidedController) RemoveServiceInstance(id string) (*brokerapi.DeleteServiceInstanceResponse, error) {
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()

	// DEBUG
	glog.Infof("[DEBUG] Remove ServiceInstance Request (ID: %q)", id)

	if _, ok := c.instanceMap[id]; ! ok {
		return nil, errNoSuchInstance{instanceID: id}
	}
	switch c.instanceMap[id].ServiceID {
	case serviceidUserProvided:
		break
	case serviceidDatabasePod:
		if err := doDBDeprovision(id, c.instanceMap[id].Namespace); err != nil {
			err = fmt.Errorf("Error deprovisioning instance %q, %v", id, err)
			glog.Error(err)
			return nil, err
		}
	}
	glog.Infof("Deprovisioned Instance: %q", c.instanceMap[id].Id)
	delete(c.instanceMap, id)
	return nil, nil
}

// TODO implment bindMap to track db bindings (user, bindId, etc.)
func (c *userProvidedController) Bind(
	instanceID,
	bindingID string,
	req *brokerapi.BindingRequest,
) (*brokerapi.CreateServiceBindingResponse, error) {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()

	// DEBUG
	glog.Infof("[DEBUG] Bind ServiceInstance Request (ID: %q)", instanceID)

	instance, ok := c.instanceMap[instanceID]
	if !ok {
		return nil, errNoSuchInstance{instanceID: instanceID}
	}
	var newCredential *brokerapi.Credential
	switch c.instanceMap[instanceID].ServiceID {
	case serviceidUserProvided:
		// Extract credentials from request or generate dummy
		newCredential = &brokerapi.Credential{
			"special-key-1": "special-value-1",
			"special-key-2": "special-value-2",
		}
	case serviceidDatabasePod:
		ip, port, err := doDBBind(instanceID, instance.Namespace)
		if err != nil {
			return nil, err
		}
		newCredential = &brokerapi.Credential{
			"mongo_svc_ip_port": fmt.Sprintf("%s:%d", ip, port),
		}
	}
	instance.Credential = newCredential
	glog.Infof("Bound Instance: %q", instanceID)
	return &brokerapi.CreateServiceBindingResponse{Credentials: *newCredential}, nil
}

//TODO implement DB unbinding
func (c *userProvidedController) UnBind(instanceID string, bindingID string) error {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()
	// DEBUG
	glog.Infof("[DEBUG] Unind ServiceInstance Request (ID: %q)", instanceID)

	instance, ok := c.instanceMap[instanceID]
	if !ok {
		return errNoSuchInstance{instanceID: instanceID}
	}
	switch instance.ServiceID {
	case serviceidUserProvided:
		// nothing to do
	case serviceidDatabasePod:
		doDBUnbind()
	}
	glog.Infof("Unbound Instance: %q", instanceID)
	return nil
}
