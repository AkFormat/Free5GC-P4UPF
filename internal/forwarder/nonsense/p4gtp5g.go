package forwarder

import (
	"context"
	"encoding/binary"
	"github.com/free5gc/go-upf/pkg/factory"
	"sync"

	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"

	"github.com/free5gc/go-upf/internal/forwarder/buff"
	"github.com/free5gc/go-upf/internal/logger"
	"github.com/free5gc/go-upf/internal/report"

	"github.com/golang/protobuf/proto"
	p4config "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/grpc"
)

type P4Gtp5g struct {
	client         p4.P4RuntimeClient
	p4info         *p4config.P4Info
	bs             *buff.Server
	log            *logrus.Entry
	initParameters *InitParameters
}

/*
 * free5gc 对于PDR的规则加入，我认为是以精确规则加入的，即一个session，就对应一个PDR，不存在多个用户使用一个PDR规则（存疑）
 *
 * 但是up4的table用于多个用户使用一个PDR规则
 *
 */

func OpenP4Gtp5g(wg *sync.WaitGroup, parameters *InitParameters) (*P4Gtp5g, error) {
	// 0. 创建一个空的Driver对象
	driver := &P4Gtp5g{}
	// 1. 生成log对象
	driver.log = logger.FwderLog.WithField(logger.FieldCategory, "P4Gtp5g")
	driver.initParameters = parameters
	// 2. 连接交换机的P4runtime控制面, 并生成client对象
	conn, err := grpc.Dial(parameters.gRPC, grpc.WithInsecure())
	if err != nil {
		driver.log.Errorf("%v,连接硬件设备的控制面失败", err)
	}
	driver.client = p4.NewP4RuntimeClient(conn)

	// 3. 设置因为意外而出现的处理函数
	defer func(conn *grpc.ClientConn) {
		err := conn.Close()
		if err != nil {
			driver.log.Errorf("因为 %v, 关闭与硬件设备的grpc连接", err)
		}
	}(conn)

	// 4. 向硬件设备填充UPF数据面逻辑程序
	p4infoBytes, err := ioutil.ReadFile("../../config/main_p4rt.txt") // TODO: p4程序文件应该以配置文件的形式得到
	if err != nil {
		driver.log.Errorf("无法读取P4配置文件: %v", err)
	}

	driver.p4info = &p4config.P4Info{}
	err = proto.UnmarshalText(string(p4infoBytes), driver.p4info)
	if err != nil {
		driver.log.Errorf("无法解码P4info数据: %v", err)
	}

	bmv2Config, err := ioutil.ReadFile("../../config/main.json") // TODO: bmv2配置文件应该以配置文件的形式得到
	if err != nil {
		driver.log.Errorf("无法读取 BMv2 JSON 配置文件: %v", err)
	}

	p4DeviceConfig := &p4.ForwardingPipelineConfig{
		P4Info:         driver.p4info,
		P4DeviceConfig: bmv2Config,
	}

	req := &p4.SetForwardingPipelineConfigRequest{
		DeviceId: 1, //TODO: 该变量应该以配置文件的形式得到
		Action:   p4.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		Config:   p4DeviceConfig,
	}

	rsp, err := driver.client.SetForwardingPipelineConfig(context.Background(), req)
	if err != nil {
		driver.log.Errorf("发送 SetForwardingPipelineConfigRequest 请求失败：%v", err)
	}
	driver.log.Infoln("SetForwardingPipelineConfigRequest 发送成功, 响应为: %v", rsp)

	// *！ 5. 打开buffer服务器，但是目前不知道这个怎么用，先开这个吧
	bs, err := buff.OpenServer(wg, SOCKPATH)
	if err != nil {
		driver.Close()
		return nil, errors.Wrap(err, "open buff server")
	}
	driver.bs = bs

	// 6. 添加 lookup_interface table 的表项
	N3SrcIfTypeAttr := SrcIfTypeAttr{
		SrcIPAddr: parameters.N3IPAddr,
		SrcIPMask: 32,
		SrcIfType: []byte{ie.SrcInterfaceAccess},
		Direction: []byte{1},
	}
	N6SrcIfTypeAttr := SrcIfTypeAttr{
		SrcIPAddr: parameters.N6IPAddr,
		SrcIPMask: 32,
		SrcIfType: []byte{ie.SrcInterfaceCore},
		Direction: []byte{2},
	}
	err = InsertInterfaceTable(&N3SrcIfTypeAttr, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "Insert N3SrcIfTypeAttr")
	}
	err = InsertInterfaceTable(&N6SrcIfTypeAttr, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "Insert N6SrcIfTypeAttr")
	}

	// 7. 添加 my_station table 的表项, 该表用于鉴别数据包是不是发送到UPF端口的，防止广播包
	N3StationAttr := StationAttr{Mac: parameters.N3Mac}
	N6StationAttr := StationAttr{Mac: parameters.N6Mac}
	err = InsertStationTable(&N3StationAttr, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "Insert N3StationAttr")
	}
	err = InsertStationTable(&N6StationAttr, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "Insert N6StationAttr")
	}

	// 8. 添加 route 的表项
	// 	8.1 首先添加actionProfileGroup
	routeActionParams := [][][]byte{
		{driver.initParameters.N3Mac, driver.initParameters.gNBMac, driver.initParameters.N3SwitchPort},
		{driver.initParameters.N6Mac, driver.initParameters.DNNMac, driver.initParameters.N6SwitchPort},
	}
	err = InstallActionProfileGroup(Route_ActionProfileName, Route_ActionName,
		routeActionParams, Route_ActionProfileGroupID, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "InstallActionProfileGroup error")
	}
	// 8.2 添加对应的表项
	coreRouteAttr := RouteAttr{ // 出口到DNN网络
		NextHopIPAddr:        driver.initParameters.DNNIPAddr,
		NetHopIPMask:         32,
		ActionProfileGroupID: Route_ActionProfileGroupID,
	}
	accessRouteAttr := RouteAttr{ // 出口到gnb网络
		NextHopIPAddr:        driver.initParameters.gNBAddr,
		NetHopIPMask:         32,
		ActionProfileGroupID: Route_ActionProfileGroupID,
	}
	err = InsertRouteTable(&coreRouteAttr, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "InsertRouteTable coreRouteAttr error")
	}
	err = InsertRouteTable(&accessRouteAttr, driver.client, driver.p4info)
	if err != nil {
		return nil, errors.Wrap(err, "InsertRouteTable accessRouteAttr error")
	}
	// 9 至此，我们已经添加了路由表项，station表项和lookupSrc表项，可以返回driver对象了
	return driver, nil
}

func (g *P4Gtp5g) Close() {

	// TODO: 没有找到 p4.client close方法，只能使用如此不优雅的方法，希望将来换一种优雅方法
	if g.client != nil {
		g.client = nil
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
			attr.SourceInterface = []byte{v}
		case ie.DestinationInterface:

		case ie.FTEID: // Full TEID = TEID + ipv4
			v, err := x.FTEID()
			if err != nil {
				break
			}
			attr.TEID = make([]byte, 4)
			binary.BigEndian.PutUint32(attr.TEID, v.TEID)
			attr.GTPuAddr = v.IPv4Address // 这个是UPF的地址

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
			attr.UEIPv4 = v.IPv4Address
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

func (g *P4Gtp5g) CreatePDR(lSeid uint64, req *ie.IE) error {
	// lSeid 在free5gc的定义里是local session id
	/*
	  中心思想是，读取IE中的每一个元素，将其对应上Table Insert函数中的参数, 参数是一个结构体，需要将结构体填充
	*/
	var pdrEntry PDREntryAttr
	pdrEntry.FSEID = make([]byte, 12) // !警告：在UP4中，fseid = session id， 但是free5gc只传给local session id，在free5gc中，free5gc中 session id = remote session id
	binary.BigEndian.PutUint64(pdrEntry.FSEID, lSeid)

	ies, err := req.CreatePDR()
	if err != nil {
		return err
	}
	for _, i := range ies {
		switch i.Type {
		case ie.PDRID:
			pdrid, err := i.PDRID()
			if err != nil {
				break
			}
			pdrEntry.PDRID = make([]byte, 2)
			pdrEntry.CounterIndex = make([]byte, 2)
			binary.BigEndian.PutUint16(pdrEntry.PDRID, pdrid)        // 对应PDR table中的 action 参数
			binary.BigEndian.PutUint16(pdrEntry.CounterIndex, pdrid) // 对应PDR table中的 action 参数
			precedence, err := i.Precedence()
			if err != nil {
				break
			}
			pdrEntry.Priority = precedence //PDR规则优先级，uint32类型，对应三态匹配的优先级

		case ie.PDI:
			pdi, err := g.newPdi(i)
			if err != nil {
				break
			}
			pdrEntry.TEID = pdi.TEID                       // pdr TEID
			pdrEntry.TunnelIpv4Dst = pdi.GTPuAddr          // pdr TunnelIpv4Dst, UPF的地址
			pdrEntry.SourceInterface = pdi.SourceInterface // pdr SourceInterface
			if pdi.SDFFilter != nil {                      // 判断SDFFilter是否存在
				if pdi.SDFFilter.FD.Action == "out" { // 下行流量
					pdrEntry.UEAddr = pdi.SDFFilter.FD.Dst.IP.To4()
					pdrEntry.INETAddr = pdi.SDFFilter.FD.Src.IP.To4()
					pdrEntry.INETL4Port = Uint16ToByte(pdi.SDFFilter.FD.SrcPorts)
					pdrEntry.UEL4Port = Uint16ToByte(pdi.SDFFilter.FD.DstPorts)
				}

				if pdi.SDFFilter.FD.Action == "in" { // 上行流量
					pdrEntry.UEAddr = pdi.SDFFilter.FD.Src.IP.To4()
					pdrEntry.INETAddr = pdi.SDFFilter.FD.Dst.IP.To4()
					pdrEntry.INETL4Port = Uint16ToByte(pdi.SDFFilter.FD.SrcPorts)
					pdrEntry.UEL4Port = Uint16ToByte(pdi.SDFFilter.FD.DstPorts)
				}
			}

		case ie.OuterHeaderRemoval:
			_, err := i.OuterHeaderRemovalDescription() // pdr ifDeGTP
			if err != nil {
				break
			}
			pdrEntry.IfGtpuDecap = []byte{1} // 只要存在这个字段，就是需要拆除外部字段，所以无需在意字段的值
			// ignore GTPUExternsionHeaderDeletion
		case ie.FARID:
			farid, err := i.FARID() // 对应 pdr FARID
			if err != nil {
				break
			}
			pdrEntry.FARID = make([]byte, 4)
			binary.BigEndian.PutUint32(pdrEntry.FARID, farid)
		}
	}

	err = InsertPDRtoSwitch(&pdrEntry, g.client, g.p4info)
	if err != nil {
		return err
	}

	return nil
}

func (g *P4Gtp5g) UpdatePDR(lSeid uint64, req *ie.IE) error {
	//
	//var pdrEntry PDREntryAttr
	//ies, err := req.UpdatePDR()
	//if err != nil {
	//	return err
	//}
	//for _, i := range ies {
	//	switch i.Type {
	//	case ie.PDRID:
	//		pdrid, err := i.PDRID() // 对应PDR table中的 action 参数
	//		if err != nil {
	//			break
	//		}
	//		pdrEntry.PDRID = make([]byte, 2)
	//		binary.BigEndian.PutUint16(pdrEntry.PDRID, pdrid)
	//		precedence, err := i.Precedence() //PDR规则优先级，uint32类型，对应三态匹配的优先级
	//		if err != nil {
	//			break
	//		}
	//		pdrEntry.Priority = precedence
	//	case ie.PDI:
	//		pdi, err := g.newPdi(i)
	//		if err != nil {
	//			break
	//		}
	//		pdrEntry.TEID = pdi.TEID
	//		pdrEntry.TunnelIpv4Dst = pdi.GTPuAddr
	//		// TODO: FSEID
	//		pdrEntry.SourceInterface = pdi.SourceInterface
	//	case ie.OuterHeaderRemoval:
	//		des, err := i.OuterHeaderRemovalDescription() // 对应 action中参数
	//		if err != nil {
	//			break
	//		}
	//		pdrEntry.IfGtpuDecap = []byte{des}
	//	// ignore GTPUExternsionHeaderDeletion
	//	case ie.FARID:
	//		farid, err := i.FARID() // 对应 action中的参数
	//		if err != nil {
	//			break
	//		}
	//		pdrEntry.FARID = make([]byte, 4)
	//		binary.BigEndian.PutUint32(pdrEntry.FARID, farid)
	//	}
	//}
	return nil
}

func (g *P4Gtp5g) RemovePDR(uint64, *ie.IE) error {
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
			fd.OuterHeaderCreationDescription = make([]byte, 2)
			binary.BigEndian.PutUint16(fd.OuterHeaderCreationDescription, v.OuterHeaderCreationDescription)

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
				fd.TEID = make([]byte, 4)
				binary.BigEndian.PutUint32(fd.TEID, v.TEID)
				// GTPv1-U port
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_PORT,
				//	Value: nl.AttrU16(factory.UpfGtpDefaultPort),
				//})
				fd.PortNumber = make([]byte, 2)
				binary.BigEndian.PutUint16(fd.PortNumber, factory.UpfGtpDefaultPort)

			} else {
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_PORT,
				//	Value: nl.AttrU16(v.PortNumber),
				//})
				fd.PortNumber = make([]byte, 2)
				binary.BigEndian.PutUint16(fd.PortNumber, v.PortNumber)
			}
			if x.HasIPv4() {
				//hc = append(hc, nl.Attr{
				//	Type:  gtp5gnl.OUTER_HEADER_CREATION_PEER_ADDR_IPV4,
				//	Value: nl.AttrBytes(v.IPv4Address),
				//})
				fd.IPv4Address = v.IPv4Address.To4()
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

func (g *P4Gtp5g) CreateFAR(lSeid uint64, req *ie.IE) error {
	var FAREntryAttr *FAREntryAttr
	FAREntryAttr.FSEID = make([]byte, 8)
	binary.BigEndian.PutUint64(FAREntryAttr.FSEID, lSeid) // !Warning: lSeid != FSEID
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
			FAREntryAttr.FARID = make([]byte, 4)
			binary.BigEndian.PutUint32(FAREntryAttr.FARID, v)
		case ie.ApplyAction:
			v, err := i.ApplyAction()
			if err != nil {
				return err
			}
			if isBitSet(int(v), 0) {
				FAREntryAttr.IfDrop = []byte{1}
			} else {
				FAREntryAttr.IfDrop = []byte{0}
			}
			if isBitSet(int(v), 1) {
				FAREntryAttr.Forward = true
			} else {
				FAREntryAttr.Forward = false
			}
			if isBitSet(int(v), 2) {
				FAREntryAttr.IfBuffering = []byte{1}
			} else {
				FAREntryAttr.IfBuffering = []byte{0}
			}
			if isBitSet(int(v), 3) {
				FAREntryAttr.NotifyCP = []byte{1}
			} else {
				FAREntryAttr.NotifyCP = []byte{0}
			}

		case ie.ForwardingParameters:
			xs, err := i.ForwardingParameters() // 生成转发参数
			if err != nil {
				return err
			}
			v, err := g.newForwardingParameter(xs)
			if err != nil {
				break
			}
			FAREntryAttr.TunnelType = v.OuterHeaderCreationDescription
			FAREntryAttr.forwardingParameter = v
		}
	}
	err = InsertFARtoSwitch(FAREntryAttr, g.client, g.p4info, nil)
	if err != nil {
		return err
	}

	return nil
}

func (g *P4Gtp5g) UpdateFAR(uint64, *ie.IE) error {
	return nil
}

func (g *P4Gtp5g) RemoveFAR(uint64, *ie.IE) error {
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
