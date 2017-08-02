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

// errNoSuchInstance implements the Error interface.
// This struct handles the common error of an unrecogonzied instanceID
// and should be used as a returned error value.
// e.g. return errNoSuchInstance{instanceID: <id>}
type errNoSuchInstance struct {
	instanceID string
}

func (e errNoSuchInstance) Error() string {
	return fmt.Sprintf("No such instance with ID %s", e.instanceID)
}

// userProvidedServiceInstance contains identifying data for each existing service instance.
//   `Id` is the instanceID
//	 `Namespace` is the k8s namespace provided in the CreateServiceInstanceReqeust.ContextProfile.Namespace
//   `ServiceID` is the service's associated id.
//	 `Credential` is the binding credential created during Bind()
type userProvidedServiceInstance struct {
	Id         string                   `json:"id"`
	Namespace  string                   `json:"namespace"`
	ServiceID  string                   `json:"serviceid"`
	Credential *brokerapi.Credential    `json:"credential"`
}

// userProvidedController implements the OSB API and represents the actual Broker.
//   `rwMutex` controls concurrent R and RW access.
//   `instanceMap` should take instanceIDs as the key and maps to that ID's userProvidedServiceInstance
type userProvidedController struct {
	rwMutex     sync.RWMutex
	instanceMap map[string]*userProvidedServiceInstance
}

const (
	// Service IDs should always be constants.  The variable names should be prefixed with "serviceid"
	// serviceidUserProvided is the basic demo. It provides no actual service
	serviceidUserProvided string = "4f6e6cf6-ffdd-425f-a2c7-3c9258ad2468"
	// serviceidDatabasePod  provides an instance of a mongo db
	serviceidDatabasePod  string = "database-1"
)

// CreateController initializes the service broker.  This function is called by server.Start()
func CreateController() controller.Controller {
	var instanceMap = make(map[string]*userProvidedServiceInstance)
	return &userProvidedController{
		instanceMap: instanceMap,
	}
}

// Catalog is an OSB method.  It returns a slice of services.
// New services should be specified here.
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

// CreateServiceInstance is an OSB method.  It handles provisioning of service instances
// as determined by the instance's serviceID.
// New services should be added as a new case in the switch.
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

// GetServiceInstance is an OSB method. It gets an instance by the instance's ID and returns it as a json string.
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

// RemoveServiceInstance is an OSB method.  It handles deprovisioning determined by the serviceID.
// New services should be added as a new case in the switch.
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
		// Do nothing.
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

// Bind is an OSB method.  It handles bindings as determined by the serviceID.
// New services should be added as a new case in the switch.
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
			"mongoInstanceIp": ip,
			"mongoInstancePort": port,
		}
	}
	c.instanceMap[instanceID].Credential = newCredential
	glog.Infof("Bound Instance: %q", instanceID)
	return &brokerapi.CreateServiceBindingResponse{Credentials: *newCredential}, nil
}

// UnBind is an OSB method.  It handles credentials deletion relative to each service.
// New services should be added as a new case in the switch.
//TODO implement DB unbinding (delete user, etc)
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
		// Do nothing
	case serviceidDatabasePod:
		doDBUnbind()
	}
	glog.Infof("Unbound Instance: %q", instanceID)
	return nil
}
