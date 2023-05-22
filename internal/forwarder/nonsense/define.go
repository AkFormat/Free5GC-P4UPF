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
	N3IPAddr []byte
	N3Mac    []byte
	//N3Mask []byte
	N3SwitchPort []byte
	N6IPAddr     []byte
	N6Mac        []byte
	//N6Mask []byte
	N6SwitchPort []byte
	gNBAddr      []byte
	gNBMac       []byte
	//gNBMask []byte
	UEIPNet   []byte
	UEIPMask  []byte
	DNNIPAddr []byte
	DNNMac    []byte
	gRPC      string // 用于控制交换机
}

// MatchKeys 用于 P4 表项的四种匹配方式：精确，最长，三态和范围
type MatchKeys struct {
	ExactValue   []byte
	LpmValue     []byte
	LpmPrefixLen int32
	TernaryValue []byte
	TernaryMask  []byte
	RangeLow     []byte
	RangeHigh    []byte
	DontCare     bool
}

// free5gc PDI 结构体

type SDFFilterAttr struct {
	FD  *FlowDesc
	TTC *uint16
	SPI *uint32
	FL  *uint32
	BID *uint32
}

type PDIAttr struct {
	TEID            []byte
	GTPuAddr        []byte
	GNBIpv4         []byte
	UEIPv4          []byte
	SourceInterface []byte
	NetworkInstance string
	SDFFilter       *SDFFilterAttr
	ApplicationID   []byte
}

// PDREntryAttr PDR 表项对应的字段
type PDREntryAttr struct {
	SourceInterface []byte   // bit<8>, PDI
	TunnelIpv4Dst   []byte   // bit <32>, PDI
	TEID            []byte   // bit <32> TUNNAL ID, PDI
	UEAddr          []byte   // bit <32> , SDF
	UEL4Port        [][]byte // bit <16>, SDF
	INETAddr        []byte   // bit <32> , SDF
	INETL4Port      [][]byte // bit <16>, SDF
	IPProto         []byte   // bit<8>, SDF
	IfPSC           []byte   // bit<1> bool, QFI
	PSCQFI          []byte   // bit<6>, from PSC IE, QFI
	PDRID           []byte   // bit<32>, ACTION
	FSEID           []byte   // bit<96>  8-byte SEID + UPF IP(v4/v6) address, ACTION, session_id
	CounterIndex    []byte   // bit<32>, ACTION，必填
	FARID           []byte   // bit<32>, ACTION
	IfGtpuDecap     []byte   // bit<1> bool, ACTION
	IfQFIPush       []byte   // bit<1> bool, ACTION
	ActionQFI       []byte   // bit <6>, used for set QFI, only for downlink, ACTION
	Priority        uint32
}

type ForwardingParameter struct {
	DestinationInterface           uint8  // 应该不会用到
	NetworkInstance                []byte // 应该不会用
	OuterHeaderCreationDescription []byte // 目前不清楚这个描述是描述哪种协议的
	TEID                           []byte
	PortNumber                     []byte
	IPv4Address                    []byte
}

// FAREntryAttr FAR 表项定义的字段
type FAREntryAttr struct {
	FARID               []byte // bit<32>
	FSEID               []byte // bit<96>, session id
	IfDrop              []byte // bit<1> needs_dropping
	Forward             bool   // table中不存在，但是便于后续开发
	NotifyCP            []byte // bit<1> notify_cp
	IfBuffering         []byte //bit<1> needs_buffering
	TunnelType          []byte //bit<8> GTP-U    = 0x3
	forwardingParameter *ForwardingParameter
}

// SrcIfTypeAttr
/*
enum bit<8> Direction {
    UNKNOWN             = 0x0,
    UPLINK              = 0x1,
    DOWNLINK            = 0x2,
    OTHER               = 0x3
};

enum bit<8> InterfaceType { // 修改为和free5gc一致的变量
    UNKNOWN       = 0x9, // 胡乱写的一个数据，应该用不到
    ACCESS        = 0x0,
    CORE          = 0x1,
    N6_LAN        = 0x2, // unused, 可能会用到
    VN_INTERNAL   = 0x4, // unused
    CONTROL_PLANE = 0x3 // N4 and N4-u
}
*/
type SrcIfTypeAttr struct {
	SrcIPAddr []byte
	SrcIPMask int32
	SrcIfType []byte
	Direction []byte
}

type StationAttr struct {
	Mac []byte
}

//
/*
 action route(mac_addr_t src_mac,
                 mac_addr_t dst_mac,
                 port_num_t egress_port) {
        std_meta.egress_spec = egress_port;
        hdr.ethernet.src_addr = src_mac;
        hdr.ethernet.dst_addr = dst_mac;
    }


    table routes_v4 {
        key = {
            local_meta.next_hop_ip   : lpm @name("dst_prefix");
            hdr.ipv4.src_addr      : selector;
            hdr.ipv4.proto         : selector;
            local_meta.l4_sport    : selector;
            local_meta.l4_dport    : selector;
        }
        actions = {
            route;
        }
*/

type RouteAttr struct {
	NextHopIPAddr        []byte
	NetHopIPMask         int32
	ActionProfileGroupID int
}
