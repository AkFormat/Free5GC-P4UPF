package forwarder

import (
	"context"
	"strings"
	"sync"

	"github.com/free5gc/go-upf/pkg/factory"

	"github.com/free5gc/go-upf/internal/forwarder/buff"
	"github.com/free5gc/go-upf/internal/logger"
	"github.com/free5gc/go-upf/internal/report"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"google.golang.org/grpc"
)

type P4Gtp5g struct {
	conn           *grpc.ClientConn
	client         GrpcServiceClient
	bs             *buff.Server
	log            *logrus.Entry
	initParameters *InitParameters
}

func OpenP4Gtp5g(wg *sync.WaitGroup, parameters *InitParameters) (*P4Gtp5g, error) {
	logger.MainLog.Infof("***** OpenP4Gtp5g start *******************\n ")
	driver := &P4Gtp5g{}
	// 1. 生成log对象
	driver.log = logger.FwderLog.WithField(logger.FieldCategory, "P4Gtp5g")
	driver.initParameters = parameters
	// 2. 连接交换机的P4runtime控制面, 并生成client对象
	conn, err := grpc.Dial(parameters.gRPC, grpc.WithInsecure())
	driver.log.Infoln("连接交换机的P4runtime控制面")
	driver.conn = conn
	if err != nil {
		driver.log.Errorf("%v,连接硬件设备的控制面失败", err)
		logger.MainLog.Fatal("%v,连接硬件设备的控制面失败", err)
	}
	driver.client = NewGrpcServiceClient(conn)

	//defer func(conn *grpc.ClientConn) {
	//	err := conn.Close()
	//	if err != nil {
	//		driver.log.Errorf("因为 %v, 关闭与硬件设备的grpc连接", err)
	//		logger.MainLog.Fatal("%v,关闭与硬件设备的grpc连接", err)
	//	}
	//}(conn)

	bs, err := buff.OpenServer(wg, SOCKPATH) // 开大缓存服务器，暂时还不知道有什么用
	if err != nil {
		driver.Close()
		return nil, errors.Wrap(err, "open buff server")
	}
	driver.bs = bs

	//添加 lookup_interface table 的表项
	N3srcLookTableKey := SrcIfLookupTableKey{
		DstAddr:          driver.initParameters.N3IPAddr,
		DstAddrPrefixLen: 32,
	}

	/*
			enum bit<8> InterfaceType {
		    UNKNOWN       = 0x0,
		    ACCESS        = 0x1,
		    CORE          = 0x2,
		    N6_LAN        = 0x3, // unused
		    VN_INTERNAL   = 0x4, // unused
		    CONTROL_PLANE = 0x5 // N4 and N4-u
		}
	*/
	N3srcLookTableData := SrcIfLookupTableData_ActionSetSourceIface{
		SrcIface:  0, // FIXME: 这个应该不是这样 //已修改：ACCESS
		Direction: 1,
	}

	N3srcInfo := SrcIfLookupTableData{
		Action:              SrcIfLookupTableData_set_source_iface,
		SrcIfLookupAction_1: &N3srcLookTableData,
	}
	N3CreateSrcInfo := CreateSrcIfLookTableRequest{
		Key:  &N3srcLookTableKey,
		Data: &N3srcInfo,
	}

	N6srcLookTableKey := SrcIfLookupTableKey{
		DstAddr:          driver.initParameters.N6IPAddr,
		DstAddrPrefixLen: 32,
	}
	N6srcLookTableData := SrcIfLookupTableData_ActionSetSourceIface{
		SrcIface:  2, // CORE
		Direction: 2,
	}
	N6srcInfo := SrcIfLookupTableData{
		Action:              SrcIfLookupTableData_set_source_iface,
		SrcIfLookupAction_1: &N6srcLookTableData,
	}
	N6CreateSrcInfo := CreateSrcIfLookTableRequest{
		Key:  &N6srcLookTableKey,
		Data: &N6srcInfo,
	}

	UeLookTableKey := SrcIfLookupTableKey{
		DstAddr:          driver.initParameters.UEIPNet,
		DstAddrPrefixLen: 24,
	}
	UeLookTableData := SrcIfLookupTableData_ActionSetSourceIface{
		SrcIface:  2, // CORE
		Direction: 2,
	}
	UeInfo := SrcIfLookupTableData{
		Action:              SrcIfLookupTableData_set_source_iface,
		SrcIfLookupAction_1: &UeLookTableData,
	}
	UeCreateSrcInfo := CreateSrcIfLookTableRequest{
		Key:  &UeLookTableKey,
		Data: &UeInfo,
	}

	_, err = driver.client.CreateSrcIfaceLook(context.Background(), &N3CreateSrcInfo)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateSrcIf Error: %v", err)
		}
	}
	_, err = driver.client.CreateSrcIfaceLook(context.Background(), &N6CreateSrcInfo)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateSrcIf Error: %v", err)
		}
	}

	_, err = driver.client.CreateSrcIfaceLook(context.Background(), &UeCreateSrcInfo)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateSrcIf Error: %v", err)
		}
	}
	//..........................................................
	//添加 my_station table 的表项
	N3mac := MyStationTableKey{DstMac: driver.initParameters.N3Mac}
	N6mac := MyStationTableKey{DstMac: driver.initParameters.N6Mac}
	N3MystaData := MyStationTableData{Action: MyStationTableData_no_action}
	N6MystaData := MyStationTableData{Action: MyStationTableData_no_action}
	N3CreateMyStation := CreateMyStationTableRequest{
		Key:  &N3mac,
		Data: &N3MystaData,
	}
	N6CreateMyStation := CreateMyStationTableRequest{
		Key:  &N6mac,
		Data: &N6MystaData,
	}

	_, err = driver.client.CreateMyStation(context.Background(), &N3CreateMyStation)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateMyStation Error: %v", err)
		}
	}
	_, err = driver.client.CreateMyStation(context.Background(), &N6CreateMyStation)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateMyStation Error: %v", err)
		}
	}
	//..........................................................
	//添加route表项
	DNNRouteKey := RouteTableKey{
		NextHopIp:          string(driver.initParameters.N3IPAddr),
		NextHopIpPrefixLen: 32,
	}
	DNNActionRoute := RouteTableData_ActionRoute{
		SrcMac:     string(driver.initParameters.N6Mac),
		DstMac:     string(driver.initParameters.DNNMac),
		EgressPort: 64,
	}
	DNNRouteData := RouteTableData{
		Action:        RouteTableData_route,
		RouteAction_1: &DNNActionRoute,
	}
	DNNCreateRoute := CreateRouteTableRequest{
		Key:  &DNNRouteKey,
		Data: &DNNRouteData,
	}
	GNBRouteKey := RouteTableKey{
		NextHopIp:          string(driver.initParameters.gNBAddr),
		NextHopIpPrefixLen: 32,
	}
	GNBActionRoute := RouteTableData_ActionRoute{
		SrcMac:     string(driver.initParameters.N3Mac),
		DstMac:     string(driver.initParameters.gNBMac),
		EgressPort: 65,
	}
	GNBRouteData := RouteTableData{
		Action:        RouteTableData_route,
		RouteAction_1: &GNBActionRoute,
	}
	GNBCreateRoute := CreateRouteTableRequest{
		Key:  &GNBRouteKey,
		Data: &GNBRouteData,
	}
	driver.log.Infoln("添加 route表项 ")
	driver.log.Infoln(DNNCreateRoute)
	_, err = driver.client.CreateRoute(context.Background(), &DNNCreateRoute)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateRoute Error: %v", err)
		}
	}
	driver.log.Infoln(GNBCreateRoute)
	_, err = driver.client.CreateRoute(context.Background(), &GNBCreateRoute)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateRoute Error: %v", err)
		}
	}
	UeRouteKey := RouteTableKey{
		NextHopIp:          string(driver.initParameters.UEIPNet),
		NextHopIpPrefixLen: 24,
	}
	UeActionRoute := RouteTableData_ActionRoute{
		SrcMac:     string(driver.initParameters.N3Mac),
		DstMac:     string(driver.initParameters.gNBMac),
		EgressPort: 65,
	}
	UeRouteData := RouteTableData{
		Action:        RouteTableData_route,
		RouteAction_1: &UeActionRoute,
	}
	UeCreateRoute := CreateRouteTableRequest{
		Key:  &UeRouteKey,
		Data: &UeRouteData,
	}
	driver.log.Infoln(UeCreateRoute)
	_, err = driver.client.CreateRoute(context.Background(), &UeCreateRoute)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			driver.log.Errorf("CreateRoute Error: %v", err)
		}
	}
	//..........................................................

	return driver, nil
}

//******************************************************
// 下面是进行，动态加表操作
//*****************************************************
func (g *P4Gtp5g) Close() {

	err := g.conn.Close()
	if err != nil {
		g.log.Errorf("因为 %v, 关闭与硬件设备的grpc连接", err)
		logger.MainLog.Fatal("%v,关闭与硬件设备的grpc连接", err)
	}
	if g.client != nil {
		g.client = nil
	}
	if g.conn != nil {
		g.conn.Close()
	}

	if g.bs != nil {
		g.bs.Close()
	}
}

func (g *P4Gtp5g) newFlowDesc(s string) (*FlowDesc, error) {
	fd, err := ParseFlowDesc(s)
	if err != nil {
		return nil, err
	}
	return fd, nil
}

func (g *P4Gtp5g) newSdfFilter(i *ie.IE) (*SDFFilterAttr, error) {
	var attr SDFFilterAttr

	v, err := i.SDFFilter()
	if err != nil {
		return nil, err
	}

	if v.HasFD() {
		fd, err := g.newFlowDesc(v.FlowDescription)
		// TODO: FD需要处理
		if err != nil {
			return nil, err
		}
		attr.FD = fd
	}
	if v.HasTTC() {
		// TODO:
		// v.ToSTrafficClass string
		x := uint16(29)
		attr.TTC = &x
	}
	if v.HasSPI() {
		// TODO:
		// v.SecurityParameterIndex string
		x := uint32(30)
		attr.SPI = &x
	}
	if v.HasFL() {
		// TODO:
		// v.FlowLabel string
		x := uint32(31)
		attr.FL = &x
	}
	if v.HasBID() {
		attr.BID = &(v.SDFFilterID)
	}

	return &attr, nil
}

func (g *P4Gtp5g) newPdi(i *ie.IE) (*PDIAttr, error) {
	var attr PDIAttr
	ies, err := i.PDI() // 提取PDI中的信息
	if err != nil {
		return nil, err
	}
	for _, x := range ies {
		switch x.Type {
		case ie.SourceInterface:
			v, err := x.SourceInterface()
			if err != nil {
				break
			}
			attr.SourceInterface = uint32(v)
		case ie.DestinationInterface:

		case ie.FTEID: // Full TEID = TEID + ipv4
			v, err := x.FTEID()
			if err != nil {
				break
			}
			attr.TEID = v.TEID
			attr.GTPuAddr = v.IPv4Address.String() // 这个是UPF的地址

		case ie.NetworkInstance: // 对于NetworkInstance不做区分
			v, err := x.NetworkInstance()
			if err != nil {
				break
			}
			attr.NetworkInstance = v
		case ie.UEIPAddress: // UE IPv4的地址
			v, err := x.UEIPAddress()
			if err != nil {
				break
			}
			attr.UEIPv4 = v.IPv4Address.String()
		case ie.SDFFilter: // SDF过滤器, 可以扔掉
			v, err := g.newSdfFilter(x)
			if err != nil {
				break
			}
			attr.SDFFilter = v
		case ie.ApplicationID: // 不考虑应用ID，
		}
	}
	return &attr, nil
}

func (g *P4Gtp5g) CreatePDR(lseid uint64, req *ie.IE) error {
	var pdrTable PDRTableKey
	var pdrTabData PDRTableData
	var pdrAttr PDRTableData_ActionSetPDRAttributes       // 用于action1的参数
	var pdrAttrQos PDRTableData_ActionSetPDRAttributesQos // 用于action2的参数

	var pdrId uint16 // 临时存储
	var ifUplink bool

	ies, err := req.CreatePDR()
	if err != nil {
		return err
	}
	for _, i := range ies {
		switch i.Type {
		case ie.PDRID:
			pdrId, err = i.PDRID()
			g.log.Infoln("pdrId:", pdrId)
			if err != nil {
				break
			}
			precedence, err := i.Precedence()
			if err != nil {
				break
			}
			pdrTable.Priority = precedence //PDR规则优先级，uint32类型，对应三态匹配的优先级

		case ie.PDI:
			pdi, err := g.newPdi(i)
			g.log.Infoln("pdi:", pdi)
			if err != nil {
				break
			}

			g.log.Infoln("pdi.SDFFilter:", pdi.SDFFilter)

			// pdr TEID
			// pdr TunnelIpv4Dst, UPF的地址
			pdrTable.SrcIface = pdi.SourceInterface // pdr SourceInterface
			if pdrTable.SrcIface == 0 {             // 代表这个是上行链路
				ifUplink = true
				pdrTabData.Action = PDRTableData_set_pdr_attributes
				pdrAttr.PdrId = uint32(pdrId)
				pdrAttr.Fseid = uint32(lseid)
				g.log.Infoln("table.pdrId:", pdrAttr.PdrId)
			} else if pdrTable.SrcIface == 2 {
				ifUplink = false
				pdrTabData.Action = PDRTableData_set_pdr_attributes_qos
				pdrAttrQos.PdrId = uint32(pdrId)
				pdrAttrQos.Fseid = uint32(lseid)
				pdrAttrQos.NeedsQfiPush = false
				pdrAttrQos.Qfi = 0
				g.log.Infoln("table.pdrId:", pdrAttrQos.PdrId)
			}

			//if pdi.SDFFilter != nil { // 判断SDFFilter是否存在
			//	if pdi.SDFFilter.FD.Action == "out" { // 下行流量
			//		pdrTable.UeAddr = string(pdi.SDFFilter.FD.Dst.IP.To4())
			//		pdrTable.InetAddr = string(pdi.SDFFilter.FD.Src.IP.To4())
			//		pdrTable.InetL4PortLow = uint32(pdi.SDFFilter.FD.SrcPorts[0][0])
			//		pdrTable.InetL4PortHigh = uint32(pdi.SDFFilter.FD.SrcPorts[0][0])
			//		pdrTable.UeL4PortLow = uint32(pdi.SDFFilter.FD.DstPorts[0][0])
			//		pdrTable.UeL4PortHigh = uint32(pdi.SDFFilter.FD.DstPorts[0][0])
			//	}
			//
			//	if pdi.SDFFilter.FD.Action == "in" { // 上行流量
			//		pdrTable.UeAddr = string(pdi.SDFFilter.FD.Dst.IP.To4())
			//		pdrTable.InetAddr = string(pdi.SDFFilter.FD.Src.IP.To4())
			//		pdrTable.InetL4PortLow = uint32(pdi.SDFFilter.FD.SrcPorts[0][0])
			//		pdrTable.InetL4PortHigh = uint32(pdi.SDFFilter.FD.SrcPorts[0][0])
			//		pdrTable.UeL4PortLow = uint32(pdi.SDFFilter.FD.DstPorts[0][0])
			//		pdrTable.UeL4PortHigh = uint32(pdi.SDFFilter.FD.DstPorts[0][0])
			//	}
			//}
			if ifUplink {
				pdrTable.Teid = pdi.TEID
				pdrTable.TeidMask = 4294967295
				pdrTable.TunnelIpv4Dst = pdi.GTPuAddr
				pdrTable.TunnelIpv4DstMask = "255.255.255.255"
				pdrTable.UeAddr = pdi.UEIPv4
				pdrTable.InetAddr = pdi.GTPuAddr
			} else {
				pdrTable.UeAddr = pdi.UEIPv4
				pdrTable.InetAddr = g.initParameters.N3IPAddr
			}

		case ie.OuterHeaderRemoval:
			_, err := i.OuterHeaderRemovalDescription() // pdr ifDeGTP
			if err != nil {
				break
			}
			if ifUplink {
				pdrAttr.NeedsGtpuDecap = true
			} else {
				pdrAttrQos.NeedsGtpuDecap = true
			}
			//if pdrAttr.PdrId == uint32(pdrId) {
			//	pdrAttr.NeedsGtpuDecap = true
			//} else if pdrAttrQos.PdrId == uint32(pdrId) {
			//	pdrAttrQos.NeedsGtpuDecap = true
			//}
			// 只要存在这个字段，就是需要拆除外部字段，所以无需在意字段的值
			// ignore GTPUExternsionHeaderDeletion
		case ie.FARID:
			farid, err := i.FARID() // 对应 pdr FARID
			if err != nil {
				break
			}

			if ifUplink {
				pdrAttr.FarId = farid
			} else {
				pdrAttrQos.FarId = farid
			}
			//if pdrAttr.PdrId == uint32(pdrId) {
			//	pdrAttr.FarId = farid
			//} else if pdrAttrQos.PdrId == uint32(pdrId) {
			//	pdrAttrQos.FarId = farid
			//}
		}
	}

	if pdrTable.UeL4PortLow == 0 {
		pdrTable.UeL4PortLow = 1
		pdrTable.UeL4PortHigh = 65535
	}
	if pdrTable.InetL4PortLow == 0 {
		pdrTable.InetL4PortLow = 1
		pdrTable.InetL4PortHigh = 65535
	}
	g.log.Infoln(pdrAttr) // ?
	g.log.Infoln(pdrAttrQos)
	if ifUplink {
		pdrTabData.PdrAction_1 = &pdrAttr
	} else {
		pdrTabData.PdrAction_2 = &pdrAttrQos
	}

	pdrTable.UeAddrMask = "255.255.255.255"
	pdrTable.InetAddrMask = "255.255.255.255"
	//pdrTable.Ipv4Proto = 0
	//pdrTable.Ipv4ProtoMask = 0
	//pdrTable.HasQfi = false
	//pdrTable.HasQfiMask = 0
	//pdrTable.Qfi = 0
	//pdrTable.QfiMask = 0

	createPdrTable := CreatePDRRequest{
		Key:  &pdrTable,
		Data: &pdrTabData,
	}

	g.log.Infoln(createPdrTable)
	_, err = g.client.CreatePDR(context.Background(), &createPdrTable)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			g.log.Errorf("CreatePdr Error: %v", err)
		}
	}

	return nil
}

func (g *P4Gtp5g) UpdatePDR(lseid uint64, req *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) RemovePDR(lseid uint64, req *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) newForwardingParameter(ies []*ie.IE) (*ForwardingParameter, error) {
	var fd ForwardingParameter
	for _, x := range ies {
		switch x.Type {
		case ie.DestinationInterface:
			v, err := x.DestinationInterface()
			if err != nil {
				break
			}
			fd.DestinationInterface = v
		case ie.NetworkInstance:
		case ie.OuterHeaderCreation:
			v, err := x.OuterHeaderCreation()
			if err != nil {
				break
			}
			fd.OuterHeaderCreationDescription = uint32(v.OuterHeaderCreationDescription)

			//var hc nl.AttrList
			//hc = append(hc, nl.Attr{
			//	Type:  gtp5gnl.OUTER_HEADER_CREATION_DESCRIPTION,
			//	Value: nl.AttrU16(v.OuterHeaderCreationDescription),
			//})
			if x.HasTEID() {
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_O_TEID,
				//	Value: nl.AttrU32(v.TEID),
				//})
				fd.TEID = v.TEID

				// GTPv1-U port
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_PORT,
				//	Value: nl.AttrU16(factory.UpfGtpDefaultPort),
				//})
				fd.PortNumber = factory.UpfGtpDefaultPort

			} else {
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_PORT,
				//	Value: nl.AttrU16(v.PortNumber),
				//})
				fd.PortNumber = uint32(v.PortNumber)

			}
			if x.HasIPv4() {
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_PEER_ADDR_IPV4,
				//	Value: nl.AttrBytes(v.IPv4Address),
				//})
				fd.IPv4Address = v.IPv4Address.String()
			}
			//case ie.ForwardingPolicy: // 不知道干嘛用的
			//	v, err := x.ForwardingPolicyIdentifier()
			//	if err != nil {
			//		break
			//	}
			//	attrs = append(attrs, nl.Attr{
			//		Type:  gtp5gnl.FORWARDING_PARAMETER_FORWARDING_POLICY,
			//		Value: nl.AttrString(v),
			//	})
			//case ie.PFCPSMReqFlags: // 忽略
			//	v, err := x.PFCPSMReqFlags()
			//	if err != nil {
			//		break
			//	}
			//	attrs = append(attrs, nl.Attr{
			//		Type:  gtp5gnl.FORWARDING_PARAMETER_PFCPSM_REQ_FLAGS,
			//		Value: nl.AttrU8(v),
			//	})
		}
	}
	return &fd, nil
}

func (g *P4Gtp5g) CreateFAR(lseid uint64, req *ie.IE) error {
	var farTable FARTableKey
	var farTabData FARTableData
	var farNormal FARTableData_ActionLoadTunnelFarAttributes
	var farTunnel FARTableData_ActionLoadTunnelFarAttributes

	var ifDrop bool
	var ifbuffering bool
	var notifycp bool
	fartunFlag := false

	farTable.Fseid = uint32(lseid)
	ies, err := req.CreateFAR()
	if err != nil {
		return err
	}
	for _, i := range ies {
		switch i.Type {
		case ie.FARID:
			v, err := i.FARID()
			if err != nil {
				return err
			}
			farTable.FarId = v
		case ie.ApplyAction:
			v, err := i.ApplyAction()
			if err != nil {
				return err
			}
			if isBitSet(int(v), 0) {
				ifDrop = true
			} else {
				ifDrop = false
			}
			if isBitSet(int(v), 2) {
				ifbuffering = true
			} else {
				ifbuffering = false
			}
			if isBitSet(int(v), 3) {
				notifycp = true
			} else {
				notifycp = false
			}

		case ie.ForwardingParameters:
			fartunFlag = true
			xs, err := i.ForwardingParameters() // 生成转发参数
			if err != nil {
				return err
			}
			v, err := g.newForwardingParameter(xs)
			if err != nil {
				break
			}
			farTunnel.NeedsDropping = ifDrop
			farTunnel.NotifyCp = notifycp
			farTunnel.NeedsBuffering = ifbuffering
			farTunnel.TunnelType = v.OuterHeaderCreationDescription
			farTunnel.SrcAddr = string(g.initParameters.N3IPAddr)
			farTunnel.DstAddr = string(g.initParameters.gNBAddr)
			farTunnel.Teid = v.TEID
			farTunnel.L4Port = 2152

		}
	}
	g.log.Infoln(fartunFlag)
	if fartunFlag == false {
		farNormal.NeedsDropping = ifDrop
		farNormal.NotifyCp = notifycp
		farNormal.NeedsBuffering = ifbuffering
		farNormal.TunnelType = 3
		farNormal.SrcAddr = string(g.initParameters.N3IPAddr)
		farNormal.DstAddr = string(g.initParameters.gNBAddr)
		farNormal.Teid = 1
		farNormal.L4Port = 2152
	}

	if fartunFlag == true {
		farTabData.Action = FARTableData_load_normal_far_attributes
		farTabData.FarAction_2 = &farTunnel
	} else {
		farTabData.Action = FARTableData_load_tunnel_far_attributes
		farTabData.FarAction_2 = &farNormal
	}

	createFarTable := CreateFARRequest{
		Key:  &farTable,
		Data: &farTabData,
	}

	g.log.Infoln(createFarTable)
	_, err = g.client.CreateFAR(context.Background(), &createFarTable)
	if err != nil {
		if strings.Contains(err.Error(), "ALREADY_EXISTS") {
		} else {
			g.log.Errorf("CreateFar Error: %v", err)
		}
	}

	return nil
}

func (g *P4Gtp5g) UpdateFAR(lseid uint64, req *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) RemoveFAR(lseid uint64, req *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) CreateQER(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) UpdateQER(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) RemoveQER(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) CreateURR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) UpdateURR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) RemoveURR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) CreateBAR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) UpdateBAR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) RemoveBAR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) HandleReport(handler report.Handler) {
	g.bs.Handle(handler)
}
