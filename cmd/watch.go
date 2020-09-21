package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/Stoakes/k8s-watcher-rds-server/cmd/internal/ads_server"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	versionedclient "istio.io/client-go/pkg/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

var watchCmd = &cobra.Command{
	Use:     "watch",
	Short:   "Watch services on Kubernetes API",
	Long:    ``,
	Example: ``,
	RunE: func(c *cobra.Command, args []string) error {
		config, err := clientcmd.BuildConfigFromFlags("", viper.GetString("kubeconfig"))
		if err != nil {
			glog.Errorln(err)
		}

		adsServer := ads_server.NewADSServer()

		// watcher goroutine
		go func() {

			ic, err := versionedclient.NewForConfig(config)
			if err != nil {
				log.Fatalf("Failed to create istio client: %s", err)
			}

			_, controller := cache.NewInformer( // also take a look at NewSharedIndexInformer
				&cache.ListWatch{
					ListFunc: func(lo metav1.ListOptions) (result runtime.Object, err error) {
						return ic.NetworkingV1alpha3().Gateways("").List(context.TODO(), metav1.ListOptions{})
					},
					WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
						return ic.NetworkingV1alpha3().Gateways("").Watch(context.TODO(), metav1.ListOptions{})
					},
				},
				&v1alpha3.Gateway{},
				0, //Duration is int64
				cache.ResourceEventHandlerFuncs{
					AddFunc: func(obj interface{}) {
						fmt.Printf("Gateway added: %s \n", obj)
						adsServer.NotifyChangeRoute(ads_server.ServiceEvent{Service: obj.(*v1alpha3.Gateway), Event: ads_server.AddEvent})
					},
					DeleteFunc: func(obj interface{}) {
						fmt.Printf("Gateway deleted: %s \n", obj)
						adsServer.NotifyChangeRoute(ads_server.ServiceEvent{Service: obj.(*v1alpha3.Gateway), Event: ads_server.DeleteEvent})
					},
					UpdateFunc: func(oldObj, newObj interface{}) {
						fmt.Printf("Gateway changed \n")
						adsServer.NotifyChangeRoute(ads_server.ServiceEvent{Service: newObj.(*v1alpha3.Gateway), Event: ads_server.UpdateEvent})
					},
				},
			)
			stop := make(chan struct{})
			defer close(stop)
			controller.Run(stop)
		}()

		// rds server
		listener, err := net.Listen("tcp", ":9876")
		if err != nil {
			log.Fatal(err)
		}
		adsServer.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			log.Fatal("Error opening grpc server:", err.Error())
		}

		return nil
	},
}
