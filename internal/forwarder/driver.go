package forwarder

import (
	"fmt"
	"net"
	"sync"

	"github.com/pkg/errors"
	"github.com/wmnsk/go-pfcp/ie"

	"github.com/free5gc/go-upf/internal/logger"
	"github.com/free5gc/go-upf/internal/report"
	"github.com/free5gc/go-upf/pkg/factory"
)

type Driver interface {
	Close()

	CreatePDR(uint64, *ie.IE) error
	UpdatePDR(uint64, *ie.IE) error
	RemovePDR(uint64, *ie.IE) error

	CreateFAR(uint64, *ie.IE) error
	UpdateFAR(uint64, *ie.IE) error
	RemoveFAR(uint64, *ie.IE) error

	CreateQER(uint64, *ie.IE) error
	UpdateQER(uint64, *ie.IE) error
	RemoveQER(uint64, *ie.IE) error

	CreateURR(uint64, *ie.IE) error
	UpdateURR(uint64, *ie.IE) error
	RemoveURR(uint64, *ie.IE) error

	CreateBAR(uint64, *ie.IE) error
	UpdateBAR(uint64, *ie.IE) error
	RemoveBAR(uint64, *ie.IE) error

	HandleReport(report.Handler)
}

func NewDriver(wg *sync.WaitGroup, cfg *factory.Config) (Driver, error) {
	cfgGtpu := cfg.Gtpu
	if cfgGtpu == nil {
		return nil, errors.Errorf("no Gtpu config")
	}

	logger.MainLog.Infof("starting Gtpu Forwarder [%s]", cfgGtpu.Forwarder)
	if cfgGtpu.Forwarder == "gtp5g" {
		var gtpuAddr string
		for _, ifInfo := range cfgGtpu.IfList {
			gtpuAddr = fmt.Sprintf("%s:%d", ifInfo.Addr, factory.UpfGtpDefaultPort)
			logger.MainLog.Infof("GTP Address: %q", gtpuAddr)
			break
		}
		if gtpuAddr == "" {
			return nil, errors.Errorf("not found GTP address")
		}
		driver, err := OpenGtp5g(wg, gtpuAddr) // 打开驱动 GTP驱动
		if err != nil {
			return nil, errors.Wrap(err, "open Gtp5g")
		}

		link := driver.Link()
		for _, dnn := range cfg.DnnList {
			_, dst, err := net.ParseCIDR(dnn.Cidr) // 解析DNN地址
			if err != nil {
				logger.MainLog.Errorln(err)
				continue
			}
			err = link.RouteAdd(dst) // 加入路由
			if err != nil {
				driver.Close()
				return nil, err
			}
			break
		}
		return driver, nil
	} // TODO 填入我们的驱动
	if cfgGtpu.Forwarder == "p4gtp5g" {

		driver, err := TofinoDriver(wg, cfg)
		if err != nil {
			return nil, err
		}
		return driver, nil
	}
	return nil, errors.Errorf("not support forwarder:%q", cfgGtpu.Forwarder)

}

func TofinoDriver(wg *sync.WaitGroup, cfg *factory.Config) (Driver, error) {
	var init InitParameters
	var err error
	cfgGNB := cfg.GNB
	if cfgGNB == nil {
		return nil, errors.Errorf("no Gtpu config")
	}
	logger.MainLog.Infof("***** TofinoDriver start *******************\n ")
	init.gNBAddr = cfgGNB.Addr // 获得GNB的mac

	init.gNBMac = cfgGNB.Mac // 获得mac地址
	logger.MainLog.Infof("********init.gNBMac: [%v]*******\n", init.gNBMac)

	cfgGtpu := cfg.Gtpu
	if cfgGtpu == nil {
		return nil, errors.Errorf("no Gtpu config")
	}
	for i, v := range cfgGtpu.IfList {
		if v.Type == "N3" { // N3, gnb -> upf 接口
			init.N3Mac = cfgGtpu.IfList[i].Mac            // 交换机 N3 接口 mac地址
			init.N3IPAddr = cfgGtpu.IfList[i].Addr        // 交换机 自定义 N3 IP地址
			init.N3SwitchPort = cfgGtpu.IfList[i].PortNum // 交换机 N3 规定的Port

		}
		if v.Type == "N6" {
			init.N6Mac = cfgGtpu.IfList[i].Mac            // 交换机 N6 接口mac地址
			init.N6IPAddr = cfgGtpu.IfList[i].Addr        // 交换机 N6 规定的ip地址
			init.N6SwitchPort = cfgGtpu.IfList[i].PortNum // 交换机 N6  规定的端口
		}
	}
	_, ipNet, err := net.ParseCIDR(cfg.DnnList[0].Cidr)
	if err != nil {
		return nil, err
	}
	init.UEIPNet = ipNet.IP.String() // UE地址族
	if init.UEIPNet == "" {
		return nil, fmt.Errorf("UEIPNet can not be parsed into IPv4")
	}
	_, ueipmask := ipNet.Mask.Size()
	init.UEIPMask = uint32(ueipmask) // UE地址掩码

	// 4. 获得DNN的IP地址和MAC地址
	init.DNNIPAddr = cfg.DnnList[0].Addr // DNN机器的IP地址

	init.DNNMac = cfg.DnnList[0].Dnnmac // DNN 机器的mac地址
	// 5. 获得UPF-D的gRPC地址
	init.gRPC = cfg.UPFC.GRPC

	driver, err := OpenP4Gtp5g(wg, &init) // python 加表控制器的grpc 地址
	if err != nil {
		return nil, err
	}
	return driver, nil

}

// func NewP4Driver(wg *sync.WaitGroup, cfg *factory.Config) (Driver, error) {
// 	// TODO: 初始化流程应该按着如下流程进行：
// 	// 1. 获得 UPF access 接口的mac地址和UPF DNN 接口的mac地址 (可以写入配置文件，然后从配置文件中得到）
// 	// 2. 使用上面的2个mac地址填写 my_station table的表项
// 	// 3. 获得 UE 对象池的IP地址，以及DNN方向的网络地址
// 	// 4. 用3中的获得地址，填写 source_iface_lookup 表项
// 	// 5. 获得 UPF IP地址，连接gnb的端口的信息（端口号，mac地址），gnb的IP地址；连接DNN的端口信息（端口号，Mac地址）以及DNN的地址
// 	// 6. 填充 routes_v4 的表项
// 	// 7. 返回driver对象

// 	//0. 提前声明变量
// 	var init InitParameters
// 	var err error
// 	//1. 获得 gnb IP MAC
// 	cfgGNB := cfg.GNB
// 	if cfgGNB == nil {
// 		return nil, errors.Errorf("no Gtpu config")
// 	}
// 	init.gNBAddr, err = utils.ConvertIpv4ToBytes(cfgGNB.Addr)
// 	if err != nil {
// 		return nil, err
// 	}
// 	init.gNBMac, err = net.ParseMAC(cfgGNB.Mac)
// 	if err != nil {
// 		return nil, err
// 	}

// 	//2. 获得N3接口的IP MAC和对应的交换端口和N6接口的IP MAC与交换机对应的端口
// 	cfgGtpu := cfg.Gtpu
// 	if cfgGtpu == nil {
// 		return nil, errors.Errorf("no Gtpu config")
// 	}
// 	for i, v := range cfgGtpu.IfList {
// 		if v.Type == "N3" {
// 			init.N3Mac, err = net.ParseMAC(cfgGtpu.IfList[i].Mac)
// 			if err != nil {
// 				return nil, err
// 			}
// 			init.N3IPAddr, err = utils.ConvertIpv4ToBytes(cfgGtpu.IfList[i].Addr)
// 			if err != nil {
// 				return nil, err
// 			}
// 			init.N3SwitchPort = []byte{cfgGtpu.IfList[i].PortNum}

// 		}
// 		if v.Type == "N6" {
// 			init.N6Mac, err = net.ParseMAC(cfgGtpu.IfList[i].Mac)
// 			if err != nil {
// 				return nil, err
// 			}
// 			init.N6IPAddr, err = utils.ConvertIpv4ToBytes(cfgGtpu.IfList[i].Addr)
// 			if err != nil {
// 				return nil, err
// 			}
// 			init.N6SwitchPort = []byte{cfgGtpu.IfList[i].PortNum}
// 		}
// 	}

// 	// 3. 获得 UE IP网络地址池，和掩码
// 	_, ipNet, err := net.ParseCIDR(cfg.DnnList[0].Cidr)
// 	if err != nil {
// 		return nil, err
// 	}
// 	init.UEIPNet = ipNet.IP.To4()
// 	if init.UEIPNet == nil {
// 		return nil, fmt.Errorf("UEIPNet can not be parsed into IPv4")
// 	}
// 	init.UEIPMask = ipNet.Mask

// 	// 4. 获得DNN的IP地址和MAC地址
// 	init.DNNIPAddr, err = utils.ConvertIpv4ToBytes(cfg.DnnList[0].Addr)
// 	if err != nil {
// 		return nil, err
// 	}
// 	init.DNNMac, err = net.ParseMAC(cfg.DnnList[0].Dnnmac)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// 5. 获得UPF-C的gRPC地址
// 	init.gRPC = cfg.UPFC.GRPC
// 	driver, err := OpenP4Gtp5g(wg, &init)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return driver, nil
// }
