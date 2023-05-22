package pfcp

import (
	"net"

	"github.com/pkg/errors"
	"github.com/wmnsk/go-pfcp/message"
)

func (s *PfcpServer) reqDispacher(msg message.Message, addr net.Addr) error {
	switch req := msg.(type) {
	case *message.HeartbeatRequest: // 心跳
		s.handleHeartbeatRequest(req, addr)
	case *message.AssociationSetupRequest: // 节点关联请求
		s.handleAssociationSetupRequest(req, addr)
	case *message.AssociationUpdateRequest: // 节点关联更新请求
		s.handleAssociationUpdateRequest(req, addr)
	case *message.AssociationReleaseRequest: // 节点关联释放请求
		s.handleAssociationReleaseRequest(req, addr)
	case *message.SessionEstablishmentRequest: // 回话建立请求
		s.handleSessionEstablishmentRequest(req, addr)
	case *message.SessionModificationRequest: // 回话修改请求
		s.handleSessionModificationRequest(req, addr)
	case *message.SessionDeletionRequest: // 回话删除请求
		s.handleSessionDeletionRequest(req, addr)
	default:
		return errors.Errorf("pfcp reqDispacher unknown msg type: %d", msg.MessageType())
	}
	return nil
}

func (s *PfcpServer) rspDispacher(msg message.Message, addr net.Addr, req message.Message) error {
	switch rsp := msg.(type) {
	case *message.SessionReportResponse: // 回话报告相应
		s.handleSessionReportResponse(rsp, addr, req)
	default:
		return errors.Errorf("pfcp rspDispacher unknown msg type: %d", msg.MessageType())
	}
	return nil
}

func (s *PfcpServer) txtoDispacher(msg message.Message, addr net.Addr) error {
	switch req := msg.(type) {
	case *message.SessionReportRequest:
		s.handleSessionReportRequestTimeout(req, addr)
	default:
		return errors.Errorf("pfcp txtoDispacher unknown msg type: %d", msg.MessageType())
	}
	return nil
}
