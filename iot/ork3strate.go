package iot

import (
	"bytes"
	"encoding/json"
	"flag"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"log"
)

//func decodeDeploymentManifests(manifests [][]byte) (deps []appsv1.Deployment, err error) {
//	deployments := make([]appsv1.Deployment, len(manifests))
//	for i, manifest := range manifests {
//		dec := json.NewDecoder(bytes.NewReader(manifest))
//		err := dec.Decode(&deployments[i])
//		if err != nil {
//			return nil, err
//		}
//	}
//	return deployments, nil
//}

func decodeDeploymentManifests(manifests []byte) (deps appsv1.DeploymentList, err error) {
	var deployments appsv1.DeploymentList
	dec := json.NewDecoder(bytes.NewReader(manifests))
	err = dec.Decode(&deployments.Items)
	if err != nil {
		return deployments, err
	}
	return deployments, nil
}

//func consolidateDeployments(deployments []appsv1.Deployment, currentDeployments *appsv1.DeploymentList, deploymentsClient v1.DeploymentInterface) {
func consolidateDeployments(deployments appsv1.DeploymentList, currentDeployments *appsv1.DeploymentList, deploymentsClient v1.DeploymentInterface) {
	for _, d := range deployments.Items {
		create := true
		for _, e := range currentDeployments.Items {
			if (d.ObjectMeta.Name == e.ObjectMeta.Name) {
				create = false
			}
		}
		if create {
			err := createDeployment(d, deploymentsClient)
			if err != nil {
				log.Println("[ork3strate] Warning:", err.Error())
			}
		} else {
			err := updateDeployment(d, deploymentsClient)
			if err != nil {
				log.Println("[ork3strate] Warning:", err.Error())
			}
		}
	}
	for _, d := range currentDeployments.Items {
		delete := true
		for _, e := range deployments.Items {
			if (d.ObjectMeta.Name == e.ObjectMeta.Name) {
				delete = false
			}
		}
		if delete {
			err := deleteDeployment(d.ObjectMeta.Name, deploymentsClient)
			if err != nil {
				log.Println("[ork3strate] Warning:", err.Error())
			}
		}
	}
}

func deleteDeployment(deploymentName string, deploymentsClient v1.DeploymentInterface) error {
	// Delete Deployment
	log.Println("[ork3strate] Deleting deployment...")
	deletePolicy := metav1.DeletePropagationForeground
	if err := deploymentsClient.Delete(deploymentName, &metav1.DeleteOptions{PropagationPolicy: &deletePolicy}); err != nil {
		return err
	}
	log.Println("[ork3strate] Deleted deployment.")
	return nil
}

func createDeployment(deployment appsv1.Deployment, deploymentsClient v1.DeploymentInterface) error {
	// Create a new deployment based on decoded manifest
	log.Println("[ork3strate] Create deployment...")
	if _, err := deploymentsClient.Create(&deployment); err != nil {
		return err
	}
	log.Println("[ork3strate] Created deployment...")
	return nil
}

func updateDeployment(deployment appsv1.Deployment, deploymentsClient v1.DeploymentInterface) error {
	// Update deployment based on decoded manifest
	log.Println("[ork3strate] Updating deployment...")
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, updateErr := deploymentsClient.Update(&deployment)
		return updateErr
	})
	if retryErr != nil {
		return retryErr
	}
	log.Println("[ork3strate] Updated deployment...")
	return nil
}

func OnConfigReceived(_ mqtt.Client, msg mqtt.Message) {
	log.Printf("[ork3strate] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
	//separator := []byte("---")
	//manifests := bytes.Split(msg.Payload(), separator)
	//deployments, err := decodeDeploymentManifests(manifests)
	deployments, err := decodeDeploymentManifests(msg.Payload())
	if err != nil {
		log.Println("[ork3strate] Warning:", err.Error())
	} else {
		log.Println("[ork3strate] Successfully decoded deployments")

		clientset, err := getClient(flag.Lookup("kube_config").Value.(flag.Getter).Get().(string))
		if err != nil {
			log.Println("[ork3strate] Warning:", err.Error())
		} else {
			deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)
			log.Println("[ork3strate] Initialised k3s client")

			// List existing deployments in namespace
			currentDeployments, err := deploymentsClient.List(metav1.ListOptions{})
			if err != nil {
				log.Println("[ork3strate] Warning:", err.Error())
			} else {
				for i, e := range currentDeployments.Items {
					log.Printf("[ork3strate] Deployment #%d: %s\n", i, e.ObjectMeta.Name)
				}
			}
			consolidateDeployments(deployments, currentDeployments, deploymentsClient)
		}
	}
}

func getClient(pathToCfg string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error
	if pathToCfg == "" {
		log.Println("[ork3strate] Using in cluster config")
		config, err = rest.InClusterConfig()
		// in cluster access
	} else {
		log.Println("[ork3strate] Using out of cluster config")
		config, err = clientcmd.BuildConfigFromFlags("", pathToCfg)
	}
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}
