package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceName                    string = "multus.network.dataworkbench.io/multus-nic-device"
	defaultMultusNicDevicesLocation string = "/var/lib/multus-nic-device"
	multusNicSocket                 string = "multus-nic.sock"
	// KubeletSocket kubelet 监听 unix 的名称
	KubeletSocket string = "kubelet.sock"
	// DevicePluginPath 默认位置
	DevicePluginPath  string = "/var/lib/kubelet/device-plugins/"
	TotalDevicesCount int    = 40
)

// MultusNicServer 是一个 device plugin server
type MultusNicServer struct {
	srv         *grpc.Server
	devices     []*pluginapi.Device
	nextDevice  int
	notify      chan bool
	ctx         context.Context
	cancel      context.CancelFunc
	restartFlag bool // 本次是否是重启
}

// NewMultusNicServer 实例化 MultusNicServer
func NewMultusNicServer() *MultusNicServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &MultusNicServer{
		devices:     make([]*pluginapi.Device, TotalDevicesCount),
		nextDevice:  0,
		srv:         grpc.NewServer(grpc.EmptyServerOption{}),
		notify:      make(chan bool),
		ctx:         ctx,
		cancel:      cancel,
		restartFlag: false,
	}
}

// Run 运行服务
func (s *MultusNicServer) Run() error {
	// 发现本地设备
	err := s.listDevice()
	if err != nil {
		log.Fatalf("list device error: %v", err)
	}

	go func() {
		err := s.watchDevice()
		if err != nil {
			log.Println("watch devices error")
		}
	}()

	pluginapi.RegisterDevicePluginServer(s.srv, s)
	err = syscall.Unlink(DevicePluginPath + multusNicSocket)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	l, err := net.Listen("unix", DevicePluginPath+multusNicSocket)
	if err != nil {
		return err
	}

	go func() {
		lastCrashTime := time.Now()
		restartCount := 0
		for {
			log.Printf("start GPPC server for '%s'", resourceName)
			err = s.srv.Serve(l)
			if err == nil {
				break
			}

			log.Printf("GRPC server for '%s' crashed with error: $v", resourceName, err)

			if restartCount > 5 {
				log.Fatal("GRPC server for '%s' has repeatedly crashed recently. Quitting", resourceName)
			}
			timeSinceLastCrash := time.Since(lastCrashTime).Seconds()
			lastCrashTime = time.Now()
			if timeSinceLastCrash > 3600 {
				restartCount = 1
			} else {
				restartCount++
			}
		}
	}()

	// Wait for server to start by lauching a blocking connection
	conn, err := s.dial(multusNicSocket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

// RegisterToKubelet 向kubelet注册device plugin
func (s *MultusNicServer) RegisterToKubelet() error {
	socketFile := filepath.Join(DevicePluginPath + KubeletSocket)

	conn, err := s.dial(socketFile, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	req := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(DevicePluginPath + multusNicSocket),
		ResourceName: resourceName,
	}
	log.Infof("Register to kubelet with endpoint %s", req.Endpoint)
	_, err = client.Register(context.Background(), req)
	if err != nil {
		return err
	}

	return nil
}

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (s *MultusNicServer) GetDevicePluginOptions(ctx context.Context, e *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	log.Infoln("GetDevicePluginOptions called")
	return &pluginapi.DevicePluginOptions{PreStartRequired: true}, nil
}

// GetPreferredAllocation
func (s *MultusNicServer) GetPreferredAllocation(ctx context.Context, req *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	log.Infoln("GetPreferredAllocation called")
	return &pluginapi.PreferredAllocationResponse{}, nil
}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disappears, ListAndWatch
// returns the new list
func (s *MultusNicServer) ListAndWatch(e *pluginapi.Empty, srv pluginapi.DevicePlugin_ListAndWatchServer) error {
	log.Infoln("ListAndWatch called")
	err := srv.Send(&pluginapi.ListAndWatchResponse{Devices: s.devices})
	if err != nil {
		log.Errorf("ListAndWatch send device error: %v", err)
		return err
	}

	// 更新 device list
	for {
		log.Infoln("waiting for device change")
		select {
		case <-s.notify:
			log.Infoln("device updated")
			srv.Send(&pluginapi.ListAndWatchResponse{Devices: s.devices})
		case <-s.ctx.Done():
			log.Info("ListAndWatch exit")
			return nil
		}
	}
}

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (s *MultusNicServer) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	log.Infoln("Allocate called")
	resps := &pluginapi.AllocateResponse{}
	for _, req := range reqs.ContainerRequests {
		log.Infof("received request: %v", strings.Join(req.DevicesIDs, ","))
		resp := pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				"MULTUS_NICS": strings.Join(req.DevicesIDs, ","),
			},
		}
		resps.ContainerResponses = append(resps.ContainerResponses, &resp)
	}
	return resps, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registeration phase,
// before each container start. Device plugin can run device specific operations
// such as reseting the device before making devices available to the container
func (s *MultusNicServer) PreStartContainer(ctx context.Context, req *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	log.Infoln("PreStartContainer called")
	return &pluginapi.PreStartContainerResponse{}, nil
}

// listDevice
func (s *MultusNicServer) listDevice() error {
	dir, err := ioutil.ReadDir(defaultMultusNicDevicesLocation)
	if err != nil {
		return err
	}
	s.nextDevice = len(dir)
	log.Infof("Current available devices count: %d, used: %d, total: %d", TotalDevicesCount-s.nextDevice, s.nextDevice, TotalDevicesCount)
	for i := 0; i < TotalDevicesCount; i++ {
		deviceName := strconv.Itoa(i)
		device := &pluginapi.Device{
			ID:     deviceName,
			Health: pluginapi.Healthy,
		}
		if i < s.nextDevice {
			device.Health = pluginapi.Unhealthy
		}
		s.devices[i] = device
	}
	return nil
}

func (s *MultusNicServer) watchDevice() error {
	log.Infoln("watching devices")
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("NewWatcher error:%v", err)
	}
	defer w.Close()

	done := make(chan bool)
	go func() {
		defer func() {
			done <- true
			log.Info("watch device exit")
		}()
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					continue
				}
				log.Infof("device event: %s, name: %s", event.Op.String(), event.Name)
				if event.Op&fsnotify.Create == fsnotify.Create {
					if s.nextDevice < TotalDevicesCount {
						s.devices[s.nextDevice].Health = pluginapi.Unhealthy
						s.nextDevice++
					}
					s.notify <- true
				} else if event.Op&fsnotify.Remove == fsnotify.Remove {
					if s.nextDevice > 0 {
						s.devices[s.nextDevice-1].Health = pluginapi.Healthy
						s.nextDevice--
					}
					s.notify <- true
				}
				log.Infof("Current available devices count: %d, used: %d, total: %d", TotalDevicesCount-s.nextDevice, s.nextDevice, TotalDevicesCount)

			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)

			case <-s.ctx.Done():
				break
			}
		}
	}()

	err = w.Add(defaultMultusNicDevicesLocation)
	if err != nil {
		return fmt.Errorf("watch device error:%v", err)
	}
	<-done

	return nil
}

func (s *MultusNicServer) dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}
