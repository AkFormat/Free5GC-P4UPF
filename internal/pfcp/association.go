package pfcp

import (
	"net"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

func (s *PfcpServer) handleAssociationSetupRequest(
	req *message.AssociationSetupRequest,
	addr net.Addr,
) {
	s.log.Infoln("handleAssociationSetupRequest")

	if req.NodeID == nil {
		s.log.Errorln("not found NodeID")
		return
	}
	rnodeid, err := req.NodeID.NodeID() // 得到请求中的发送方的node ID(这里应该是smf的IP地址）
	if err != nil {
		s.log.Errorln(err)
		return
	}
	s.log.Debugf("remote nodeid: %v\n", rnodeid)

	// deleting the existing PFCP association and associated PFCP sessions,
	// if a PFCP association was already established for the Node ID
	// received in the request, regardless of the Recovery Timestamp
	// received in the request. 如果已经建立联系，则删除以前的联系信息，重新建立联系
	if node, ok := s.rnodes[rnodeid]; ok {
		s.log.Infof("delete node: %#+v\n", node)
		node.Reset()
		delete(s.rnodes, rnodeid)
	}
	node := s.NewNode(rnodeid, addr, s.driver) // 新建node信息，并加入字典
	s.rnodes[rnodeid] = node

	rsp := message.NewAssociationSetupResponse( // 生成新的response
		req.Header.SequenceNumber,
		newIeNodeID(s.nodeID), // 本地节点的ID
		ie.NewCause(ie.CauseRequestAccepted),
		ie.NewRecoveryTimeStamp(s.recoveryTime),
		// TODO:
		// ie.NewUPFunctionFeatures()，
	)

	err = s.sendRspTo(rsp, addr)
	if err != nil {
		s.log.Errorln(err)
		return
	}
}

func (s *PfcpServer) handleAssociationUpdateRequest(
	req *message.AssociationUpdateRequest,
	addr net.Addr,
) {
	s.log.Infoln("handleAssociationUpdateRequest not supported") // 不支持升级节点关联
}

func (s *PfcpServer) handleAssociationReleaseRequest(
	req *message.AssociationReleaseRequest,
	addr net.Addr,
) {
	s.log.Infoln("handleAssociationReleaseRequest not supported") // 不支持释放节点关联
}

func newIeNodeID(nodeID string) *ie.IE {
	ip := net.ParseIP(nodeID)
	if ip != nil {
		if ip.To4() != nil {
			return ie.NewNodeID(nodeID, "", "")
		}
		return ie.NewNodeID("", nodeID, "")
	}
	return ie.NewNodeID("", "", nodeID)
}
