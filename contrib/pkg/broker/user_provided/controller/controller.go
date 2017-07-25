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
	Name         string    				`json:"name"`
	ServiceID    string    				`json:"serviceid"`
	Credential   *brokerapi.Credential 	`json:"credential"`
	PodName      string    				`json:"podname"`
	PodNamespace string 				`json:"podnamespace"`
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

// TODO add our DB service here
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
	glog.Info("[DEBUG] New CreateServiceInstanceRequest (ID: %q)", id)


	if _, ok := c.instanceMap[id]; ok {
		return nil, fmt.Errorf("Instance %q already exists", id)
	}
	// Create New Instance
	c.instanceMap[id] = &userProvidedServiceInstance{
		Name:      id,
		ServiceID: req.ServiceID,
	}

	// Extract credentials from request or generate dummy
	credString, ok := req.Parameters["credentials"]
	if ok {
		jsonCred, err := json.Marshal(credString)
		if err != nil {
			glog.Errorf("Failed to marshal credentials: %v", err)
			return nil, err
		}
		var cred brokerapi.Credential
		err = json.Unmarshal(jsonCred, &cred)
		c.instanceMap[id].Credential =  &cred
	} else {
		c.instanceMap[id].Credential = &brokerapi.Credential{
			"special-key-1": "special-value-1",
			"special-key-2": "special-value-2",
		}
	}

	// Do provisioning logic based on service id
	switch c.instanceMap[id].ServiceID {
	case serviceidUserProvided:
		break
	case serviceidDatabasePod:
		name, ns, err := provisionInstancePod(id, req.ContextProfile.Namespace)
		if err != nil {
			return nil, err
		}
		c.instanceMap[id].PodName = name
		c.instanceMap[id].PodNamespace = ns

	}
	glog.Infof("Created User Provided Service Instance: %q", c.instanceMap[id].Name)
	return nil, nil
}

func (c *userProvidedController) GetServiceInstance(id string) (string, error) {
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()

	// DEBUG
	glog.Infof("[DEBUG] GetServiceInstance, ID: %q", id)

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
	glog.Infof("[DEBUG] RemoveServiceInstance %q", id)

	if _, ok := c.instanceMap[id]; ! ok {
		return nil, errNoSuchInstance{instanceID: id}
	}
	switch c.instanceMap[id].ServiceID {
	case serviceidUserProvided:
		break
	case serviceidDatabasePod:
		if err := deprovisionInstancePod(c.instanceMap[id].PodName, c.instanceMap[id].PodNamespace); err != nil {
			errmsg := fmt.Errorf("Error deleting intance pod %q (ns: %q): %v",
				c.instanceMap[id].PodName, c.instanceMap[id].PodNamespace, err)
			glog.Error(errmsg)
			return nil, errmsg
		}
	}
	delete(c.instanceMap, id)
	return nil, nil
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
	glog.Info("[DEBUG] External debug request.")
	cs, err := getKubeClient()
	if err != nil {
		return "", err
	}
	msg, err := cs.ServerVersion()
	return msg.String(), err
}
