package ads_server

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	rds "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
)

const (
	// TypePrefix is the grpc type prefix
	TypePrefix = "type.googleapis.com/envoy.api.v2."

	/* XDS Constants */

	// RouteType is used for routes discovery.
	RouteType = TypePrefix + "RouteConfiguration"

	// HostnameAnnotation annotation for hostname exposition. Must contain hostname on gateway
	HostnameAnnotation = "stoakes.github.com/hostname"
	// ClusterAnnotation annotation for cluster. Must contain cluster to which hostname will be routed to
	ClusterAnnotation = "stoakes.github.com/cluster"
	// PrefixAnnotation annotation for route prefix. Optionnal, will default to / if not found on a Gateway object
	PrefixAnnotation = "stoakes.github.com/prefix"
)

// AdsServer is a small gRPC server (initially created for ADS mock for pilotctl, repurposed here as RDS server)
type AdsServer struct {
	grpcServer        *grpc.Server
	streams           map[uint64]*streamStore // list of opened connections to the server
	connectionCounter *atomic.Uint64
	versionCounter    *atomic.Uint64
	gateways          map[string]*v1alpha3.Gateway // list of gateways in the cluster
}

// streamStore represents an open connection and stores its control channel
type streamStore struct {
	stream  xdsapi.RouteDiscoveryService_StreamRoutesServer
	id      uint64
	control chan *xdsapi.DiscoveryResponse // every response posted on this channel will be sent on the stream
	quit    chan bool                      // a message on this channel will terminate the stream
}

// NewADSServer creates a ADS server.
// It is caller duty to start the server using *AdsServer.Serve
func NewADSServer() *AdsServer {
	adsServer := &AdsServer{
		grpcServer:        grpc.NewServer(),
		streams:           make(map[uint64]*streamStore),
		connectionCounter: atomic.NewUint64(0),
		versionCounter:    atomic.NewUint64(0),
		gateways:          make(map[string]*v1alpha3.Gateway),
	}
	xdsapi.RegisterRouteDiscoveryServiceServer(adsServer.grpcServer, adsServer)

	return adsServer
}

// StreamRoutes implements RDS Server interface
func (a *AdsServer) StreamRoutes(stream xdsapi.RouteDiscoveryService_StreamRoutesServer) error {
	log.Println("Got new RDS Connection from ", stream)
	ss := &streamStore{
		stream:  stream,
		id:      a.connectionCounter.Load(),
		control: make(chan *xdsapi.DiscoveryResponse),
		quit:    make(chan bool),
	}
	a.connectionCounter.Inc()
	a.streams[ss.id] = ss

	ctx := stream.Context()
	version := strconv.FormatUint(a.versionCounter.Load(), 10)
	stream.Send(a.gatewaysToRoutes(version))
	log.Println("sent initial config")

	for {
		select {
		case <-ctx.Done():
			delete(a.streams, ss.id)
			return nil
		case <-ss.quit:
			return nil
		case xds := <-ss.control:
			stream.Send(xds)
		}
	}
}

func isValidSubscriber(req *xdsapi.DiscoveryRequest) bool {
	return (len(req.Node.Cluster) > 0) && (len(req.Node.Id) > 0)
}

// DeltaRoutes implements RDS Server interface
func (a *AdsServer) DeltaRoutes(stream xdsapi.RouteDiscoveryService_DeltaRoutesServer) error {
	return fmt.Errorf("Unimplemented")
}

// FetchRoutes implements RDS server interface
func (a *AdsServer) FetchRoutes(context.Context, *xdsapi.DiscoveryRequest) (*xdsapi.DiscoveryResponse, error) {
	return &xdsapi.DiscoveryResponse{}, fmt.Errorf("Unimplemented")
}

// Shutdown shuts down the server mock
func (a *AdsServer) Shutdown() {
	time.Sleep(100 * time.Millisecond)
	for _, stream := range a.streams {
		stream.quit <- true
	}
	a.grpcServer.Stop()
	time.Sleep(50 * time.Millisecond)
}

// Serve starts the grpc xds server
func (a *AdsServer) Serve(listener net.Listener) {
	a.grpcServer.Serve(listener)
}

// NotifyChangeRoute sends a message of change to every connection
// For nwo, this method lacks concurrency protection
func (a *AdsServer) NotifyChangeRoute(event ServiceEvent) {
	log.Printf("Got event %v", event)
	if event.Event == AddEvent {
		a.gateways[event.Service.Name+"-"+event.Service.Namespace] = event.Service
	}
	if event.Event == DeleteEvent {
		delete(a.gateways, event.Service.Name+"-"+event.Service.Namespace)
	}
	if event.Event == UpdateEvent {
		log.Println("Just a prototype, update will be supported later on")
	}
	version := strconv.FormatUint(a.versionCounter.Load(), 10)
	for id, stream := range a.streams {
		log.Printf("Notifying socket %d", id)
		stream.control <- a.gatewaysToRoutes(version)
	}
	a.versionCounter.Inc()
}

func (a *AdsServer) gatewaysToRoutes(version string) *xdsapi.DiscoveryResponse {
	out := &xdsapi.DiscoveryResponse{
		TypeUrl:     RouteType,
		VersionInfo: "0",
		Nonce:       time.Now().String() + "version",
	}

	/*
		A route in Envoy Config is:
			name: local_route
			virtual_hosts:
			- name: local_service
				domains: ["*"]
				routes:
				- match: { prefix: "/" }
				route: { cluster: some_service }
	*/
	for _, gateway := range a.gateways {
		hostname := ""
		cluster := ""
		prefix := "/"
		for k, annot := range gateway.Annotations {
			if k == HostnameAnnotation {
				hostname = annot
				continue
			}
			if k == ClusterAnnotation {
				cluster = annot
				continue
			}
			if k == PrefixAnnotation {
				prefix = annot
				continue
			}
		}
		if len(hostname) == 0 || len(cluster) == 0 { // No annotation => no need to propagate it as RDS
			continue
		}
		route := xdsapi.RouteConfiguration{
			Name: "rds_config_name", // must match Envoy config: route_config_name
			VirtualHosts: []*rds.VirtualHost{
				&rds.VirtualHost{
					Name:    gateway.Name + "-" + gateway.Namespace,
					Domains: strings.Split(hostname, ","),
					Routes: []*rds.Route{
						&rds.Route{
							Match: &rds.RouteMatch{
								PathSpecifier: &rds.RouteMatch_Prefix{
									Prefix: prefix,
								},
							},
							Action: &rds.Route_Route{
								Route: &rds.RouteAction{
									ClusterSpecifier: &rds.RouteAction_Cluster{
										Cluster: cluster,
									},
								},
							},
						},
					},
				},
			},
		}
		resource, _ := messageToAny(&route)
		out.Resources = append(out.Resources, resource)
	}
	fmt.Println(out)

	return out
}

func messageToAny(msg proto.Message) (*any.Any, error) {
	b := proto.NewBuffer(nil)
	b.SetDeterministic(true)
	err := b.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return &any.Any{
		TypeUrl: "type.googleapis.com/" + proto.MessageName(msg),
		Value:   b.Bytes(),
	}, nil
}
