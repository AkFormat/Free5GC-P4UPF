package main

import (
	"context"
	"fmt"
	"log"

	"github.com/free5gc/go-upf/internal/forwarder"
	"google.golang.org/grpc"
)

func main() {
	bit_32_full := 4294967295
	conn, err := grpc.Dial("192.168.1.56:50021", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("could not connect to server: %v", err)
	}
	defer func(conn *grpc.ClientConn) {
		err := conn.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(conn)

	client := forwarder.NewGrpcServiceClient(conn)
	//request := forwarder.Request{Request: "HELLO"}
	// request := forwarder.CreateMyStationTableRequest{
	// 	Key:  &forwarder.MyStationTableKey{DstMac: "00:0c:29:a4:93:0a"},
	// 	Data: &forwarder.MyStationTableData{Action: forwarder.MyStationTableData_no_action},
	// }

	// request := forwarder.CreateRouteTableRequest{
	// 	Key:  &forwarder.RouteTableKey{NextHopIp: "192.168.1.1", NextHopIpPrefixLen: 24},
	// 	Data: &forwarder.RouteTableData{Action: forwarder.RouteTableData_route, RouteAction_1: &forwarder.RouteTableData_ActionRoute{SrcMac: "01:0c:29:a4:93:0a", DstMac: "02:0c:29:a4:93:0a", EgressPort: 65}},
	// }

	// request := forwarder.CreateRouteTableRequest{
	// 	Key:  &forwarder.RouteTableKey{NextHopIp: "192.168.100.100", NextHopIpPrefixLen: 24},
	// 	Data: &forwarder.RouteTableData{Action: forwarder.RouteTableData_route, RouteAction_1: &forwarder.RouteTableData_ActionRoute{SrcMac: "01:0c:29:a4:93:01", DstMac: "02:0c:29:a4:93:03", EgressPort: 65}},
	// }

	// request := forwarder.CreateSrcIfLookTableRequest{
	// 	Key:  &forwarder.SrcIfLookupTableKey{DstAddr: "192.168.20.5", DstAddrPrefixLen: 24},
	// 	Data: &forwarder.SrcIfLookupTableData{Action: forwarder.SrcIfLookupTableData_set_source_iface, SrcIfLookupAction_1: &forwarder.SrcIfLookupTableData_ActionSetSourceIface{SrcIface: 1, Direction: 1}},
	// }

	request := forwarder.CreatePDRRequest{
		Key: &forwarder.PDRTableKey{
			SrcIface:          1,
			TunnelIpv4Dst:     "192.168.5.1",
			TunnelIpv4DstMask: "255.255.255.0",
			Teid:              2,
			TeidMask:          uint32(bit_32_full),
			//UeAddr:            "192.168.5.6",
			// UeAddrMask:   "255.255.255.0",
			// InetAddr:     "10.1.1.1",
			// InetAddrMask: "255.255.255.0",
			// //UeL4PortLow:    2005,
			// UeL4PortHigh:   2010,
			InetL4PortLow:  1,
			InetL4PortHigh: 9999,
			// Ipv4Proto:      18,
			// Ipv4ProtoMask:  0,
			// HasQfi:         false,
			// HasQfiMask:     1,
			// Qfi:            0,
			// QfiMask:        0,
			Priority: 21,
		},

		Data: &forwarder.PDRTableData{
			Action: forwarder.PDRTableData_set_pdr_attributes,
			PdrAction_1: &forwarder.PDRTableData_ActionSetPDRAttributes{
				PdrId:          16,
				Fseid:          14,
				FarId:          11,
				NeedsGtpuDecap: true,
				// 			//NeedsQfiPush:   false,
				// 			//Qfi:            2,
			},
		},
	}

	// requst := forwarder.CreateFARRequest{
	// 	Key: &forwarder.FARTableKey{
	// 		FarId: 3,
	// 		Fseid: 14,
	// 	},

	// 	Data: &forwarder.FARTableData{
	// 		Action: forwarder.FARTableData_load_normal_far_attributes,
	// 		FarAction_1: &forwarder.FARTableData_ActionLoadNormalFarAttributes{
	// 			NeedsDropping: true,
	// 			NotifyCp:      true,
	// 		},
	// 	},
	// }

	//requst := forwarder.CreateFARRequest{
	//	Key: &forwarder.FARTableKey{
	//		FarId: 12,
	//		Fseid: 15,
	//	},
	//
	//	Data: &forwarder.FARTableData{
	//		Action: forwarder.FARTableData_load_tunnel_far_attributes,
	//		FarAction_2: &forwarder.FARTableData_ActionLoadTunnelFarAttributes{
	//			NeedsDropping:  false,
	//			NotifyCp:       true,
	//			NeedsBuffering: false,
	//			TunnelType:     1,
	//			SrcAddr:        "192.168.1.20",
	//			DstAddr:        "10.1.1.1",
	//			Teid:           15,
	//			L4Port:         2512,
	//		},
	//	},
	//}

	fmt.Println(request)
	reply, err := client.CreatePDR(context.Background(), &request)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(reply.Response))
	// reply, err := client.CreateRoute(context.Background(), &request3)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(string(reply.Response))
}
