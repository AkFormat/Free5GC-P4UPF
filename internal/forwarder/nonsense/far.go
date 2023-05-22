package forwarder

import (
	p4config "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

/*
  table load_far_attributes {
        key = {
            local_meta.far.id : exact      @name("far_id");
            local_meta.fseid  : exact      @name("session_id");
        }
        actions = {
            load_normal_far_attributes;
            load_tunnel_far_attributes;
        }
    }

	action load_normal_far_attributes(bit<1> needs_dropping,
                                      bit<1> notify_cp)

	action load_tunnel_far_attributes(bit<1> needs_dropping,
                                    bit<1> notify_cp,
                                    bit<1> needs_buffering,
                                    TunnelType     tunnel_type,
                                    ipv4_addr_t    src_addr, //32
                                    ipv4_addr_t    dst_addr, // 32
                                    teid_t         teid,  // 32
                                    L4Port         sport) // 32
*/

func InsertFARtoSwitch(attr *FAREntryAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info, N3Addr []byte) error {
	tableOpt := NewOperator(FAR_TableName, p4info)
	var keyFields []*MatchKeys
	var actionParams [][]byte
	var actionName string

	keyFields = append(
		keyFields,
		IfWildcardMatchKey("exact", attr.FARID, nil, 0),
		IfWildcardMatchKey("exact", nil, nil, 0), // 忽略far中FSEID的匹配
	)

	if attr.forwardingParameter != nil {
		actionName = FAR_ActionTunnelName
		actionParams = [][]byte{
			attr.IfDrop,
			attr.IfBuffering,
			attr.TunnelType, // 要和 up4 中的东西对应好
			N3Addr,          // 需要获得UPF地址
			attr.forwardingParameter.IPv4Address,
			attr.forwardingParameter.TEID,
			attr.forwardingParameter.PortNumber,
		}
	} else {
		actionName = FAR_ActionNromalName
		actionParams = [][]byte{
			attr.IfDrop,
			attr.IfBuffering,
		}
	}
	entry := tableOpt.EntryBuilder(keyFields, actionName, actionParams, 0)
	err := InsertTableEntry(client, entry)
	return err
}

func DeleteFARtoSwitch() {}

func ModifyFARtoSwitch() {}
