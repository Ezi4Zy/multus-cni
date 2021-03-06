package constants

import (
	"errors"
	"github.com/containernetworking/cni/pkg/types"
)

const (
	DefaultSocketPath               = "/var/run/hostnic/hostnic.socket"
	DefaultUnixSocketPath           = "unix://" + DefaultSocketPath
	DefaultConfigPath               = "/etc/hostnic"
	DefaultConfigName               = "hostnic"
	DefaultMultusNicDevicesLocation = "/var/lib/multus-nic-device"

	DefaultJobSyn   = 10
	DefaultNodeSync = 1 * 60
	DefaultVipSync  = 5

	DefaultLowPoolSize  = 3
	DefaultHighPoolSize = 5

	NicNumLimit           = 63
	VxnetNicNumLimit      = 252
	DefaultRouteTableBase = 260

	NicPrefix    = "multus_hostnic_"
	PodRefPrefix = "pod_related_"
	VIPConfName  = "VIPAllocator.json"
	MaxRetry     = 3
	VIPNumLimit  = 255
	RetrySteps   = 15

	HostNicPassThrough = "passthrough"
	HostNicVeth        = "veth"

	HostNicPrefix = "vnic"

	DefaultNatMark        = "0x10000"
	DefaultPrimaryNic     = "eth0"
	MainTable             = 254
	ManglePreroutingChain = "HOSTNIC-PREROUTING"
	MangleOutputChain     = "HOSTNIC-OUTPUT"
	MangleForward         = "HOSTNIC-FORWARD"

	ToContainerRulePriority   = 1535
	FromContainerRulePriority = 1536
)

type ResourceType string

const (
	ResourceTypeInstance ResourceType = "instance"
	ResourceTypeVxnet    ResourceType = "vxnet"
	ResourceTypeNic      ResourceType = "nic"
)

type NetConf struct {
	CNIVersion   string          `json:"cniVersion,omitempty"`
	Name         string          `json:"name,omitempty"`
	Type         string          `json:"type,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	IPAM         struct {
		Name string `json:"name,omitempty"`
		Type string `json:"type,omitempty"`
	} `json:"server,omitempty"`
	HostVethPrefix string `json:"vethPrefix,omitempty"`
	HostNicType    string `json:"hostNicType,omitempty"`
	MTU            int    `json:"mtu,omitempty"`
	Service        string `json:"serviceCIDR,omitempty"`
	// Route table to pod
	RT2Pod    int    `json:"rt2Pod,omitempty"`
	Interface string `json:"interface,omitempty"`
	Hairpin   bool   `json:"hairpin,omitempty"`
	// 0x8000 for kube-proxy filter
	// 0x4000 for kube-proxy nat
	// 0xff000000 for calico
	NatMark  string `json:"natMark,omitempty"`
	LogLevel int    `json:"logLevel,omitempty"`
	LogFile  string `json:"logFile,omitempty"`
}

// K8sArgs is the valid CNI_ARGS used for Kubernetes
type K8sArgs struct {
	types.CommonArgs
	// K8S_POD_NAME is pod's name
	K8S_POD_NAME types.UnmarshallableString
	// K8S_POD_NAMESPACE is pod's namespace
	K8S_POD_NAMESPACE types.UnmarshallableString
	// K8S_POD_INFRA_CONTAINER_ID is pod's container id
	K8S_POD_INFRA_CONTAINER_ID types.UnmarshallableString
}

var (
	ErrNoAvailableNIC = errors.New("no free nic")
	ErrNicNotFound    = errors.New("hostnic not found")
)

const (
	// NodeNameEnvKey is env to get the name of current node
	NodeNameEnvKey   = "MY_NODE_NAME"
	AnnoHostNicVxnet = "multus.network.dataworkbench.io/vxnet"
	AnnoHostNic      = "multus.network.dataworkbench.io/nic"
	AnnoHostNicIP    = "multus.network.dataworkbench.io/ip"
	AnnoHostNicType  = "multus.network.dataworkbench.io/type"
)
