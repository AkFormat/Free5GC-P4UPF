package forwarder

const Station_TableName string = "PreQosPipe.my_station"

const SourceInterfaceLookup_TableName string = "PreQosPipe.source_iface_lookup"
const SourceInterfaceLookup_ActionName string = "PreQosPipe.set_source_iface"

const Route_TableName string = "PreQosPipe.Routing.routes_v4"
const Route_ActionName string = "PreQosPipe.Routing.route"
const Route_ActionProfileName string = "hashed_selector"
const Route_ActionProfileGroupID int = 0

const PDR_TableName string = "PreQosPipe.pdrs"
const PDR_ActionName string = "PreQosPipe.set_pdr_attributes"
const PDR_ActionQoSName string = "PreQosPipe.set_pdr_attributes_qos"

const FAR_TableName string = "PreQosPipe.load_far_attributes"
const FAR_ActionNromalName string = "PreQosPipe.load_normal_far_attributes"
const FAR_ActionTunnelName string = "PreQosPipe.load_tunnel_far_attributes"

//const Acl_TableName string = "Acl.acls"

type InitParameters struct {
	N3IPAddr string
	N3Mac    string
	//N3Mask []byte
	N3SwitchPort uint32
	N6IPAddr     string
	N6Mac        string
	//N6Mask []byte
	N6SwitchPort uint32
	gNBAddr      string
	gNBMac       string
	//gNBMask []byte
	UEIPNet   string
	UEIPMask  uint32
	DNNIPAddr string
	DNNMac    string
	gRPC      string // 用于控制交换机
}

type SDFFilterAttr struct {
	FD  *FlowDesc
	TTC *uint16
	SPI *uint32
	FL  *uint32
	BID *uint32
}

type PDIAttr struct {
	TEID            uint32
	GTPuAddr        string
	GNBIpv4         string
	UEIPv4          string
	SourceInterface uint32
	NetworkInstance string
	SDFFilter       *SDFFilterAttr
	ApplicationID   uint32
}

type ForwardingParameter struct {
	DestinationInterface           uint8  // 应该不会用到
	NetworkInstance                string // 应该不会用
	OuterHeaderCreationDescription uint32 // 目前不清楚这个描述是描述哪种协议的
	TEID                           uint32
	PortNumber                     uint32
	IPv4Address                    string
}
