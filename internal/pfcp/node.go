package pfcp

import (
	"fmt"
	"net"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"

	"github.com/free5gc/go-upf/internal/forwarder"
	"github.com/free5gc/go-upf/internal/logger"
)

const (
	BUFFQ_LEN = 512
)

// ************************* Session 对象以及相关方法 **************************

type Sess struct { // session 会话型数据包
	rnode    *RemoteNode         // 远程实例
	LocalID  uint64              // 本地session id
	RemoteID uint64              // 远程 session id
	PDRIDs   map[uint16]struct{} // 会话关联的PDR ID
	FARIDs   map[uint32]struct{}
	QERIDs   map[uint32]struct{}
	URRIDs   map[uint32]struct{}
	BARIDs   map[uint8]struct{}
	q        map[uint16]chan []byte // key: PDR_ID
	qlen     int
	log      *logrus.Entry
}

// Close 删除该回话所有的PFCP 规则
func (s *Sess) Close() {
	for id := range s.FARIDs {
		i := ie.NewRemoveFAR(ie.NewFARID(id))
		err := s.RemoveFAR(i)
		if err != nil {
			s.log.Errorf("Remove FAR err: %+v", err)
		}
	}
	for id := range s.QERIDs {
		i := ie.NewRemoveQER(ie.NewQERID(id))
		err := s.RemoveQER(i)
		if err != nil {
			s.log.Errorf("Remove QER err: %+v", err)
		}
	}
	for id := range s.URRIDs {
		i := ie.NewRemoveURR(ie.NewURRID(id))
		err := s.RemoveURR(i)
		if err != nil {
			s.log.Errorf("Remove URR err: %+v", err)
		}
	}
	for id := range s.BARIDs {
		i := ie.NewRemoveBAR(ie.NewBARID(id))
		err := s.RemoveBAR(i)
		if err != nil {
			s.log.Errorf("Remove BAR err: %+v", err)
		}
	}
	for id := range s.PDRIDs {
		i := ie.NewRemovePDR(ie.NewPDRID(id))
		err := s.RemovePDR(i)
		if err != nil {
			s.log.Errorf("remove PDR err: %+v", err)
		}
	}
	for _, q := range s.q {
		close(q)
	}
}

func (s *Sess) CreatePDR(req *ie.IE) error {
	err := s.rnode.driver.CreatePDR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.PDRID()
	if err != nil {
		return err
	}
	s.PDRIDs[id] = struct{}{}
	return nil
}

func (s *Sess) UpdatePDR(req *ie.IE) error {
	return s.rnode.driver.UpdatePDR(s.LocalID, req)
}

func (s *Sess) RemovePDR(req *ie.IE) error {
	err := s.rnode.driver.RemovePDR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.PDRID()
	if err != nil {
		return err
	}
	delete(s.PDRIDs, id)
	return nil
}

func (s *Sess) CreateFAR(req *ie.IE) error {
	err := s.rnode.driver.CreateFAR(s.LocalID, req) // 调用底层驱动进行创建规则

	if err != nil {
		return err
	}

	id, err := req.FARID()
	if err != nil {
		return err
	}
	s.FARIDs[id] = struct{}{}
	return nil
}

func (s *Sess) UpdateFAR(req *ie.IE) error {
	return s.rnode.driver.UpdateFAR(s.LocalID, req)
}

func (s *Sess) RemoveFAR(req *ie.IE) error {
	err := s.rnode.driver.RemoveFAR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.FARID()
	if err != nil {
		return err
	}
	delete(s.FARIDs, id)
	return nil
}

func (s *Sess) CreateQER(req *ie.IE) error {
	err := s.rnode.driver.CreateQER(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.QERID()
	if err != nil {
		return err
	}
	s.QERIDs[id] = struct{}{}
	return nil
}

func (s *Sess) UpdateQER(req *ie.IE) error {
	return s.rnode.driver.UpdateQER(s.LocalID, req)
}

func (s *Sess) RemoveQER(req *ie.IE) error {
	err := s.rnode.driver.RemoveQER(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.QERID()
	if err != nil {
		return err
	}
	delete(s.QERIDs, id)
	return nil
}

func (s *Sess) CreateURR(req *ie.IE) error {
	err := s.rnode.driver.CreateURR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.URRID()
	if err != nil {
		return err
	}
	s.URRIDs[id] = struct{}{}
	return nil
}

func (s *Sess) UpdateURR(req *ie.IE) error {
	return s.rnode.driver.UpdateURR(s.LocalID, req)
}

func (s *Sess) RemoveURR(req *ie.IE) error {
	err := s.rnode.driver.RemoveURR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.URRID()
	if err != nil {
		return err
	}
	delete(s.URRIDs, id)
	return nil
}

func (s *Sess) CreateBAR(req *ie.IE) error {
	err := s.rnode.driver.CreateBAR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.BARID()
	if err != nil {
		return err
	}
	s.BARIDs[id] = struct{}{}
	return nil
}

func (s *Sess) UpdateBAR(req *ie.IE) error {
	return s.rnode.driver.UpdateBAR(s.LocalID, req)
}

func (s *Sess) RemoveBAR(req *ie.IE) error {
	err := s.rnode.driver.RemoveBAR(s.LocalID, req)
	if err != nil {
		return err
	}

	id, err := req.BARID()
	if err != nil {
		return err
	}
	delete(s.BARIDs, id)
	return nil
}

// Push pdrid = PDR ID, p 是PDR数据包的内容
func (s *Sess) Push(pdrid uint16, p []byte) {
	pkt := make([]byte, len(p)) // 生成pkt的所需的buffer
	copy(pkt, p)
	q, ok := s.q[pdrid] // 找到pdr id 对应的通道
	if !ok {
		s.q[pdrid] = make(chan []byte, s.qlen) // 如果没有相应的通道，则生成一个s.qlen 大小的通道
		q = s.q[pdrid]
	}

	select {
	case q <- pkt:
		s.log.Debugf("Push bufPkt to q[%d](len:%d)", pdrid, len(q))
	default: // 如果通道阻塞，丢弃数据包
		s.log.Debugf("q[%d](len:%d) is full, drop it", pdrid, len(q))
	}
}

func (s *Sess) Len(pdrid uint16) int {
	q, ok := s.q[pdrid]
	if !ok {
		return 0
	}
	return len(q)
}

// Pop 从通道中取出一个PDR数据包
func (s *Sess) Pop(pdrid uint16) ([]byte, bool) {
	q, ok := s.q[pdrid]
	if !ok {
		return nil, ok
	}
	select {
	case pkt := <-q:
		s.log.Debugf("Pop bufPkt from q[%d](len:%d)", pdrid, len(q))
		return pkt, true
	default:
		return nil, false
	}
}

type RemoteNode struct {
	ID     string              // 可能是 smf的ID？
	addr   net.Addr            // 远程节点的网络地址
	local  *LocalNode          // 与其关联的本地node ID
	sess   map[uint64]struct{} // key: Local SEID， session对象的字典
	driver forwarder.Driver
	log    *logrus.Entry
}

func NewRemoteNode(
	id string,
	addr net.Addr,
	local *LocalNode,
	driver forwarder.Driver,
	log *logrus.Entry,
) *RemoteNode {
	n := new(RemoteNode)
	n.ID = id
	n.addr = addr
	n.local = local
	n.sess = make(map[uint64]struct{})
	n.driver = driver
	n.log = log
	return n
}

func (n *RemoteNode) Reset() {
	for id := range n.sess {
		n.DeleteSess(id)
	}
	n.sess = make(map[uint64]struct{})
}

// Sess 获得远程节点中的session
func (n *RemoteNode) Sess(lSeid uint64) (*Sess, error) {
	_, ok := n.sess[lSeid]
	if !ok {
		return nil, errors.Errorf("Sess: sess not found (lSeid:0x%x)", lSeid)
	}
	return n.local.Sess(lSeid)
}

// NewSess  在远程节点中新建一个session
func (n *RemoteNode) NewSess(rSeid uint64) *Sess {
	s := n.local.NewSess(rSeid, BUFFQ_LEN) // 在关联的本地node中新建一个session
	n.sess[s.LocalID] = struct{}{}
	s.rnode = n
	s.log = n.log.WithField(logger.FieldSessionID, fmt.Sprintf("SEID:L(0x%x),R(0x%x)", s.LocalID, rSeid))
	s.log.Infoln("New session")
	return s
}

// DeleteSess  在远程节点中删除一个session
func (n *RemoteNode) DeleteSess(lSeid uint64) {
	_, ok := n.sess[lSeid]
	if !ok {
		return
	}
	delete(n.sess, lSeid)
	err := n.local.DeleteSess(lSeid)
	if err != nil {
		n.log.Warnln(err)
	}
}

// ======================== LocalNode和相关方法 ========================

type LocalNode struct {
	sess []*Sess  // Session 数组
	free []uint64 // 这个是干什么用的？ 可能用于分配local session ID用的
}

func (n *LocalNode) Reset() {
	for _, sess := range n.sess {
		if sess != nil {
			sess.Close()
		}
	}
	n.sess = []*Sess{}
	n.free = []uint64{}
}

// Sess 从localNode 中获得local session，依靠 local session id
func (n *LocalNode) Sess(lSeid uint64) (*Sess, error) {
	if lSeid == 0 {
		return nil, errors.New("Sess: invalid lSeid:0")
	}
	i := int(lSeid) - 1   // 转化为 数组的index
	if i >= len(n.sess) { // 超过了 session 数组的边界
		return nil, errors.Errorf("Sess: sess not found (lSeid:0x%x)", lSeid)
	}
	sess := n.sess[i]
	if sess == nil {
		return nil, errors.Errorf("Sess: sess not found (lSeid:0x%x)", lSeid)
	}
	return sess, nil
}

func (n *LocalNode) RemoteSess(rSeid uint64, addr net.Addr) (*Sess, error) { // 得到remote sess 的实例
	for _, s := range n.sess {
		if s.RemoteID == rSeid && s.rnode.addr.String() == addr.String() {
			return s, nil
		}
	}
	return nil, errors.Errorf("RemoteSess: invalid rSeid:0x%x, addr:%s ", rSeid, addr)
}

// NewSess 在local node 新建session对象
func (n *LocalNode) NewSess(rSeid uint64, qlen int) *Sess {
	s := &Sess{
		RemoteID: rSeid, // 远程session ID
		PDRIDs:   make(map[uint16]struct{}),
		FARIDs:   make(map[uint32]struct{}),
		QERIDs:   make(map[uint32]struct{}),
		q:        make(map[uint16]chan []byte),
		qlen:     qlen,
	}
	last := len(n.free) - 1 // free 数组的最后一个index
	if last >= 0 {          // free 队列不为0
		s.LocalID = n.free[last] // session的localID赋值为free的最后一个元素, 换句话说就是分配一个单独的ID
		n.free = n.free[:last]   // free的数组，从后向前平移一步，减少free数组
		n.sess[s.LocalID-1] = s  //本地节点的session数组进行赋值
	} else { // 如果n.free已经满了
		n.sess = append(n.sess, s)
		s.LocalID = uint64(len(n.sess))
	}
	return s
}

func (n *LocalNode) DeleteSess(lSeid uint64) error {
	if lSeid == 0 {
		return errors.New("DeleteSess: invalid lSeid:0")
	}
	i := int(lSeid) - 1
	if i >= len(n.sess) {
		return errors.Errorf("DeleteSess: sess not found (lSeid:0x%x)", lSeid)
	}
	if n.sess[i] == nil {
		return errors.Errorf("DeleteSess: sess not found (lSeid:0x%x)", lSeid)
	}
	n.sess[i].log.Infoln("sess deleted")
	n.sess[i].Close()
	n.sess[i] = nil
	n.free = append(n.free, lSeid) // 将空闲的id添加到free数组里
	return nil
}
