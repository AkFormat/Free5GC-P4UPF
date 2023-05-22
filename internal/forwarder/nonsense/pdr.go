package forwarder

import (
	p4config "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

func InsertPDRtoSwitch(attr *PDREntryAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info) error {
	// 在free5gc中，PDR规则包括：PDR ID,Precedence,PDI,OuterHeaderRemoval,FARID，QERID，URRID 这些字段
	// 2023年4月18日：目前，由于free5gc对于PDR规则的添加是一条一条的，并没有复用的情况，因此，我们使用精确版本的三态匹配
	// PDR table 的P4代码如下：
	/*
		table pdrs {
			key = {
				// PDI
				local_meta.src_iface        : exact     @name("src_iface"); // To differentiate uplink and downlink
				hdr.outer_ipv4.dst_addr     : ternary   @name("tunnel_ipv4_dst"); // combines with TEID to make F-TEID
				local_meta.teid             : ternary   @name("teid");
				// One SDF filter from a PDR's filter set
				local_meta.ue_addr          : ternary   @name("ue_addr");
				local_meta.inet_addr        : ternary   @name("inet_addr");
				local_meta.ue_l4_port       : range     @name("ue_l4_port");
				local_meta.inet_l4_port     : range     @name("inet_l4_port");
				hdr.ipv4.proto              : ternary   @name("ip_proto");
				// Match on QFI, valid for 5G traffic only
				hdr.gtpu_ext_psc.isValid()  : ternary  @name("has_qfi");
				hdr.gtpu_ext_psc.qfi        : ternary  @name("qfi");
			}
			actions = {
				set_pdr_attributes;
				set_pdr_attributes_qos;
			}
		}

	*/

	tableOpt := NewOperator(PDR_TableName, p4info)
	var keyFields []*MatchKeys
	var actionParams [][]byte
	mask32, _ := EasyHexDecodeString("ffffffff") // 一个16进制数占4bit，所以需要8个f
	mask16, _ := EasyHexDecodeString("ffff")     // 16 bit mask
	mask6, _ := EasyHexDecodeString("3f")        // 6bit mask

	// 字段的顺序很重要！所以添加不要乱了顺序，

	keyFields = append(
		keyFields,
		IfWildcardMatchKey("exact", attr.SourceInterface, nil, 0),
		IfWildcardMatchKey("ternary", attr.TunnelIpv4Dst, mask32, 0),
		IfWildcardMatchKey("ternary", attr.TEID, mask32, 0),
		IfWildcardMatchKey("ternary", attr.UEAddr, mask32, 0),
		IfWildcardMatchKey("ternary", attr.INETAddr, mask32, 0),
		IfWildcardMatchKey("range", attr.UEL4Port[0], attr.UEL4Port[0], 0),     // 变相精确匹配
		IfWildcardMatchKey("range", attr.INETL4Port[0], attr.INETL4Port[0], 0), // 变相精确匹配
		IfWildcardMatchKey("ternary", attr.IPProto, mask16, 0),
		IfWildcardMatchKey("ternary", attr.IfPSC, []byte{0}, 0), // 忽略
		IfWildcardMatchKey("ternary", attr.PSCQFI, mask6, 0),    // 忽略
	)

	//在UP4中，考虑了QoS的情况，例如QFI和psc,但是free5gc的PDR规则并没有这一项，因此我们忽略他们
	// 所以 action 只用 set_pdr_attributes， 无论是上行还是下行
	/*
		action set_pdr_attributes(pdr_id_t          id,
								fseid_t           fseid,
								counter_index_t   ctr_id,
								far_id_t          far_id,
								bit<1>            needs_gtpu_decap )
	*/
	actionParams = [][]byte{
		attr.PDRID, attr.FSEID, attr.CounterIndex, attr.FARID, attr.IfGtpuDecap,
	}

	req := tableOpt.EntryBuilder(keyFields, PDR_ActionName, actionParams, int32(attr.Priority))
	err := InsertTableEntry(client, req)
	if err != nil {
		return err
	}
	return nil
}

func DeletePDRtoSwitch(attr *PDREntryAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info) error {
	// TODO: 删除PDR表项
	return nil
}

func ModifyPDRtoSwitch(attr *PDREntryAttr, client p4.P4RuntimeClient, p4info *p4config.P4Info) error {
	return nil
}
