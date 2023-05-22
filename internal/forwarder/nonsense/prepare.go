package forwarder

import (
	p4config "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

// InsertInterfaceTable
/*
action set_source_iface(InterfaceType src_iface, Direction direction) {
    // Interface type can be access, core, n6_lan, etc (see InterfaceType enum)
    // If interface is from the control plane, direction can be either up or down
    local_meta.src_iface = src_iface;
    local_meta.direction = direction;
}
table source_iface_lookup {
    key = {
        hdr.ipv4.dst_addr : lpm @name("ipv4_dst_prefix");
        // Eventually should also check VLAN ID here
    }
    actions = {
        set_source_iface;
    }
    const default_action = set_source_iface(InterfaceType.UNKNOWN, Direction.UNKNOWN);
}

*/
func InsertInterfaceTable(attr *SrcIfTypeAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info) error {
	tableOpt := NewOperator(SourceInterfaceLookup_TableName, p4info)
	var keyFields []*MatchKeys
	var actionParams [][]byte
	keyFields = append(
		keyFields,
		IfWildcardMatchKey("lpm", attr.SrcIPAddr, nil, attr.SrcIPMask),
	)
	actionParams = [][]byte{
		attr.SrcIfType,
		attr.Direction,
	}
	entry := tableOpt.EntryBuilder(keyFields, SourceInterfaceLookup_ActionName, actionParams, 0)
	err := InsertTableEntry(client, entry)
	if err != nil {
		return err
	}
	return nil
}

// InsertStationTable
/*
  table my_station {
        key = {
            hdr.ethernet.dst_addr : exact @name("dst_mac");
        }
        actions = {
            NoAction;
        }
    }
*/
func InsertStationTable(attr *StationAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info) error {
	tableOpt := NewOperator(Station_TableName, p4info)
	var keyFileds []*MatchKeys
	keyFileds = append(
		keyFileds,
		IfWildcardMatchKey("exact", attr.Mac, nil, 0),
	)
	entry := tableOpt.EntryBuilder(keyFileds, "NoAction", [][]byte{}, 0)
	err := InsertTableEntry(client, entry)
	if err != nil {
		return err
	}
	return nil
}

// InsertRouteTable
/*
control Routing(inout parsed_headers_t    hdr,
                inout local_metadata_t    local_meta,
                inout standard_metadata_t std_meta) {
    action drop() {
        mark_to_drop(std_meta);
    }

    // 选择出去的端口，并填充Mac地址
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
        @name("hashed_selector")
        implementation = action_selector(HashAlgorithm.crc16, 32w1024, 32w16); // TODO: action_selector 是什么
        size = MAX_ROUTES;
    }


    apply {
        // Normalize IP address for routing table, and decrement TTL
        // TODO: find a better alternative to this hack
        if (hdr.outer_ipv4.isValid()) {
            local_meta.next_hop_ip = hdr.outer_ipv4.dst_addr;
            hdr.outer_ipv4.ttl = hdr.outer_ipv4.ttl - 1;
        } else if (hdr.ipv4.isValid()){
            local_meta.next_hop_ip = hdr.ipv4.dst_addr;
            hdr.ipv4.ttl = hdr.ipv4.ttl - 1;

        }

        if (hdr.ipv4.ttl == 0) {
            drop();
        }
        else {
            routes_v4.apply();
        }
    }
}
*/
func InsertRouteTable(attr *RouteAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info) error {
	tableOpt := NewOperator(Route_TableName, p4info)
	var keyFields []*MatchKeys
	keyFields = append(
		keyFields,
		IfWildcardMatchKey("lpm", attr.NextHopIPAddr, nil, attr.NetHopIPMask),
	)
	entry, err := tableOpt.EntryBuilder4(keyFields, attr.ActionProfileGroupID, 0)
	if err != nil {
		return err
	}
	err = InsertTableEntry(client, entry)
	if err != nil {
		return err
	}
	return nil
}

func InstallActionProfileGroup(
	actionProfileName string,
	actionName string,
	actionParams [][][]byte,
	actionProfileGroupID int,
	client p4.P4RuntimeClient,
	p4info *p4config.P4Info,
) error {
	actionProfileMem, actionGroup, err := ActionProfileGroupCreateRequestBuilder3(
		actionProfileName,
		actionName,
		actionParams,
		actionProfileGroupID,
		p4info)
	if err != nil {
		return err
	}

	err = InsertActionProfileGroup2(client, actionProfileMem, actionGroup)
	if err != nil {
		return err
	}
	return nil
}
