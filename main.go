package main

import (
	"os"
	"os/signal"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type InformerInfo struct {
	shortName   string
	stopChannel chan struct{}
	informer    cache.SharedIndexInformer
}

func main() {
	cfg, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		logrus.WithError(err).Fatal("could not get config")
	}

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		logrus.WithError(err).Fatal("could not generate dynamic client for config")
	}

	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, 0, "pn", nil)
	// informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dc, 0, v1.NamespaceAll, nil)

	handler := createHandler()

	informerInfos := []InformerInfo{}
	informerInfos = append(informerInfos, startWatching(informerFactory, "secrets", schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, handler))
	informerInfos = append(informerInfos, startWatching(informerFactory, "cms", schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, handler))

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt)

	ticker := time.NewTicker(1 * time.Second)
	stopTicker := make(chan bool)
	go func() {
		for {
			select {
			case <-stopTicker:
				return
			case <-ticker.C:
				fields := logrus.Fields{}
				for _, informerInfo := range informerInfos {
					list := informerInfo.informer.GetStore().List()
					fields[informerInfo.shortName] = len(list)
				}
				logrus.WithFields(fields).Info("Checking caches")
			}
		}
	}()

	<-signalChannel
	for _, informerInfo := range informerInfos {
		close(informerInfo.stopChannel)
	}
	stopTicker <- true
}

func createHandler() (handler cache.ResourceEventHandlerFuncs) {
	handler = cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			logrus.WithFields(logrus.Fields{
				"kind":      u.GetKind(),
				"name":      u.GetName(),
				"namespace": u.GetNamespace(),
			}).Info("received add event!")
		},
		UpdateFunc: func(oldObj, obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			logrus.WithFields(logrus.Fields{
				"kind":      u.GetKind(),
				"name":      u.GetName(),
				"namespace": u.GetNamespace(),
			}).Info("received update event!")
		},
		DeleteFunc: func(obj interface{}) {
			u := obj.(*unstructured.Unstructured)
			logrus.WithFields(logrus.Fields{
				"kind":      u.GetKind(),
				"name":      u.GetName(),
				"namespace": u.GetNamespace(),
			}).Info("received delete event!")
		},
	}
	return
}

func startWatching(informerFactory dynamicinformer.DynamicSharedInformerFactory, shortName string, gvr schema.GroupVersionResource, handler cache.ResourceEventHandlerFuncs) (informerInfo InformerInfo) {
	informerInfo.shortName = shortName
	informerInfo.informer = informerFactory.ForResource(gvr).Informer()
	informerInfo.stopChannel = make(chan struct{})
	informerInfo.informer.AddEventHandler(handler)
	go informerInfo.informer.Run(informerInfo.stopChannel)
	return
}
