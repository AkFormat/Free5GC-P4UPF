package pfcp

import (
	"encoding/hex"
	"fmt"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/message"

	"github.com/free5gc/go-upf/internal/forwarder"
	"github.com/free5gc/go-upf/internal/logger"
	"github.com/free5gc/go-upf/internal/report"
	"github.com/free5gc/go-upf/pkg/factory"
)

const (
	RECEIVE_CHANNEL_LEN       = 512 // 通道缓冲大小为 512，这个限制了pfcp命令的数据包
	REPORT_CHANNEL_LEN        = 64
	TRANS_TIMEOUT_CHANNEL_LEN = 64
	MAX_PFCP_MSG_LEN          = 1500 // PFCP 数据包为 1500
)

type ReceivePacket struct {
	RemoteAddr net.Addr
	Buf        []byte
}

type TransType int

const (
	TX TransType = iota
	RX
)

type TransactionTimeout struct {
	TrType TransType
	TrID   string
}

type PfcpServer struct {
	cfg          *factory.Config           // 配置文件对象
	listen       string                    // 监听的网络地址
	nodeID       string                    // 本地PFCP服务的ID，此处应该是pfcp绑定的IP地址
	rcvCh        chan ReceivePacket        // 接收packet的通道
	srCh         chan report.SessReport    // session report通道
	trToCh       chan TransactionTimeout   // 超时事件通道
	conn         *net.UDPConn              // pfcp UP网络连接
	recoveryTime time.Time                 //
	driver       forwarder.Driver          // 数据面服务驱动
	lnode        LocalNode                 // 本地节点
	rnodes       map[string]*RemoteNode    // 与之关联的远程节点
	txTrans      map[string]*TxTransaction // key: RemoteAddr-Sequence 存放发数据包送事件,
	rxTrans      map[string]*RxTransaction // key: RemoteAddr-Sequence 存放接收数据包事件
	txSeq        uint32                    // 发送序号？
	log          *logrus.Entry
}

func NewPfcpServer(cfg *factory.Config, driver forwarder.Driver) *PfcpServer {
	listen := fmt.Sprintf("%s:%d", cfg.Pfcp.Addr, factory.UpfPfcpDefaultPort)
	return &PfcpServer{
		cfg:          cfg,
		listen:       listen,
		nodeID:       cfg.Pfcp.NodeID,
		rcvCh:        make(chan ReceivePacket, RECEIVE_CHANNEL_LEN),
		srCh:         make(chan report.SessReport, REPORT_CHANNEL_LEN),
		trToCh:       make(chan TransactionTimeout, TRANS_TIMEOUT_CHANNEL_LEN),
		recoveryTime: time.Now(),
		driver:       driver,
		rnodes:       make(map[string]*RemoteNode),
		txTrans:      make(map[string]*TxTransaction),
		rxTrans:      make(map[string]*RxTransaction),
		log:          logger.PfcpLog.WithField(logger.FieldListenAddr, listen),
	}
}

func (s *PfcpServer) main(wg *sync.WaitGroup) {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			s.log.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}

		s.log.Infoln("pfcp server stopped")
		s.stopTrTimers()
		close(s.rcvCh)
		close(s.srCh)
		close(s.trToCh)
		wg.Done()
	}()

	var err error
	laddr, err := net.ResolveUDPAddr("udp", s.listen) // 解析从配置文件中的得到的ip地址
	if err != nil {
		s.log.Errorf("Resolve err: %+v", err)
		return
	}

	conn, err := net.ListenUDP("udp", laddr) // 绑定IP地址
	if err != nil {
		s.log.Errorf("Listen err: %+v", err)
		return
	}
	s.conn = conn

	wg.Add(1)         // 设置并行barrier数量为1
	go s.receiver(wg) // 并行执行 PFCP receiver 函数

	for { // 无限循环，处理各个 goroutine 关联通道发来的信号
		select {
		case sr := <-s.srCh: // 当收到了session request消息时
			s.ServeReport(&sr)
		case rcvPkt := <-s.rcvCh: // 接收到了数据包
			if len(rcvPkt.Buf) == 0 {
				// receiver closed
				return
			}
			msg, err := message.Parse(rcvPkt.Buf) // 解析该数据包
			if err != nil {
				s.log.Errorln(err)
				s.log.Tracef("ignored undecodable message:\n%+v", hex.Dump(rcvPkt.Buf))
				continue
			}

			trID := fmt.Sprintf("%s-%d", rcvPkt.RemoteAddr, msg.Sequence()) // 新来的数据包当做一个事物对待，远程地址 + pfcp的序列号 作为事物的ID
			if isRequest(msg) {                                             // 如果是request报文
				rx, ok := s.rxTrans[trID] // 查到trID的key是否有对应的value
				if !ok {
					rx = NewRxTransaction(s, rcvPkt.RemoteAddr, msg.Sequence()) // 创建新的Rx事务
					s.rxTrans[trID] = rx                                        // 存储到字典
				}
				needDispatch, err1 := rx.recv(msg) // 判断是否需要处理该request（之前发送的response是否发过去了）
				if err1 != nil {
					s.log.Warnf("rcvCh: %v", err1)
					continue
				} else if !needDispatch { // 不需要处理，重新开始循环
					s.log.Debugf("rcvCh: rxtr[%s] req no need to dispatch", trID)
					continue
				}
				err = s.reqDispacher(msg, rcvPkt.RemoteAddr) // 开始分配request给对应函数,真正干活的函数
				if err != nil {
					s.log.Errorln(err)
					s.log.Tracef("ignored undecodable message:\n%+v", hex.Dump(rcvPkt.Buf))
				}
			} else if isResponse(msg) { // 如果数据包是response
				tx, ok := s.txTrans[trID] // 查看字典里是否有发送事务
				if !ok {
					s.log.Debugf("rcvCh: No txtr[%s] found for rsp", trID)
					continue
				}
				req := tx.recv(msg)
				err = s.rspDispacher(msg, rcvPkt.RemoteAddr, req) // 将该rsp分配给合适的函数进行处理
				if err != nil {
					s.log.Errorln(err)
					s.log.Tracef("ignored undecodable message:\n%+v", hex.Dump(rcvPkt.Buf))
				}
			}
		case trTo := <-s.trToCh:
			if trTo.TrType == TX { // 如果超时时间是TX
				tx, ok := s.txTrans[trTo.TrID]
				if !ok {
					s.log.Warnf("trToCh: txtr[%s] not found", trTo.TrID)
					continue
				}
				tx.handleTimeout()
			} else { // RX
				rx, ok := s.rxTrans[trTo.TrID]
				if !ok {
					s.log.Warnf("trToCh: rxtr[%s] not found", trTo.TrID)
					continue
				}
				rx.handleTimeout()
			}
		}
	}
}

func (s *PfcpServer) receiver(wg *sync.WaitGroup) {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			s.log.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}

		s.log.Infoln("pfcp reciver stopped")
		wg.Done()
	}()

	for {
		buf := make([]byte, MAX_PFCP_MSG_LEN) // 开辟 PFCP msg的长度的缓冲区
		n, addr, err := s.conn.ReadFrom(buf)  // 函数返回 长度，远程地址和可能出现的错误
		if err != nil {
			s.log.Errorf("%+v", err)
			s.rcvCh <- ReceivePacket{} // 如果出错，则返回空值
			break
		}
		s.rcvCh <- ReceivePacket{ // 返回正常的数据包
			RemoteAddr: addr,
			Buf:        buf[:n],
		}
	}
}

func (s *PfcpServer) Start(wg *sync.WaitGroup) {
	s.log.Infoln("starting pfcp server")
	wg.Add(1)
	go s.main(wg)
	s.log.Infoln("pfcp server started")
}

func (s *PfcpServer) Stop() {
	s.log.Infoln("Stopping pfcp server")
	if s.conn != nil {
		err := s.conn.Close()
		if err != nil {
			s.log.Errorf("Stop pfcp server err: %+v", err)
		}
	}
}

func (s *PfcpServer) NewNode(id string, addr net.Addr, driver forwarder.Driver) *RemoteNode {
	n := NewRemoteNode(
		id,
		addr,
		&s.lnode,
		driver,
		s.log.WithField(logger.FieldRemoteNodeID, "rNodeID:"+id),
	)
	n.log.Infoln("New node")
	return n
}

func (s *PfcpServer) UpdateNodeID(n *RemoteNode, newId string) {
	s.log.Infof("Update nodeId %q to %q", n.ID, newId)
	delete(s.rnodes, n.ID)
	n.ID = newId
	n.log = s.log.WithField(logger.FieldRemoteNodeID, "rNodeID:"+newId)
	s.rnodes[newId] = n
}

func (s *PfcpServer) NotifySessReport(sr report.SessReport) {
	s.srCh <- sr
}

func (s *PfcpServer) NotifyTransTimeout(trType TransType, trID string) {
	s.trToCh <- TransactionTimeout{TrType: trType, TrID: trID}
}

func (s *PfcpServer) PopBufPkt(seid uint64, pdrid uint16) ([]byte, bool) {
	sess, err := s.lnode.Sess(seid)
	if err != nil {
		s.log.Errorln(err)
		return nil, false
	}
	return sess.Pop(pdrid)
}

//
func (s *PfcpServer) sendReqTo(msg message.Message, addr net.Addr) error {
	if !isRequest(msg) {
		return errors.Errorf("sendReqTo: invalid req type(%d)", msg.MessageType())
	}

	txtr := NewTxTransaction(s, addr, s.txSeq)
	s.txSeq++
	s.txTrans[txtr.id] = txtr

	return txtr.send(msg)
}

func (s *PfcpServer) sendRspTo(msg message.Message, addr net.Addr) error {
	if !isResponse(msg) {
		return errors.Errorf("sendRspTo: invalid rsp type(%d)", msg.MessageType())
	}

	// find transaction
	trID := fmt.Sprintf("%s-%d", addr, msg.Sequence())
	rxtr, ok := s.rxTrans[trID]
	if !ok {
		return errors.Errorf("sendRspTo: rxtr(%s) not found", trID)
	}

	return rxtr.send(msg)
}

func (s *PfcpServer) stopTrTimers() {
	for _, tx := range s.txTrans {
		if tx.timer == nil {
			continue
		}
		tx.timer.Stop()
		tx.timer = nil
	}
	for _, rx := range s.rxTrans {
		if rx.timer == nil {
			continue
		}
		rx.timer.Stop()
		rx.timer = nil
	}
}

func isRequest(msg message.Message) bool {
	switch msg.MessageType() {
	case message.MsgTypeHeartbeatRequest:
		return true
	case message.MsgTypePFDManagementRequest:
		return true
	case message.MsgTypeAssociationSetupRequest:
		return true
	case message.MsgTypeAssociationUpdateRequest:
		return true
	case message.MsgTypeAssociationReleaseRequest:
		return true
	case message.MsgTypeNodeReportRequest:
		return true
	case message.MsgTypeSessionSetDeletionRequest:
		return true
	case message.MsgTypeSessionEstablishmentRequest:
		return true
	case message.MsgTypeSessionModificationRequest:
		return true
	case message.MsgTypeSessionDeletionRequest:
		return true
	case message.MsgTypeSessionReportRequest:
		return true
	default:
	}
	return false
}

func isResponse(msg message.Message) bool {
	switch msg.MessageType() {
	case message.MsgTypeHeartbeatResponse:
		return true
	case message.MsgTypePFDManagementResponse:
		return true
	case message.MsgTypeAssociationSetupResponse:
		return true
	case message.MsgTypeAssociationUpdateResponse:
		return true
	case message.MsgTypeAssociationReleaseResponse:
		return true
	case message.MsgTypeNodeReportResponse:
		return true
	case message.MsgTypeSessionSetDeletionResponse:
		return true
	case message.MsgTypeSessionEstablishmentResponse:
		return true
	case message.MsgTypeSessionModificationResponse:
		return true
	case message.MsgTypeSessionDeletionResponse:
		return true
	case message.MsgTypeSessionReportResponse:
		return true
	default:
	}
	return false
}

func setReqSeq(msgtmp message.Message, seq uint32) {
	switch msg := msgtmp.(type) {
	case *message.HeartbeatRequest:
		msg.SetSequenceNumber(seq)
	case *message.PFDManagementRequest:
		msg.SetSequenceNumber(seq)
	case *message.AssociationSetupRequest:
		msg.SetSequenceNumber(seq)
	case *message.AssociationUpdateRequest:
		msg.SetSequenceNumber(seq)
	case *message.AssociationReleaseRequest:
		msg.SetSequenceNumber(seq)
	case *message.NodeReportRequest:
		msg.SetSequenceNumber(seq)
	case *message.SessionSetDeletionRequest:
		msg.SetSequenceNumber(seq)
	case *message.SessionEstablishmentRequest:
		msg.SetSequenceNumber(seq)
	case *message.SessionModificationRequest:
		msg.SetSequenceNumber(seq)
	case *message.SessionDeletionRequest:
		msg.SetSequenceNumber(seq)
	case *message.SessionReportRequest:
		msg.SetSequenceNumber(seq)
	default:
	}
}
